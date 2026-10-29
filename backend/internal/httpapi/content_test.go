package httpapi_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"

	"campaigncomposer/internal/httpapi"
	"campaigncomposer/internal/llm"
)

type sseEvent struct {
	Name string
	Data json.RawMessage
}

// readSSE reads a whole text/event-stream response into its events.
func readSSE(t *testing.T, r io.Reader) []sseEvent {
	t.Helper()
	var events []sseEvent
	var cur sseEvent
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			if cur.Name != "" || cur.Data != nil {
				events = append(events, cur)
			}
			cur = sseEvent{}
		case strings.HasPrefix(line, "event: "):
			cur.Name = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			cur.Data = append(cur.Data, strings.TrimPrefix(line, "data: ")...)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("read SSE: %v", err)
	}
	return events
}

type smsContent struct {
	Body string `json:"body"`
}

type emailContent struct {
	Subject   string `json:"subject"`
	Preheader string `json:"preheader"`
	Body      string `json:"body"`
	CTAText   string `json:"cta_text"`
}

type issue struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type contentDone struct {
	EnabledChannels []string `json:"enabled_channels"`
	Content         struct {
		SMS   *smsContent   `json:"sms"`
		Email *emailContent `json:"email"`
	} `json:"content"`
	Validation map[string]struct {
		Valid  bool    `json:"valid"`
		Issues []issue `json:"issues"`
	} `json:"validation"`
}

type contentDelta struct {
	SMS   *smsContent   `json:"sms"`
	Email *emailContent `json:"email"`
}

// createCampaign creates a Campaign from brief and fails the test unless it is created.
func createCampaign(t *testing.T, baseURL, brief string) campaign {
	t.Helper()
	var c campaign
	segmentDone(t, streamSegment(t, baseURL+"/api/campaigns", map[string]string{"brief": brief}), &c)
	return c
}

// streamContent asks for the Campaign's Content and returns every SSE event.
func streamContent(t *testing.T, baseURL string, id int64) []sseEvent {
	t.Helper()
	res, err := http.Post(baseURL+"/api/campaigns/"+strconv.FormatInt(id, 10)+"/content", "application/json", nil)
	if err != nil {
		t.Fatalf("POST content: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST content: status code = %d, want 200", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}
	return readSSE(t, res.Body)
}

// lastDone decodes the stream's final content.done event.
func lastDone(t *testing.T, events []sseEvent) contentDone {
	t.Helper()
	if len(events) == 0 || events[len(events)-1].Name != "content.done" {
		t.Fatalf("stream did not end with content.done: %s", eventNames(events))
	}
	var d contentDone
	if err := json.Unmarshal(events[len(events)-1].Data, &d); err != nil {
		t.Fatalf("decode content.done: %v", err)
	}
	return d
}

func eventNames(events []sseEvent) []string {
	names := make([]string, len(events))
	for i, e := range events {
		names[i] = e.Name
	}
	return names
}

// scriptedContent returns a mock LLM whose Content call streams chunks.
func scriptedContent(chunks ...string) *llm.Mock {
	return &llm.Mock{ContentFunc: func(_ context.Context, _ llm.ContentRequest, emit func(string)) error {
		for _, c := range chunks {
			emit(c)
		}
		return nil
	}}
}

func TestExampleBriefContentStreamsProgressivelyForBothChannels(t *testing.T) {
	srv := startAPI(t, httpapi.Options{MockLLM: true, LLM: llm.NewMock()})
	c := createCampaign(t, srv.URL, exampleBrief)

	events := streamContent(t, srv.URL, c.ID)

	// The SMS body arrives in growing prefixes before the stream ends.
	var smsBodies []string
	for _, e := range events {
		if e.Name != "content.delta" {
			continue
		}
		var d contentDelta
		if err := json.Unmarshal(e.Data, &d); err != nil {
			t.Fatalf("decode content.delta: %v", err)
		}
		if d.SMS != nil && d.SMS.Body != "" && (len(smsBodies) == 0 || smsBodies[len(smsBodies)-1] != d.SMS.Body) {
			smsBodies = append(smsBodies, d.SMS.Body)
		}
	}
	if len(smsBodies) < 3 {
		t.Fatalf("saw %d distinct partial SMS bodies, want the body to stream in several steps", len(smsBodies))
	}

	done := lastDone(t, events)
	final := done.Content.SMS
	if final == nil || final.Body == "" {
		t.Fatalf("final SMS Content is empty")
	}
	for _, partial := range smsBodies[:len(smsBodies)-1] {
		if !strings.HasPrefix(final.Body, partial) {
			t.Errorf("partial SMS body %q is not a prefix of the final body %q", partial, final.Body)
		}
	}
	if !slices.Equal(done.EnabledChannels, []string{"sms", "email"}) {
		t.Errorf("enabled_channels = %q, want [sms email]", done.EnabledChannels)
	}
	if e := done.Content.Email; e == nil || e.Subject == "" || e.Preheader == "" || e.Body == "" {
		t.Errorf("email Content = %+v, want subject, preheader and body", e)
	}
	for _, ch := range []string{"sms", "email"} {
		if v := done.Validation[ch]; !v.Valid {
			t.Errorf("%s validation = %+v, want valid", ch, v)
		}
	}
	// A Turkish Brief yields Turkish copy.
	if !strings.Contains(done.Content.Email.Subject+done.Content.Email.Body, "indirim") {
		t.Errorf("email Content is not Turkish: %+v", done.Content.Email)
	}
}

func TestOnlyTheChannelsClaudeInfersAreEnabled(t *testing.T) {
	m := scriptedContent(`{"channels": ["sms"], "sms": {"bo`, `dy": "Merhaba {{first_name}}"}, "email": {"subject": "x", "preheader": "y", "body": "z"}}`)
	srv := startAPI(t, httpapi.Options{LLM: m})
	c := createCampaign(t, srv.URL, "Sadece SMS gönder")

	done := lastDone(t, streamContent(t, srv.URL, c.ID))

	if !slices.Equal(done.EnabledChannels, []string{"sms"}) {
		t.Errorf("enabled_channels = %q, want [sms]", done.EnabledChannels)
	}
	if done.Content.Email != nil {
		t.Errorf("email Content = %+v, want none for an SMS-only Brief", done.Content.Email)
	}
	if _, ok := done.Validation["email"]; ok {
		t.Errorf("validation has an email entry for a Channel Claude did not choose")
	}
	if done.Content.SMS == nil || done.Content.SMS.Body != "Merhaba {{first_name}}" {
		t.Errorf("SMS Content = %+v, want the streamed body", done.Content.SMS)
	}
}

func TestContentOverTheLimitsOrWithUnknownPlaceholdersIsSentBackWithEveryIssue(t *testing.T) {
	invalid := `{"channels": ["sms", "email"], "sms": {"body": "Merhaba {{name}}"}, "email": {"subject": "` +
		strings.Repeat("a", 81) + `", "preheader": "` + strings.Repeat("b", 121) +
		`", "body": "Merhaba {{last_name}}, {{city}} {{first_name}}"}}`
	script := &contentScript{answers: []string{invalid, smsOnly("Merhaba {{first_name}}")}}
	baseURL := repairAPI(t, script)
	c := createCampaign(t, baseURL, exampleBrief)

	events := streamContent(t, baseURL, c.ID)

	lastDone(t, events)
	notices := repairNotices(t, events)
	if len(notices) != 1 {
		t.Fatalf("repair notices = %+v, want one", notices)
	}
	perField := map[string][]string{}
	for _, e := range notices[0].Errors {
		field, msg, _ := strings.Cut(e, ": ")
		perField[field] = append(perField[field], msg)
	}
	for field, want := range map[string]string{
		"email.subject":   "80",
		"email.preheader": "120",
		"email.body":      "{{last_name}}",
		"sms.body":        "{{name}}",
	} {
		if msgs := perField[field]; len(msgs) != 1 || !strings.Contains(msgs[0], want) {
			t.Errorf("%s issues = %q, want one naming %s", field, msgs, want)
		}
	}
	if !slices.Equal(script.requests[1].Repair.Errors, notices[0].Errors) {
		t.Errorf("repair prompt errors = %q, want the notice's %q", script.requests[1].Repair.Errors, notices[0].Errors)
	}
}

func TestEmailBodyLimitIsFifteenHundredCharacters(t *testing.T) {
	atLimit := strings.Repeat("ş", 1500)
	for _, tc := range []struct {
		body    string
		repairs int
	}{{atLimit, 0}, {atLimit + "ş", 1}} {
		email := `{"channels": ["email"], "email": {"subject": "s", "preheader": "p", "body": "` + tc.body + `"}}`
		valid := `{"channels": ["email"], "email": {"subject": "s", "preheader": "p", "body": "b"}}`
		baseURL := repairAPI(t, &contentScript{answers: []string{email, valid}})
		c := createCampaign(t, baseURL, exampleBrief)

		events := streamContent(t, baseURL, c.ID)
		lastDone(t, events)
		notices := repairNotices(t, events)
		if len(notices) != tc.repairs {
			t.Errorf("body of %d characters: %d repairs, want %d", len([]rune(tc.body)), len(notices), tc.repairs)
		}
		if tc.repairs > 0 && !strings.Contains(strings.Join(notices[0].Errors, ""), "1500") {
			t.Errorf("repair errors = %q, want the 1500 character limit", notices[0].Errors)
		}
	}
}

func TestContentOfAnUnknownCampaignIsNotFound(t *testing.T) {
	srv := startAPI(t, httpapi.Options{})

	res, err := http.Post(srv.URL+"/api/campaigns/999999/content", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status code = %d, want 404", res.StatusCode)
	}
}

func TestEnglishBriefYieldsEnglishContent(t *testing.T) {
	srv := startAPI(t, httpapi.Options{LLM: llm.NewMock()})
	c := createCampaign(t, srv.URL, "Remind shoppers in İstanbul who added to cart in the last 30 days but did not buy. Send an email with 30% off.")

	done := lastDone(t, streamContent(t, srv.URL, c.ID))

	if !slices.Equal(done.EnabledChannels, []string{"email"}) {
		t.Errorf("enabled_channels = %q, want [email]", done.EnabledChannels)
	}
	if e := done.Content.Email; e == nil || !strings.Contains(e.Body, "cart") {
		t.Errorf("email Content = %+v, want English copy", e)
	}
}

func putJSON(t *testing.T, url string, body, into any) int {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT %s: %v", url, err)
	}
	defer res.Body.Close()
	if err := json.NewDecoder(res.Body).Decode(into); err != nil {
		t.Fatalf("decode %s: %v", url, err)
	}
	return res.StatusCode
}

type channels struct {
	EnabledChannels []string `json:"enabled_channels"`
}

func TestMarketerCanToggleEachChannel(t *testing.T) {
	srv := startAPI(t, httpapi.Options{LLM: llm.NewMock()})
	c := createCampaign(t, srv.URL, exampleBrief)
	lastDone(t, streamContent(t, srv.URL, c.ID))
	base := srv.URL + "/api/campaigns/" + strconv.FormatInt(c.ID, 10) + "/channels/"

	var got channels
	if code := putJSON(t, base+"email", map[string]bool{"enabled": false}, &got); code != http.StatusOK {
		t.Fatalf("disable email: status code = %d, want 200", code)
	}
	if !slices.Equal(got.EnabledChannels, []string{"sms"}) {
		t.Errorf("after disabling email: enabled_channels = %q, want [sms]", got.EnabledChannels)
	}
	putJSON(t, base+"sms", map[string]bool{"enabled": false}, &got)
	if len(got.EnabledChannels) != 0 {
		t.Errorf("after disabling both: enabled_channels = %q, want none", got.EnabledChannels)
	}
	putJSON(t, base+"email", map[string]bool{"enabled": true}, &got)
	putJSON(t, base+"sms", map[string]bool{"enabled": true}, &got)
	if !slices.Equal(got.EnabledChannels, []string{"sms", "email"}) {
		t.Errorf("after enabling both: enabled_channels = %q, want [sms email]", got.EnabledChannels)
	}

	var body struct{ Error string }
	if code := putJSON(t, base+"push", map[string]bool{"enabled": true}, &body); code != http.StatusNotFound {
		t.Errorf("unknown Channel: status code = %d, want 404", code)
	}
}

func TestEmailPreviewRendersTheFixedTemplateWithSampleValues(t *testing.T) {
	srv := startAPI(t, httpapi.Options{})

	b, _ := json.Marshal(map[string]string{
		"subject":   "{{first_name}}, indirim seni bekliyor",
		"preheader": "Sepetin hâlâ orada",
		"body":      "Merhaba {{first_name}},\n\n{{city}} için <script>alert(1)</script> fırsat.\nKaçırma!",
		"cta_text":  "Siparişi tamamla",
	})
	res, err := http.Post(srv.URL+"/api/email/preview", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	html, _ := io.ReadAll(res.Body)
	page := string(html)

	if res.StatusCode != http.StatusOK || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/html") {
		t.Fatalf("status %d, Content-Type %q, want 200 text/html", res.StatusCode, res.Header.Get("Content-Type"))
	}
	for _, want := range []string{
		"<title>Ayşe, indirim seni bekliyor</title>",
		"Sepetin hâlâ orada",
		"<p>Merhaba Ayşe,</p>",
		"<p>İstanbul için &lt;script&gt;alert(1)&lt;/script&gt; fırsat.<br>Kaçırma!</p>",
		"Siparişi tamamla",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("preview does not contain %q:\n%s", want, page)
		}
	}
	if strings.Contains(page, "{{") || strings.Contains(page, "<script>") {
		t.Errorf("preview leaks a placeholder or raw HTML:\n%s", page)
	}
}

func TestEmailPreviewOmitsTheButtonWithoutCTAText(t *testing.T) {
	srv := startAPI(t, httpapi.Options{})

	b, _ := json.Marshal(map[string]string{"subject": "s", "preheader": "p", "body": "b"})
	res, err := http.Post(srv.URL+"/api/email/preview", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	html, _ := io.ReadAll(res.Body)
	if strings.Contains(string(html), `data-cta`) {
		t.Errorf("preview has a CTA button without CTA text:\n%s", html)
	}
}
