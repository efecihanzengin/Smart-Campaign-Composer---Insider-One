package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"campaigncomposer/internal/app"
	"campaigncomposer/internal/httpapi"
	"campaigncomposer/internal/testenv"

	"github.com/jackc/pgx/v5/pgxpool"
)

// startAPI bootstraps a fresh database exactly as the api container does on
// startup and serves the HTTP API in-process.
func startAPI(t *testing.T, opts httpapi.Options) *httptest.Server {
	t.Helper()
	pool := testenv.NewDatabase(t)
	if err := app.Bootstrap(context.Background(), pool, time.Now()); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	srv := httptest.NewServer(httpapi.NewHandler(pool, opts))
	t.Cleanup(srv.Close)
	return srv
}

func getJSON(t *testing.T, url string, into any) int {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer res.Body.Close()
	if err := json.NewDecoder(res.Body).Decode(into); err != nil {
		t.Fatalf("decode %s: %v", url, err)
	}
	return res.StatusCode
}

func TestHealthzIsHealthyWhenPostgresIsReachable(t *testing.T) {
	srv := startAPI(t, httpapi.Options{})

	var body struct{ Status string }
	if code := getJSON(t, srv.URL+"/healthz", &body); code != http.StatusOK {
		t.Fatalf("status code = %d, want 200", code)
	}
	if body.Status != "ok" {
		t.Fatalf("status = %q, want ok", body.Status)
	}
}

type overview struct {
	RecipientCount int `json:"recipient_count"`
	LLM            struct {
		Mock  bool   `json:"mock"`
		Model string `json:"model"`
	} `json:"llm"`
}

func TestOverviewReportsSeededRecipientsAndMockLLM(t *testing.T) {
	srv := startAPI(t, httpapi.Options{MockLLM: true, Model: "claude-sonnet-5-5"})

	var body overview
	if code := getJSON(t, srv.URL+"/api/overview", &body); code != http.StatusOK {
		t.Fatalf("status code = %d, want 200", code)
	}
	if body.RecipientCount != 5000 {
		t.Errorf("recipient_count = %d, want 5000", body.RecipientCount)
	}
	if !body.LLM.Mock {
		t.Errorf("llm.mock = false, want true")
	}
	if body.LLM.Model != "claude-sonnet-5-5" {
		t.Errorf("llm.model = %q, want claude-sonnet-5-5", body.LLM.Model)
	}
}

func TestOverviewReportsRealLLMWhenNotMocked(t *testing.T) {
	srv := startAPI(t, httpapi.Options{MockLLM: false, Model: "claude-opus-5-5"})

	var body overview
	getJSON(t, srv.URL+"/api/overview", &body)
	if body.LLM.Mock {
		t.Errorf("llm.mock = true, want false")
	}
}

func TestHealthzIsUnhealthyWhenPostgresIsUnreachable(t *testing.T) {
	// Nothing listens on port 1; the pool connects lazily, so this only fails on use.
	pool, err := pgxpool.New(context.Background(), "postgres://campaign:campaign@127.0.0.1:1/campaign?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	srv := httptest.NewServer(httpapi.NewHandler(pool, httpapi.Options{}))
	t.Cleanup(srv.Close)

	var body struct{ Status string }
	if code := getJSON(t, srv.URL+"/healthz", &body); code != http.StatusServiceUnavailable {
		t.Fatalf("status code = %d, want 503", code)
	}
	if body.Status != "unhealthy" {
		t.Fatalf("status = %q, want unhealthy", body.Status)
	}
}
