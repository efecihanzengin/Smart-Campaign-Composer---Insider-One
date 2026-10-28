package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"campaigncomposer/internal/httpapi"
	"campaigncomposer/internal/llm"
)

// exampleBrief is the Brief from the case, used as the placeholder in the UI.
const exampleBrief = "Son 30 günde sepetine ürün ekleyip alışveriş yapmayan İstanbul'daki kullanıcılara %30 indirim hatırlatması gönder. Samimi ama kısa olsun."

type campaign struct {
	ID      int64  `json:"id"`
	Status  string `json:"status"`
	Brief   string `json:"brief"`
	Segment struct {
		Rule                json.RawMessage `json:"rule"`
		Labels              []string        `json:"labels"`
		UnsupportedCriteria []string        `json:"unsupported_criteria"`
	} `json:"segment"`
	Audience struct {
		SMS   int `json:"sms"`
		Email int `json:"email"`
	} `json:"audience"`
}

func postJSON(t *testing.T, url string, body, into any) int {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer res.Body.Close()
	if err := json.NewDecoder(res.Body).Decode(into); err != nil {
		t.Fatalf("decode %s: %v", url, err)
	}
	return res.StatusCode
}

// scriptedSegment returns a mock LLM whose Segment call always answers with raw.
func scriptedSegment(raw string) *llm.Mock {
	return &llm.Mock{SegmentFunc: func(context.Context, llm.SegmentRequest) (json.RawMessage, error) {
		return json.RawMessage(raw), nil
	}}
}

func TestExampleBriefYieldsSegmentRuleAndPerChannelAudience(t *testing.T) {
	srv, pool := startAPIWithPool(t, httpapi.Options{MockLLM: true, LLM: llm.NewMock()})

	var c campaign
	if code := postJSON(t, srv.URL+"/api/campaigns", map[string]string{"brief": exampleBrief}, &c); code != http.StatusCreated {
		t.Fatalf("status code = %d, want 201", code)
	}
	if c.ID == 0 || c.Status != "draft" || c.Brief != exampleBrief {
		t.Errorf("campaign = {id %d, status %q, brief %q}, want a draft with the Brief", c.ID, c.Status, c.Brief)
	}

	wantLabels := []string{"Şehir: İstanbul", "Son 30 günde sepete ekledi", "Son 30 günde satın almadı"}
	if !slices.Equal(c.Segment.Labels, wantLabels) {
		t.Errorf("labels = %q, want %q", c.Segment.Labels, wantLabels)
	}
	if len(c.Segment.UnsupportedCriteria) != 0 {
		t.Errorf("unsupported_criteria = %q, want none", c.Segment.UnsupportedCriteria)
	}
	if bytes.Contains(bytes.ToLower(c.Segment.Rule), []byte("consent")) {
		t.Errorf("Segment Rule mentions consent: %s", c.Segment.Rule)
	}

	// An independent count over the seed, written directly in SQL.
	var wantSMS, wantEmail int
	queryRow(t, pool, `
		SELECT count(*) FILTER (WHERE sms_consent), count(*) FILTER (WHERE email_consent)
		FROM recipients r
		WHERE city = 'İstanbul'
		  AND EXISTS (SELECT 1 FROM events e WHERE e.recipient_id = r.id AND e.type = 'cart_add'
		              AND e.occurred_at >= now() - interval '30 days')
		  AND NOT EXISTS (SELECT 1 FROM events e WHERE e.recipient_id = r.id AND e.type = 'purchase'
		              AND e.occurred_at >= now() - interval '30 days')`, nil, &wantSMS, &wantEmail)

	if c.Audience.SMS != wantSMS || c.Audience.Email != wantEmail {
		t.Errorf("audience = %d SMS / %d email, want %d / %d", c.Audience.SMS, c.Audience.Email, wantSMS, wantEmail)
	}
	// The seed is tuned so the example Brief demonstrates the consent rule.
	if wantSMS < 100 || wantEmail < 100 || wantSMS == wantEmail {
		t.Errorf("seed gives %d SMS / %d email for the example Brief, want hundreds each and different", wantSMS, wantEmail)
	}
}

func TestUnsupportedCriteriaComeBackAsAWarning(t *testing.T) {
	srv := startAPI(t, httpapi.Options{LLM: scriptedSegment(`{
		"rule": {"all": [{"field": "city", "op": "eq", "value": "Ankara"}]},
		"unsupported_criteria": ["ayakkabı satın alanlar"]
	}`)})

	var c campaign
	if code := postJSON(t, srv.URL+"/api/campaigns", map[string]string{"brief": "Ankara'da ayakkabı alanlara yaz"}, &c); code != http.StatusCreated {
		t.Fatalf("status code = %d, want 201", code)
	}
	if !slices.Equal(c.Segment.UnsupportedCriteria, []string{"ayakkabı satın alanlar"}) {
		t.Errorf("unsupported_criteria = %q, want [ayakkabı satın alanlar]", c.Segment.UnsupportedCriteria)
	}
	if !slices.Equal(c.Segment.Labels, []string{"Şehir: Ankara"}) {
		t.Errorf("labels = %q, want [Şehir: Ankara]", c.Segment.Labels)
	}
}

func TestSegmentRuleThatReferencesConsentIsRejected(t *testing.T) {
	srv := startAPI(t, httpapi.Options{LLM: scriptedSegment(`{
		"rule": {"all": [{"field": "sms_consent", "op": "eq", "value": false}]},
		"unsupported_criteria": []
	}`)})

	var body struct{ Error string }
	if code := postJSON(t, srv.URL+"/api/campaigns", map[string]string{"brief": exampleBrief}, &body); code != http.StatusBadGateway {
		t.Fatalf("status code = %d, want 502", code)
	}
	if body.Error == "" {
		t.Error("error message is empty")
	}
}

func TestEmptyBriefIsRejected(t *testing.T) {
	srv := startAPI(t, httpapi.Options{LLM: llm.NewMock()})

	var body struct{ Error string }
	if code := postJSON(t, srv.URL+"/api/campaigns", map[string]string{"brief": "  "}, &body); code != http.StatusBadRequest {
		t.Fatalf("status code = %d, want 400", code)
	}
}
