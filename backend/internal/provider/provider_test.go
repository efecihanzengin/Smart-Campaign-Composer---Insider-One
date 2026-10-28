package provider_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

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
	srv := httptest.NewServer(provider.New())
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
	srv := httptest.NewServer(provider.New())
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

func TestARequestWithoutAnIdempotencyKeyIsRejected(t *testing.T) {
	srv := httptest.NewServer(provider.New())
	defer srv.Close()

	code, _ := send(t, srv.URL+"/sms", "", map[string]string{"to": "+905550000001", "body": "Merhaba"})

	if code != http.StatusBadRequest {
		t.Errorf("status code = %d, want 400", code)
	}
	if n := len(received(t, srv.URL).Messages); n != 0 {
		t.Errorf("recorded %d messages, want 0", n)
	}
}
