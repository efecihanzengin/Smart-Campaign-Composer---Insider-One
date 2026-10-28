package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"

	"campaigncomposer/internal/httpapi"
	"campaigncomposer/internal/llm"
)

const (
	originalSMS  = "Merhaba {{first_name}}, sepetin seni bekliyor!"
	bothChannels = `{"channels": ["sms", "email"], "sms": {"body": "` + originalSMS + `"},
		"email": {"subject": "{{first_name}}, sepetin seni bekliyor", "preheader": "Ürünler hâlâ orada",
		"body": "Merhaba {{first_name}},\n\nSiparişini tamamla.", "cta_text": "Siparişi tamamla"}}`
	shorterSMS = "Sepetin seni bekliyor!"
)

// regenerateContent asks Claude to rewrite one Channel's Content and returns every SSE event.
func regenerateContent(t *testing.T, baseURL string, id int64, channel, instruction string) []sseEvent {
	t.Helper()
	b, _ := json.Marshal(map[string]string{"instruction": instruction})
	res, err := http.Post(contentURL(baseURL, id, channel)+"/regenerate", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("POST regenerate: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST regenerate %s: status code = %d, want 200", channel, res.StatusCode)
	}
	return readSSE(t, res.Body)
}

func segmentURL(baseURL string, id int64) string {
	return baseURL + "/api/campaigns/" + strconv.FormatInt(id, 10) + "/segment/regenerate"
}

// draftWithContent creates a Campaign with the Ankara Segment Rule and Content
// for both Channels, then lets the script answer the Content calls that follow.
func draftWithContent(t *testing.T, later ...string) (string, campaign, *contentScript) {
	t.Helper()
	script := &contentScript{answers: append([]string{bothChannels}, later...)}
	baseURL := repairAPI(t, script)
	c := createCampaign(t, baseURL, exampleBrief)
	lastDone(t, streamContent(t, baseURL, c.ID))
	return baseURL, c, script
}

func TestRegeneratingTheSMSSendsTheInstructionAndKeepsTheEditedEmail(t *testing.T) {
	baseURL, c, script := draftWithContent(t, smsOnly(shorterSMS))
	edited := emailContent{Subject: "Elle yazılmış konu", Preheader: "Ön izleme", Body: "Elle yazılmış gövde."}
	var saved savedContent
	if code := putJSON(t, contentURL(baseURL, c.ID, "email"), edited, &saved); code != http.StatusOK {
		t.Fatalf("edit email: status code = %d", code)
	}

	events := regenerateContent(t, baseURL, c.ID, "sms", "daha kısa")

	done := lastDone(t, events)
	if done.Content.SMS == nil || done.Content.SMS.Body != shorterSMS {
		t.Errorf("SMS Content = %+v, want the regenerated body", done.Content.SMS)
	}
	if done.Content.Email == nil || *done.Content.Email != edited {
		t.Errorf("email Content = %+v, want the Marketer's edit untouched", done.Content.Email)
	}
	if !done.Validation["sms"].Valid || !done.Validation["email"].Valid {
		t.Errorf("validation = %+v, want both Channels valid", done.Validation)
	}
	if !slices.Equal(done.EnabledChannels, []string{"sms", "email"}) {
		t.Errorf("enabled channels = %q, want both still enabled", done.EnabledChannels)
	}
	req := script.requests[len(script.requests)-1]
	if req.Regenerate == nil {
		t.Fatalf("Content call carries no regeneration")
	}
	if req.Regenerate.Instruction != "daha kısa" || req.Regenerate.Channel != "sms" {
		t.Errorf("regeneration = %+v, want the instruction for the SMS Channel", req.Regenerate)
	}
	if req.Regenerate.Current != `{"body":"`+originalSMS+`"}` {
		t.Errorf("current output = %q, want the stored SMS Content", req.Regenerate.Current)
	}
	if req.Brief != exampleBrief || !slices.Equal(req.AudienceLabels, []string{"Şehir: Ankara"}) {
		t.Errorf("request = %+v, want the Brief and Audience", req)
	}

	// Without an instruction, the regenerated SMS is the current output and
	// the email edit still stands.
	again := lastDone(t, regenerateContent(t, baseURL, c.ID, "sms", ""))
	if again.Content.Email == nil || *again.Content.Email != edited {
		t.Errorf("email Content after a second regeneration = %+v, want the edit", again.Content.Email)
	}
	if r := script.requests[len(script.requests)-1].Regenerate; r == nil || r.Instruction != "" ||
		r.Current != `{"body":"`+shorterSMS+`"}` {
		t.Errorf("second regeneration = %+v, want no instruction and the regenerated SMS as current", r)
	}
}

func TestRegeneratingTheEmailIgnoresWhateverClaudeWritesForTheSMS(t *testing.T) {
	newEmail := `{"subject": "Yeni konu", "preheader": "Yeni ön izleme", "body": "Yeni gövde."}`
	// Claude also rewrites the SMS, too long: that SMS was not asked for.
	answer := `{"channels": ["sms", "email"], "sms": {"body": "` + strings.Repeat("a", 200) + `"}, "email": ` + newEmail + `}`
	baseURL, c, script := draftWithContent(t, answer)

	events := regenerateContent(t, baseURL, c.ID, "email", "")

	if n := repairNotices(t, events); len(n) != 0 {
		t.Errorf("repair notices = %+v, want none: the SMS was not regenerated", n)
	}
	deltas := 0
	for _, e := range events {
		if e.Name != "content.delta" {
			continue
		}
		deltas++
		var d contentDelta
		if err := json.Unmarshal(e.Data, &d); err != nil || d.SMS != nil {
			t.Fatalf("delta %s, want email Content only", e.Data)
		}
	}
	if deltas == 0 {
		t.Errorf("events = %q, want the email to stream", eventNames(events))
	}
	done := lastDone(t, events)
	if done.Content.Email == nil || done.Content.Email.Subject != "Yeni konu" {
		t.Errorf("email Content = %+v, want the regenerated one", done.Content.Email)
	}
	if done.Content.SMS == nil || done.Content.SMS.Body != originalSMS {
		t.Errorf("SMS Content = %+v, want the original", done.Content.SMS)
	}
	if r := script.requests[len(script.requests)-1].Regenerate; r == nil || r.Channel != "email" ||
		!strings.Contains(r.Current, `"subject":"{{first_name}}, sepetin seni bekliyor"`) {
		t.Errorf("regeneration = %+v, want the email Channel with its current Content", r)
	}
}

func TestRegeneratedContentGoesThroughTheRepairLoop(t *testing.T) {
	tooLong := strings.Repeat("a", 172)
	baseURL, c, script := draftWithContent(t, smsOnly(tooLong), smsOnly(shorterSMS))

	events := regenerateContent(t, baseURL, c.ID, "sms", "daha kısa")

	if done := lastDone(t, events); done.Content.SMS == nil || done.Content.SMS.Body != shorterSMS {
		t.Errorf("SMS Content = %+v, want the repaired body", done.Content.SMS)
	}
	if n := repairNotices(t, events); len(n) != 1 || !strings.Contains(strings.Join(n[0].Errors, ""), "sms.body") {
		t.Fatalf("repair notices = %+v, want one about the SMS body", n)
	}
	repaired := script.requests[len(script.requests)-1]
	if repaired.Repair == nil || repaired.Repair.Previous != smsOnly(tooLong) {
		t.Errorf("repair = %+v, want the rejected SMS", repaired.Repair)
	}
	if repaired.Regenerate == nil || repaired.Regenerate.Instruction != "daha kısa" || repaired.Regenerate.Channel != "sms" {
		t.Errorf("repair call regeneration = %+v, want the instruction kept", repaired.Regenerate)
	}
}

func TestRegenerationStillInvalidAfterTwoRepairsKeepsTheCurrentContent(t *testing.T) {
	// Asked for the SMS, Claude keeps writing only an email.
	emailOnly := `{"channels": ["email"], "email": {"subject": "s", "preheader": "p", "body": "b"}}`
	baseURL, c, _ := draftWithContent(t, emailOnly)

	events := regenerateContent(t, baseURL, c.ID, "sms", "")

	last := events[len(events)-1]
	if last.Name != "error" || !strings.Contains(string(last.Data), `"retryable":true`) {
		t.Fatalf("last event = %s %s, want a retryable error", last.Name, last.Data)
	}
	if n := repairNotices(t, events); len(n) != 2 || !strings.Contains(strings.Join(n[0].Errors, ""), "sms Channel is missing") {
		t.Errorf("repair notices = %+v, want two reporting the missing SMS", n)
	}
	// Regenerating the email now shows the SMS was left as it was.
	done := lastDone(t, regenerateContent(t, baseURL, c.ID, "email", ""))
	if done.Content.SMS == nil || done.Content.SMS.Body != originalSMS {
		t.Errorf("SMS Content = %+v, want the original after the failed regeneration", done.Content.SMS)
	}
}

func TestRegeneratingAnUnknownOrNonDraftCampaignIsRejected(t *testing.T) {
	srv, pool := startAPIWithPool(t, httpapi.Options{LLM: llm.NewMock()})
	c := createCampaign(t, srv.URL, exampleBrief)

	for _, tc := range []struct {
		url  string
		want int
	}{
		{contentURL(srv.URL, 999999, "sms") + "/regenerate", http.StatusNotFound},
		{contentURL(srv.URL, c.ID, "push") + "/regenerate", http.StatusNotFound},
		{segmentURL(srv.URL, 999999), http.StatusNotFound},
	} {
		var body struct{ Error string }
		if code := postJSON(t, tc.url, map[string]string{}, &body); code != tc.want || body.Error == "" {
			t.Errorf("POST %s: status code = %d, want %d with an error", tc.url, code, tc.want)
		}
	}
	if _, err := pool.Exec(context.Background(), `UPDATE campaigns SET status = 'approved' WHERE id = $1`, c.ID); err != nil {
		t.Fatal(err)
	}
	for _, url := range []string{contentURL(srv.URL, c.ID, "sms") + "/regenerate", segmentURL(srv.URL, c.ID)} {
		var body struct{ Error string }
		if code := postJSON(t, url, map[string]string{"instruction": "daha kısa"}, &body); code != http.StatusConflict {
			t.Errorf("POST %s: status code = %d, want 409", url, code)
		}
	}
}

type regeneratedSegment struct {
	createdCampaign
	EnabledChannels []string `json:"enabled_channels"`
	Content         struct {
		SMS   *smsContent   `json:"sms"`
		Email *emailContent `json:"email"`
	} `json:"content"`
	Validation map[string]validation `json:"validation"`
}

func TestRegeneratingTheSegmentRuleRevalidatesTheEditedContentWithoutRegeneratingIt(t *testing.T) {
	konya := `{"rule": {"all": [` + konyaRule + `]}, "unsupported_criteria": []}`
	diyarbakir := `{"rule": {"all": [` + diyarbakirRule + `]}, "unsupported_criteria": ["Cinsiyet: kadın"]}`
	segments := &segmentScript{answers: []string{konya, diyarbakir}}
	contents := &contentScript{answers: []string{bothChannels}}
	srv := startAPI(t, httpapi.Options{LLM: &llm.Mock{SegmentFunc: segments.call, ContentFunc: contents.call}})
	c := createCampaign(t, srv.URL, exampleBrief)
	lastDone(t, streamContent(t, srv.URL, c.ID))
	// Fits Konya (5 letters) exactly, but not Diyarbakır (10).
	edit := map[string]string{"body": "{{city}}" + strings.Repeat("a", 155)}
	var saved savedContent
	if putJSON(t, contentURL(srv.URL, c.ID, "sms"), edit, &saved); !saved.Validation.Valid {
		t.Fatalf("edit validation = %+v, want valid for Konya", saved.Validation)
	}

	var got regeneratedSegment
	if code := postJSON(t, segmentURL(srv.URL, c.ID), map[string]string{"instruction": "sadece kadınlar"}, &got); code != http.StatusOK {
		t.Fatalf("status code = %d, want 200", code)
	}

	if !slices.Equal(got.Segment.Labels, []string{"Şehir: Diyarbakır"}) {
		t.Errorf("labels = %q, want the new Segment Rule's", got.Segment.Labels)
	}
	if !slices.Equal(got.Segment.UnsupportedCriteria, []string{"Cinsiyet: kadın"}) {
		t.Errorf("unsupported criteria = %q, want the new warning", got.Segment.UnsupportedCriteria)
	}
	if got.Audience == c.Audience || got.Audience.SMS == 0 {
		t.Errorf("audience = %+v, want Diyarbakır's counts, not Konya's %+v", got.Audience, c.Audience)
	}
	if got.Content.SMS == nil || got.Content.SMS.Body != edit["body"] {
		t.Errorf("SMS Content = %+v, want the Marketer's edit kept", got.Content.SMS)
	}
	sms := got.Validation["sms"]
	if sms.Valid || sms.Usage["body"].Used != 165 || len(sms.Issues) == 0 {
		t.Errorf("SMS validation = %+v, want it flagged at 165/160 for Diyarbakır", sms)
	}
	if got.Content.Email == nil || !got.Validation["email"].Valid {
		t.Errorf("email = %+v / %+v, want the untouched, still valid email", got.Content.Email, got.Validation["email"])
	}
	if len(contents.requests) != 1 {
		t.Errorf("Content calls = %d, want only the first: Content is never regenerated", len(contents.requests))
	}
	req := segments.requests[len(segments.requests)-1]
	if req.Regenerate == nil || req.Regenerate.Instruction != "sadece kadınlar" || req.Brief != exampleBrief {
		t.Fatalf("Segment request = %+v, want the Brief and the instruction", req)
	}
	if !strings.Contains(req.Regenerate.Current, `"value":"Konya"`) {
		t.Errorf("current output = %q, want the current Konya rule", req.Regenerate.Current)
	}

	// The edit validates against the new Audience from now on.
	if v := validateContent(t, srv.URL, c.ID, "sms", edit); v.Valid {
		t.Errorf("validation after the change = %+v, want invalid", v)
	}
}

func TestSegmentRuleRegenerationGoesThroughTheRepairLoop(t *testing.T) {
	segments := &segmentScript{answers: []string{ankaraSegment, unknownFieldSegment, ankaraSegment}}
	srv := startAPI(t, httpapi.Options{LLM: &llm.Mock{SegmentFunc: segments.call}})
	c := createCampaign(t, srv.URL, exampleBrief)

	var got regeneratedSegment
	if code := postJSON(t, segmentURL(srv.URL, c.ID), map[string]string{}, &got); code != http.StatusOK {
		t.Fatalf("status code = %d, want 200", code)
	}
	if got.Repairs != 1 {
		t.Errorf("repairs = %d, want 1", got.Repairs)
	}
	if last := segments.requests[len(segments.requests)-1]; last.Repair == nil || last.Regenerate == nil {
		t.Errorf("repair call = %+v, want both the repair and the regeneration", last)
	}
}

func TestSegmentRuleRegenerationThatCannotBeRepairedKeepsTheCurrentRule(t *testing.T) {
	segments := &segmentScript{answers: []string{ankaraSegment, unknownFieldSegment}}
	srv, pool := startAPIWithPool(t, httpapi.Options{LLM: &llm.Mock{SegmentFunc: segments.call}})
	c := createCampaign(t, srv.URL, exampleBrief)

	var body struct {
		Error     string `json:"error"`
		Retryable bool   `json:"retryable"`
	}
	if code := postJSON(t, segmentURL(srv.URL, c.ID), map[string]string{"instruction": "x"}, &body); code != http.StatusBadGateway || !body.Retryable {
		t.Fatalf("status code = %d, body %+v; want a retryable 502", code, body)
	}
	if len(segments.requests) != 4 {
		t.Errorf("Segment calls = %d, want the first + 1 + 2 repairs", len(segments.requests))
	}
	var rule string
	queryRow(t, pool, `SELECT segment_rule::text FROM campaigns WHERE id = $1`, []any{c.ID}, &rule)
	if !strings.Contains(rule, "Ankara") {
		t.Errorf("stored rule = %s, want the Ankara rule kept", rule)
	}
}

func TestTheDefaultMockRegeneratesValidContentAndSegmentRules(t *testing.T) {
	srv := startAPI(t, httpapi.Options{LLM: llm.NewMock()})
	c := createCampaign(t, srv.URL, exampleBrief)
	first := lastDone(t, streamContent(t, srv.URL, c.ID))

	for _, tc := range []struct{ channel, instruction string }{{"sms", "daha kısa"}, {"email", "daha kısa"}, {"email", ""}, {"sms", ""}} {
		done := lastDone(t, regenerateContent(t, srv.URL, c.ID, tc.channel, tc.instruction))
		if !done.Validation[tc.channel].Valid {
			t.Errorf("%s %q: validation = %+v, want valid", tc.channel, tc.instruction, done.Validation[tc.channel])
		}
	}
	var got regeneratedSegment
	if code := postJSON(t, segmentURL(srv.URL, c.ID), map[string]string{"instruction": "sadece Ankara"}, &got); code != http.StatusOK {
		t.Fatalf("segment: status code = %d, want 200", code)
	}
	if !slices.Contains(got.Segment.Labels, "Şehir: Ankara") || got.Repairs != 0 {
		t.Errorf("segment = %+v, want Ankara without repairs", got.Segment)
	}
	if got.Content.SMS == nil || got.Content.SMS.Body == first.Content.SMS.Body {
		t.Errorf("SMS Content = %+v, want the regenerated SMS kept", got.Content.SMS)
	}
}
