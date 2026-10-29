// Package provider is the mock SMS and email Provider. It accepts
// `POST /sms` and `POST /email`, deduplicates on the Idempotency-Key header
// (ADR-0006) and records every message it accepted, which `GET /messages`
// lists so tests and operators can count what was actually sent.
//
// It misbehaves on purpose so retries can be exercised: a request can be
// throttled (429 with Retry-After), fail (503), or be accepted but answered
// only after the client has given up (a timeout after accept). Outcomes are
// drawn at random from the rates in Options, or pinned per request by
// Options.Script for deterministic tests.
package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"
)

// Outcome is how the Provider answers one request.
type Outcome struct {
	// Code is the response status code. Zero (or any 2xx) accepts the
	// message; anything else rejects this request without recording a
	// message, so a later request with the same key can still be accepted.
	Code int
	// RetryAfter is sent as the Retry-After header when set.
	RetryAfter string
	// Delay holds the response back. An acceptance is recorded before the
	// delay, so a Delay beyond the client timeout is a timeout after accept.
	Delay time.Duration
}

// Script pins the outcome of a request: request is 1 for the first request
// with this Channel and Idempotency-Key, 2 for the next, and so on. Once a
// key's message is accepted, every repeat replays that acceptance and the
// script is no longer asked.
type Script func(channel, key string, request int) Outcome

// Options configures the Provider's behaviour. The zero value accepts every
// request immediately.
type Options struct {
	// Script, when set, decides every outcome and the fields below are
	// ignored.
	Script Script

	// Rate429, Rate5xx and RateSlow are the shares (0..1) of requests for a
	// not yet accepted key that are throttled, fail with 503, or are accepted
	// but answered only after SlowDelay.
	Rate429, Rate5xx, RateSlow float64
	// RetryAfter is the Retry-After value sent with a 429, in seconds.
	RetryAfter string
	// SlowDelay is how long a slow response takes; it should exceed the
	// worker's client timeout.
	SlowDelay time.Duration
	// LatencyMin and LatencyMax bound the latency of every other response.
	LatencyMin, LatencyMax time.Duration
	// Seed seeds the random outcomes.
	Seed uint64
}

// Provider is the mock Provider's HTTP handler.
type Provider struct {
	mux    *http.ServeMux
	script Script

	mu       sync.Mutex
	messages map[messageKey]*message
	order    []messageKey
	requests map[messageKey]int // requests per key, accepted or not
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

// New returns an empty mock Provider behaving as opts says.
func New(opts Options) *Provider {
	p := &Provider{
		mux:      http.NewServeMux(),
		script:   opts.Script,
		messages: map[messageKey]*message{},
		requests: map[messageKey]int{},
	}
	if p.script == nil {
		p.script = opts.random()
	}
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
		p.requests[k]++
		m, seen := p.messages[k]
		var out Outcome
		if !seen {
			// The script runs under the lock, so the random script needs no
			// locking of its own.
			out = p.script(channel, key, p.requests[k])
			if out.Code == 0 || out.Code/100 == 2 {
				m = &message{Channel: channel, IdempotencyKey: key, Payload: payload}
				m.ID = fmt.Sprintf("%s-%d", channel, len(p.order)+1)
				m.code = http.StatusAccepted
				if out.Code != 0 {
					m.code = out.Code
				}
				m.body, _ = json.Marshal(map[string]string{"id": m.ID, "status": "accepted"})
				p.messages[k] = m
				p.order = append(p.order, k)
				seen = true
			}
		}
		var code int
		var body []byte
		if seen {
			m.Requests++
			code, body = m.code, m.body
		} else {
			code = out.Code
			body, _ = json.Marshal(map[string]string{"error": http.StatusText(out.Code)})
		}
		p.mu.Unlock()

		if out.Delay > 0 {
			select {
			case <-time.After(out.Delay):
			case <-r.Context().Done():
				return // The client gave up; an acceptance above still stands.
			}
		}
		w.Header().Set("Content-Type", "application/json")
		if out.RetryAfter != "" {
			w.Header().Set("Retry-After", out.RetryAfter)
		}
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

// random returns the Script that draws outcomes from the rates in o.
func (o Options) random() Script {
	rng := rand.New(rand.NewPCG(o.Seed, o.Seed^0x9e3779b97f4a7c15))
	return func(channel, key string, request int) Outcome {
		latency := o.LatencyMin
		if span := o.LatencyMax - o.LatencyMin; span > 0 {
			latency += time.Duration(rng.Int64N(int64(span)))
		}
		switch x := rng.Float64(); {
		case x < o.Rate429:
			return Outcome{Code: http.StatusTooManyRequests, RetryAfter: o.RetryAfter, Delay: latency}
		case x < o.Rate429+o.Rate5xx:
			return Outcome{Code: http.StatusServiceUnavailable, Delay: latency}
		case x < o.Rate429+o.Rate5xx+o.RateSlow:
			return Outcome{Delay: o.SlowDelay}
		}
		return Outcome{Delay: latency}
	}
}

// OptionsFromEnv reads the Provider's behaviour from the environment, with
// the defaults documented in .env.example.
func OptionsFromEnv(getenv func(string) string) (Options, error) {
	o := Options{
		Rate429: 0.10, Rate5xx: 0.05, RateSlow: 0.03,
		RetryAfter: "1",
		SlowDelay:  10 * time.Second,
		LatencyMin: 20 * time.Millisecond, LatencyMax: 300 * time.Millisecond,
		Seed: uint64(time.Now().UnixNano()),
	}
	for name, dst := range map[string]*float64{
		"PROVIDER_RATE_429": &o.Rate429, "PROVIDER_RATE_5XX": &o.Rate5xx, "PROVIDER_RATE_SLOW": &o.RateSlow,
	} {
		if v := getenv(name); v != "" {
			f, err := strconv.ParseFloat(v, 64)
			if err != nil || f < 0 || f > 1 {
				return o, fmt.Errorf("%s must be a number between 0 and 1, got %q", name, v)
			}
			*dst = f
		}
	}
	for name, dst := range map[string]*time.Duration{
		"PROVIDER_SLOW_MS": &o.SlowDelay, "PROVIDER_LATENCY_MIN_MS": &o.LatencyMin, "PROVIDER_LATENCY_MAX_MS": &o.LatencyMax,
	} {
		if v := getenv(name); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 0 {
				return o, fmt.Errorf("%s must be a non-negative number of milliseconds, got %q", name, v)
			}
			*dst = time.Duration(n) * time.Millisecond
		}
	}
	if v := getenv("PROVIDER_RETRY_AFTER"); v != "" {
		o.RetryAfter = v
	}
	if v := getenv("PROVIDER_SEED"); v != "" {
		n, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			return o, fmt.Errorf("PROVIDER_SEED must be a non-negative integer, got %q", v)
		}
		o.Seed = n
	}
	if o.Rate429+o.Rate5xx+o.RateSlow > 1 {
		return o, fmt.Errorf("PROVIDER_RATE_429 + PROVIDER_RATE_5XX + PROVIDER_RATE_SLOW must not exceed 1")
	}
	if o.LatencyMax < o.LatencyMin {
		return o, fmt.Errorf("PROVIDER_LATENCY_MAX_MS must not be below PROVIDER_LATENCY_MIN_MS")
	}
	return o, nil
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}
