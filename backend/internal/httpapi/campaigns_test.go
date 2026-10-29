package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
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
	MockBriefUnrecognised bool `json:"mock_brief_unrecognised"`
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

// streamSegment POSTs body to a Segment Rule generation endpoint and returns
// every SSE event, failing the test unless the stream opened.
func streamSegment(t *testing.T, url string, body any) []sseEvent {
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
	if ct := res.Header.Get("Content-Type"); res.StatusCode != http.StatusOK || !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("POST %s: status code %d, Content-Type %q; want a 200 event stream", url, res.StatusCode, ct)
	}
	return readSSE(t, res.Body)
}

// segmentDone decodes the stream's final segment.done event into into.
func segmentDone(t *testing.T, events []sseEvent, into any) {
	t.Helper()
	if len(events) == 0 || events[len(events)-1].Name != "segment.done" {
		t.Fatalf("stream did not end with segment.done: %s", eventNames(events))
	}
	if err := json.Unmarshal(events[len(events)-1].Data, into); err != nil {
		t.Fatalf("decode segment.done: %v", err)
	}
}

type streamFailure struct {
	Error     string `json:"error"`
	Retryable bool   `json:"retryable"`
}

// lastError decodes the stream's final error event.
func lastError(t *testing.T, events []sseEvent) streamFailure {
	t.Helper()
	if len(events) == 0 || events[len(events)-1].Name != "error" {
		t.Fatalf("stream did not end with an error: %s", eventNames(events))
	}
	var e streamFailure
	if err := json.Unmarshal(events[len(events)-1].Data, &e); err != nil {
		t.Fatalf("decode error event: %v", err)
	}
	return e
}

// scriptedSegment returns a mock LLM whose Segment call always answers with raw.
func scriptedSegment(raw string) *llm.Mock {
	return &llm.Mock{SegmentFunc: func(context.Context, llm.SegmentRequest) (json.RawMessage, error) {
		return json.RawMessage(raw), nil
	}}
}

func TestMockFlagsABriefOtherThanTheExample(t *testing.T) {
	srv := startAPI(t, httpapi.Options{MockLLM: true, LLM: llm.NewMock()})

	pasted := "  \u201c" + strings.ReplaceAll(strings.ReplaceAll(exampleBrief, "'", "\u2019"), " ", "  ") + "\u201d\n"
	if c := createCampaign(t, srv.URL, pasted); c.MockBriefUnrecognised {
		t.Errorf("example Brief with other whitespace and quotes: mock_brief_unrecognised = true, want false")
	}
	if c := createCampaign(t, srv.URL, "Ankara'daki kullanıcılara yeni yıl kutlaması gönder."); !c.MockBriefUnrecognised {
		t.Errorf("other Brief: mock_brief_unrecognised = false, want true")
	}

	real := startAPI(t, httpapi.Options{MockLLM: false, LLM: llm.NewMock()})
	if c := createCampaign(t, real.URL, "Ankara'daki kullanıcılara yeni yıl kutlaması gönder."); c.MockBriefUnrecognised {
		t.Errorf("real LLM: mock_brief_unrecognised = true, want false")
	}
}

func TestExampleBriefYieldsSegmentRuleAndPerChannelAudience(t *testing.T) {
	srv, pool := startAPIWithPool(t, httpapi.Options{MockLLM: true, LLM: llm.NewMock()})

	c := createCampaign(t, srv.URL, exampleBrief)
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

	c := createCampaign(t, srv.URL, "Ankara'da ayakkabı alanlara yaz")
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

	failure := lastError(t, streamSegment(t, srv.URL+"/api/campaigns", map[string]string{"brief": exampleBrief}))
	if failure.Error == "" || !failure.Retryable {
		t.Errorf("error = %+v, want a retryable error message", failure)
	}
}

func TestEmptyBriefIsRejected(t *testing.T) {
	srv := startAPI(t, httpapi.Options{LLM: llm.NewMock()})

	var body struct{ Error string }
	if code := postJSON(t, srv.URL+"/api/campaigns", map[string]string{"brief": "  "}, &body); code != http.StatusBadRequest {
		t.Fatalf("status code = %d, want 400", code)
	}
}
