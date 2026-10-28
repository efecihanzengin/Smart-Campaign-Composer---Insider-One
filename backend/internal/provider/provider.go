// Package provider is the mock SMS and email Provider. It accepts
// `POST /sms` and `POST /email`, deduplicates on the Idempotency-Key header
// (ADR-0006) and records every message it accepted, which `GET /messages`
// lists so tests and operators can count what was actually sent.
package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"sync"
)

// Provider is the mock Provider's HTTP handler. In this slice every request
// succeeds; configurable failures arrive with retries (#13).
type Provider struct {
	mux *http.ServeMux

	mu       sync.Mutex
	messages map[messageKey]*message
	order    []messageKey
}

// messageKey identifies a message: idempotency keys are scoped per Channel.
type messageKey struct {
	channel string
	key     string
}

type message struct {
	Channel        string          `json:"channel"`
	IdempotencyKey string          `json:"idempotency_key"`
	ID             string          `json:"id"`
	Requests       int             `json:"requests"`
	Payload        json.RawMessage `json:"payload"`

	// The response returned for this key, replayed for every repeat.
	code int
	body []byte
}

// New returns an empty mock Provider.
func New() *Provider {
	p := &Provider{mux: http.NewServeMux(), messages: map[messageKey]*message{}}
	p.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	p.mux.HandleFunc("POST /sms", p.accept("sms"))
	p.mux.HandleFunc("POST /email", p.accept("email"))
	p.mux.HandleFunc("GET /messages", p.list)
	return p
}

func (p *Provider) ServeHTTP(w http.ResponseWriter, r *http.Request) { p.mux.ServeHTTP(w, r) }

func (p *Provider) accept(channel string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("Idempotency-Key")
		if key == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Idempotency-Key header is required"})
			return
		}
		payload, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil || !json.Valid(payload) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body must be JSON"})
			return
		}

		p.mu.Lock()
		k := messageKey{channel, key}
		m, seen := p.messages[k]
		if !seen {
			m = &message{Channel: channel, IdempotencyKey: key, Payload: payload}
			m.ID = fmt.Sprintf("%s-%d", channel, len(p.order)+1)
			m.code = http.StatusAccepted
			m.body, _ = json.Marshal(map[string]string{"id": m.ID, "status": "accepted"})
			p.messages[k] = m
			p.order = append(p.order, k)
		}
		m.Requests++
		code, body := m.code, m.body
		p.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_, _ = w.Write(body)
	}
}

func (p *Provider) list(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	out := make([]message, 0, len(p.order))
	for _, k := range p.order {
		out = append(out, *p.messages[k])
	}
	p.mu.Unlock()
	sort.SliceStable(out, func(i, j int) bool { return out[i].Channel < out[j].Channel })
	writeJSON(w, http.StatusOK, map[string]any{"messages": out})
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}
