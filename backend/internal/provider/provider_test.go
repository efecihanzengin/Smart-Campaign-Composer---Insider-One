package provider_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"campaigncomposer/internal/provider"
)

func send(t *testing.T, url, key string, body any) (int, []byte) {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer res.Body.Close()
	out, _ := io.ReadAll(res.Body)
	return res.StatusCode, out
}

type messages struct {
	Messages []struct {
		Channel        string          `json:"channel"`
		IdempotencyKey string          `json:"idempotency_key"`
		Requests       int             `json:"requests"`
		Payload        json.RawMessage `json:"payload"`
	} `json:"messages"`
}

func received(t *testing.T, baseURL string) messages {
	t.Helper()
	res, err := http.Get(baseURL + "/messages")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var m messages
	if err := json.NewDecoder(res.Body).Decode(&m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestARepeatedIdempotencyKeyGetsTheOriginalResultAndRecordsOneMessage(t *testing.T) {
	srv := httptest.NewServer(provider.New(provider.Options{}))
	defer srv.Close()
	sms := map[string]string{"to": "+905550000001", "body": "Merhaba"}

	code1, body1 := send(t, srv.URL+"/sms", "42", sms)
	code2, body2 := send(t, srv.URL+"/sms", "42", sms)

	if code1/100 != 2 || code2 != code1 || !bytes.Equal(body1, body2) {
		t.Errorf("responses = %d %s and %d %s; want the same 2xx twice", code1, body1, code2, body2)
	}
	m := received(t, srv.URL)
	if len(m.Messages) != 1 {
		t.Fatalf("recorded %d messages, want 1: %+v", len(m.Messages), m)
	}
	if got := m.Messages[0]; got.Channel != "sms" || got.IdempotencyKey != "42" || got.Requests != 2 {
		t.Errorf("message = %+v, want sms, key 42, 2 requests", got)
	}
}

func TestDifferentKeysAreDifferentMessages(t *testing.T) {
	srv := httptest.NewServer(provider.New(provider.Options{}))
	defer srv.Close()

	_, a := send(t, srv.URL+"/email", "1", map[string]string{"to": "a@example.com", "subject": "S", "html": "<p>x</p>"})
	_, b := send(t, srv.URL+"/email", "2", map[string]string{"to": "b@example.com", "subject": "S", "html": "<p>x</p>"})

	if bytes.Equal(a, b) {
		t.Errorf("both messages got the response %s", a)
	}
	if n := len(received(t, srv.URL).Messages); n != 2 {
		t.Errorf("recorded %d messages, want 2", n)
	}
}

func env(vars map[string]string) func(string) string {
	return func(k string) string { return vars[k] }
}

func TestFailureRatesDefaultToTheDocumentedValues(t *testing.T) {
	o, err := provider.OptionsFromEnv(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if o.Rate429 != 0.10 || o.Rate5xx != 0.05 || o.RateSlow != 0.03 ||
		o.LatencyMin != 20*time.Millisecond || o.LatencyMax != 300*time.Millisecond {
		t.Errorf("defaults = %+v, want 10%% 429, 5%% 5xx, 3%% slow, 20-300ms", o)
	}
}

func TestAnInvalidFailureRateIsRejected(t *testing.T) {
	if _, err := provider.OptionsFromEnv(env(map[string]string{"PROVIDER_RATE_5XX": "1.5"})); err == nil {
		t.Error("a 5xx rate of 1.5 was accepted")
	}
}

func TestA429RateOfOneThrottlesEveryNewKeyWithRetryAfter(t *testing.T) {
	o, err := provider.OptionsFromEnv(env(map[string]string{
		"PROVIDER_RATE_429": "1", "PROVIDER_RATE_5XX": "0", "PROVIDER_RATE_SLOW": "0",
		"PROVIDER_RETRY_AFTER": "7", "PROVIDER_LATENCY_MIN_MS": "0", "PROVIDER_LATENCY_MAX_MS": "0",
	}))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(provider.New(o))
	defer srv.Close()

	b, _ := json.Marshal(map[string]string{"to": "+905550000001", "body": "Merhaba"})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/sms", bytes.NewReader(b))
	req.Header.Set("Idempotency-Key", "1")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	if res.StatusCode != http.StatusTooManyRequests || res.Header.Get("Retry-After") != "7" {
		t.Errorf("response = %d with Retry-After %q, want 429 with 7", res.StatusCode, res.Header.Get("Retry-After"))
	}
	if n := len(received(t, srv.URL).Messages); n != 0 {
		t.Errorf("recorded %d messages, want 0 for a throttled request", n)
	}
}

func TestAScriptedRejectionIsNotRememberedForTheKey(t *testing.T) {
	srv := httptest.NewServer(provider.New(provider.Options{Script: func(channel, key string, request int) provider.Outcome {
		if request == 1 {
			return provider.Outcome{Code: http.StatusServiceUnavailable}
		}
		return provider.Outcome{}
	}}))
	defer srv.Close()
	sms := map[string]string{"to": "+905550000001", "body": "Merhaba"}

	code1, _ := send(t, srv.URL+"/sms", "9", sms)
	code2, _ := send(t, srv.URL+"/sms", "9", sms)
	code3, _ := send(t, srv.URL+"/sms", "9", sms)

	if code1 != http.StatusServiceUnavailable || code2 != http.StatusAccepted || code3 != http.StatusAccepted {
		t.Errorf("responses = %d, %d, %d; want 503, then 202 twice", code1, code2, code3)
	}
	if m := received(t, srv.URL).Messages; len(m) != 1 || m[0].Requests != 2 {
		t.Errorf("messages = %+v, want one with 2 requests", m)
	}
}

func TestARequestWithoutAnIdempotencyKeyIsRejected(t *testing.T) {
	srv := httptest.NewServer(provider.New(provider.Options{}))
	defer srv.Close()

	code, _ := send(t, srv.URL+"/sms", "", map[string]string{"to": "+905550000001", "body": "Merhaba"})

	if code != http.StatusBadRequest {
		t.Errorf("status code = %d, want 400", code)
	}
	if n := len(received(t, srv.URL).Messages); n != 0 {
		t.Errorf("recorded %d messages, want 0", n)
	}
}
