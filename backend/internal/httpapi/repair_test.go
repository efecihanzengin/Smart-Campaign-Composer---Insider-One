package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"

	"campaigncomposer/internal/httpapi"
	"campaigncomposer/internal/llm"

	"github.com/jackc/pgx/v5/pgxpool"
)

// segmentScript answers the Segment call with each answer in turn, repeating
// the last one, and records every request.
type segmentScript struct {
	answers  []string
	requests []llm.SegmentRequest
}

func (s *segmentScript) call(_ context.Context, req llm.SegmentRequest) (json.RawMessage, error) {
	s.requests = append(s.requests, req)
	return json.RawMessage(s.answers[min(len(s.requests), len(s.answers))-1]), nil
}

const (
	unknownFieldSegment = `{"rule": {"all": [{"field": "email", "op": "eq", "value": "x@example.com"}]}, "unsupported_criteria": []}`
	ankaraSegment       = `{"rule": {"all": [{"field": "city", "op": "eq", "value": "Ankara"}]}, "unsupported_criteria": []}`
)

type createdCampaign struct {
	campaign
	Repairs int `json:"repairs"`
}

func TestInvalidSegmentRuleIsRepairedIntoTheValidOne(t *testing.T) {
	script := &segmentScript{answers: []string{unknownFieldSegment, ankaraSegment}}
	srv := startAPI(t, httpapi.Options{LLM: &llm.Mock{SegmentFunc: script.call}})

	var c createdCampaign
	if code := postJSON(t, srv.URL+"/api/campaigns", map[string]string{"brief": exampleBrief}, &c); code != http.StatusCreated {
		t.Fatalf("status code = %d, want 201", code)
	}
	if !slices.Equal(c.Segment.Labels, []string{"Şehir: Ankara"}) {
		t.Errorf("labels = %q, want the repaired rule's [Şehir: Ankara]", c.Segment.Labels)
	}
	if c.Repairs != 1 {
		t.Errorf("repairs = %d, want 1", c.Repairs)
	}
	if len(script.requests) != 2 {
		t.Fatalf("Segment calls = %d, want 2", len(script.requests))
	}
	repair := script.requests[1].Repair
	if repair == nil {
		t.Fatalf("second Segment call carries no repair")
	}
	if repair.Previous != unknownFieldSegment {
		t.Errorf("repair previous output = %q, want the rejected output", repair.Previous)
	}
	if !strings.Contains(strings.Join(repair.Errors, "\n"), "rule.all[0].field: unknown field") {
		t.Errorf("repair errors = %q, want the validator's message about the unknown field", repair.Errors)
	}
	if script.requests[1].Brief != exampleBrief {
		t.Errorf("repair Brief = %q, want the original Brief", script.requests[1].Brief)
	}
}

func TestSegmentRuleStillInvalidAfterTwoRepairsIsARetryableError(t *testing.T) {
	// Malformed JSON, then a failed call, then an unknown field: three strikes.
	script := &segmentScript{answers: []string{`{"rule": {"all": [`, unknownFieldSegment}}
	failSecond := func(ctx context.Context, req llm.SegmentRequest) (json.RawMessage, error) {
		if len(script.requests) == 1 {
			script.requests = append(script.requests, req)
			return nil, errors.New("segment call: Claude did not call propose_segment")
		}
		return script.call(ctx, req)
	}
	srv, pool := startAPIWithPool(t, httpapi.Options{LLM: &llm.Mock{SegmentFunc: failSecond}})

	var body struct {
		Error     string `json:"error"`
		Retryable bool   `json:"retryable"`
	}
	if code := postJSON(t, srv.URL+"/api/campaigns", map[string]string{"brief": exampleBrief}, &body); code != http.StatusBadGateway {
		t.Fatalf("status code = %d, want 502", code)
	}
	if body.Error == "" || !body.Retryable {
		t.Errorf("body = %+v, want a retryable error message", body)
	}
	if len(script.requests) != 3 {
		t.Fatalf("Segment calls = %d, want 1 + 2 repairs", len(script.requests))
	}
	if errs := strings.Join(script.requests[1].Repair.Errors, "\n"); !strings.Contains(errs, "proposal: must be an object") {
		t.Errorf("first repair errors = %q, want the malformed JSON reported", errs)
	}
	if errs := strings.Join(script.requests[2].Repair.Errors, "\n"); !strings.Contains(errs, "did not call propose_segment") {
		t.Errorf("second repair errors = %q, want the failed call reported", errs)
	}
	var n int
	queryRow(t, pool, `SELECT count(*) FROM campaigns`, nil, &n)
	if n != 0 {
		t.Errorf("%d Campaigns stored, want none for a failed generation", n)
	}
}

// contentScript streams each answer in turn (in two chunks), repeating the
// last one, and records every request.
type contentScript struct {
	answers  []string
	requests []llm.ContentRequest
}

func (s *contentScript) call(_ context.Context, req llm.ContentRequest, emit func(string)) error {
	s.requests = append(s.requests, req)
	a := s.answers[min(len(s.requests), len(s.answers))-1]
	emit(a[:len(a)/2])
	emit(a[len(a)/2:])
	return nil
}

func smsOnly(body string) string {
	return `{"channels": ["sms"], "sms": {"body": "` + body + `"}}`
}

type repairNotice struct {
	Repair int      `json:"repair"`
	Max    int      `json:"max"`
	Errors []string `json:"errors"`
}

func repairNotices(t *testing.T, events []sseEvent) []repairNotice {
	t.Helper()
	var out []repairNotice
	for _, e := range events {
		if e.Name != "content.repair" {
			continue
		}
		var n repairNotice
		if err := json.Unmarshal(e.Data, &n); err != nil {
			t.Fatalf("decode content.repair: %v", err)
		}
		out = append(out, n)
	}
	return out
}

// repairAPI serves the API with an Ankara Segment Rule and the scripted Content call.
func repairAPI(t *testing.T, script *contentScript) string {
	t.Helper()
	baseURL, _ := repairAPIWithPool(t, script)
	return baseURL
}

func repairAPIWithPool(t *testing.T, script *contentScript) (string, *pgxpool.Pool) {
	t.Helper()
	srv, pool := startAPIWithPool(t, httpapi.Options{LLM: &llm.Mock{
		SegmentFunc: (&segmentScript{answers: []string{ankaraSegment}}).call,
		ContentFunc: script.call,
	}})
	return srv.URL, pool
}

func TestInvalidContentIsRepairedIntoTheValidContent(t *testing.T) {
	tooLong := strings.Repeat("a", 172) // 12 septets over the GSM-7 limit
	valid := "Merhaba {{first_name}}, sepetin seni bekliyor!"
	script := &contentScript{answers: []string{smsOnly(tooLong), smsOnly(valid)}}
	baseURL := repairAPI(t, script)
	c := createCampaign(t, baseURL, exampleBrief)

	events := streamContent(t, baseURL, c.ID)

	done := lastDone(t, events)
	if done.Content.SMS == nil || done.Content.SMS.Body != valid {
		t.Errorf("SMS Content = %+v, want the repaired body", done.Content.SMS)
	}
	if v := done.Validation["sms"]; !v.Valid {
		t.Errorf("sms validation = %+v, want valid", v)
	}
	notices := repairNotices(t, events)
	if len(notices) != 1 || notices[0].Repair != 1 || notices[0].Max != 2 {
		t.Fatalf("repair notices = %+v, want one: repair 1 of 2", notices)
	}
	if len(script.requests) != 2 {
		t.Fatalf("Content calls = %d, want 2", len(script.requests))
	}
	repair := script.requests[1].Repair
	if repair == nil || repair.Previous != smsOnly(tooLong) {
		t.Fatalf("second Content call repair = %+v, want the rejected output", repair)
	}
	errs := strings.Join(repair.Errors, "\n")
	if !strings.Contains(errs, "sms.body") || !strings.Contains(errs, "160") || !strings.Contains(errs, "172") {
		t.Errorf("repair errors = %q, want the validator's SMS length message", errs)
	}
	if !slices.Equal(script.requests[1].AudienceLabels, []string{"Şehir: Ankara"}) || script.requests[1].Brief != exampleBrief {
		t.Errorf("repair request = %+v, want the original Brief and Audience", script.requests[1])
	}
	// The notice comes before the repaired attempt streams.
	names := eventNames(events)
	if i := slices.Index(names, "content.repair"); i < 0 || !slices.Contains(names[i:], "content.delta") {
		t.Errorf("events = %q, want content.repair followed by the repaired stream", names)
	}
}

func TestMalformedContentIsRepaired(t *testing.T) {
	valid := smsOnly("Merhaba {{first_name}}")
	script := &contentScript{answers: []string{`{"channels": ["sms"], "sms": {"body": "Mer`, valid}}
	baseURL := repairAPI(t, script)
	c := createCampaign(t, baseURL, exampleBrief)

	events := streamContent(t, baseURL, c.ID)

	if done := lastDone(t, events); done.Content.SMS == nil || done.Content.SMS.Body != "Merhaba {{first_name}}" {
		t.Errorf("SMS Content = %+v, want the repaired body", done.Content.SMS)
	}
	if n := repairNotices(t, events); len(n) != 1 || !strings.Contains(strings.Join(n[0].Errors, ""), "not valid JSON") {
		t.Errorf("repair notices = %+v, want one reporting the malformed JSON", n)
	}
}

func TestContentStillInvalidAfterTwoRepairsIsARetryableErrorAndNothingIsStored(t *testing.T) {
	// Malformed JSON, then a call cut off by max_tokens, then an unknown placeholder.
	script := &contentScript{answers: []string{`{"channels": ["sms"], "sms": {"body": "Mer`, smsOnly("Merhaba {{last_name}}")}}
	calls := 0
	m := &llm.Mock{
		SegmentFunc: (&segmentScript{answers: []string{ankaraSegment}}).call,
		ContentFunc: func(ctx context.Context, req llm.ContentRequest, emit func(string)) error {
			calls++
			if calls == 2 {
				script.requests = append(script.requests, req)
				return errors.New("content call: stop_reason max_tokens")
			}
			return script.call(ctx, req, emit)
		},
	}
	srv, pool := startAPIWithPool(t, httpapi.Options{LLM: m})
	c := createCampaign(t, srv.URL, exampleBrief)

	events := streamContent(t, srv.URL, c.ID)

	names := eventNames(events)
	if slices.Contains(names, "content.done") {
		t.Fatalf("events = %q, want no content.done for invalid output", names)
	}
	last := events[len(events)-1]
	var body struct {
		Error     string `json:"error"`
		Retryable bool   `json:"retryable"`
	}
	if err := json.Unmarshal(last.Data, &body); last.Name != "error" || err != nil || body.Error == "" || !body.Retryable {
		t.Fatalf("last event = %s %s, want a retryable error", last.Name, last.Data)
	}
	if calls != 3 {
		t.Errorf("Content calls = %d, want 1 + 2 repairs", calls)
	}
	notices := repairNotices(t, events)
	if len(notices) != 2 || notices[1].Repair != 2 {
		t.Fatalf("repair notices = %+v, want repairs 1 and 2", notices)
	}
	if !strings.Contains(strings.Join(notices[1].Errors, ""), "max_tokens") {
		t.Errorf("second repair errors = %q, want the failed call reported", notices[1].Errors)
	}
	var stored int
	queryRow(t, pool, `SELECT count(*) FROM contents WHERE campaign_id = $1`, []any{c.ID}, &stored)
	if stored != 0 {
		t.Errorf("%d Content rows stored, want none", stored)
	}
}
