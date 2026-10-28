// Package httpapi is the HTTP surface of the api process.
package httpapi

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"campaigncomposer/internal/llm"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Options configures the handler.
type Options struct {
	// MockLLM is true when no Anthropic API key is configured and the scripted
	// mock LLM stands in for Claude.
	MockLLM bool
	Model   string
	// LLM answers the Segment call; nil means the scripted mock.
	LLM llm.LLM
	// DashboardInterval is how often the dashboard stream re-reads the
	// Delivery counts; zero means 500ms.
	DashboardInterval time.Duration
}

type server struct {
	pool *pgxpool.Pool
	opts Options
}

// NewHandler returns the API's HTTP handler.
func NewHandler(pool *pgxpool.Pool, opts Options) http.Handler {
	if opts.LLM == nil {
		opts.LLM = llm.NewMock()
	}
	if opts.DashboardInterval <= 0 {
		opts.DashboardInterval = 500 * time.Millisecond
	}
	s := &server{pool: pool, opts: opts}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("GET /metrics", s.metrics)
	mux.HandleFunc("GET /api/overview", s.overview)
	mux.HandleFunc("POST /api/campaigns", s.createCampaign)
	mux.HandleFunc("GET /api/campaigns/{id}", s.getCampaign)
	mux.HandleFunc("GET /api/campaigns/{id}/deliveries", s.listDeliveries)
	mux.HandleFunc("GET /api/dashboard", s.dashboard)
	mux.HandleFunc("POST /api/campaigns/{id}/content", s.generateContent)
	mux.HandleFunc("PUT /api/campaigns/{id}/content/{channel}", s.updateContent)
	mux.HandleFunc("POST /api/campaigns/{id}/content/{channel}/validate", s.validateContent)
	mux.HandleFunc("POST /api/campaigns/{id}/content/{channel}/regenerate", s.regenerateContent)
	mux.HandleFunc("POST /api/campaigns/{id}/segment/regenerate", s.regenerateSegment)
	mux.HandleFunc("PUT /api/campaigns/{id}/channels/{channel}", s.setChannel)
	mux.HandleFunc("POST /api/campaigns/{id}/approve", s.approveCampaign)
	mux.HandleFunc("POST /api/campaigns/{id}/cancel", s.cancelCampaign)
	mux.HandleFunc("POST /api/email/preview", s.previewEmail)
	return mux
}

func (s *server) healthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.pool.Ping(ctx); err != nil {
		log.Printf("healthz: %v", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unhealthy"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *server) overview(w http.ResponseWriter, r *http.Request) {
	var count int
	if err := s.pool.QueryRow(r.Context(), "SELECT count(*) FROM recipients").Scan(&count); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"recipient_count": count,
		"llm":             map[string]any{"mock": s.opts.MockLLM, "model": s.opts.Model},
	})
}

func writeError(w http.ResponseWriter, err error) {
	log.Printf("internal error: %v", err)
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("write response: %v", err)
	}
}
