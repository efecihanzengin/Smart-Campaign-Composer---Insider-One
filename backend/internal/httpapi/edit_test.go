package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"campaigncomposer/internal/httpapi"
	"campaigncomposer/internal/llm"
)

type length struct {
	Encoding string `json:"encoding"`
	Used     int    `json:"used"`
	Limit    int    `json:"limit"`
}

type validation struct {
	Valid  bool              `json:"valid"`
	Issues []issue           `json:"issues"`
	Usage  map[string]length `json:"usage"`
}

func contentURL(baseURL string, id int64, channel string) string {
	return baseURL + "/api/campaigns/" + strconv.FormatInt(id, 10) + "/content/" + channel
}

// validateContent asks the backend validator about Content for one Channel.
func validateContent(t *testing.T, baseURL string, id int64, channel string, c any) validation {
	t.Helper()
	var v validation
	if code := postJSON(t, contentURL(baseURL, id, channel)+"/validate", c, &v); code != http.StatusOK {
		t.Fatalf("validate %s: status code = %d, want 200", channel, code)
	}
	return v
}

// campaignFor creates a Campaign whose Segment Rule is rule.
func campaignFor(t *testing.T, rule string) (baseURL string, c campaign) {
	t.Helper()
	srv := startAPI(t, httpapi.Options{LLM: scriptedSegment(`{"rule": ` + rule + `, "unsupported_criteria": []}`)})
	return srv.URL, createCampaign(t, srv.URL, exampleBrief)
}

// cut repeats s and cuts it to n characters.
func cut(s string, n int) string {
	return string([]rune(strings.Repeat(s, n/len([]rune(s))+1))[:n])
}

const (
	turkishSMS        = "Sepetindeki ürünler seni bekliyor, alışverişini bugün tamamla! "
	transliteratedSMS = "Sepetindeki ürünler seni bekliyor, alisverisini bugün tamamla! "
	everyoneRule      = `{"field": "language", "op": "in", "value": ["tr", "en"]}`
	konyaRule         = `{"field": "city", "op": "eq", "value": "Konya"}`
	diyarbakirRule    = `{"field": "city", "op": "eq", "value": "Diyarbakır"}`
	konyaOrSamsunRule = `{"field": "city", "op": "in", "value": ["Konya", "Samsun"]}`
)

func TestTurkishLettersLimitAnSMSToSeventyCharactersAndTransliteratedTextTo160(t *testing.T) {
	baseURL, c := campaignFor(t, everyoneRule)

	for _, tc := range []struct {
		name     string
		body     string
		valid    bool
		encoding string
		used     int
	}{
		{"70 characters with Turkish letters", cut(turkishSMS, 70), true, "ucs2", 70},
		{"71 characters with Turkish letters", cut(turkishSMS, 71), false, "ucs2", 71},
		{"the same text transliterated, 160 septets", cut(transliteratedSMS, 160), true, "gsm7", 160},
		{"the same text transliterated, 161 septets", cut(transliteratedSMS, 161), false, "gsm7", 161},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := validateContent(t, baseURL, c.ID, "sms", map[string]string{"body": tc.body})
			u := v.Usage["body"]
			if v.Valid != tc.valid || u.Encoding != tc.encoding || u.Used != tc.used {
				t.Errorf("valid %v, usage %+v; want valid %v, %s %d", v.Valid, u, tc.valid, tc.encoding, tc.used)
			}
			if !tc.valid && (len(v.Issues) != 1 || !strings.Contains(v.Issues[0].Message, strconv.Itoa(u.Limit))) {
				t.Errorf("issues = %+v, want one naming the %d limit", v.Issues, u.Limit)
			}
		})
	}
}

func TestSMSLengthIsCheckedWithTheLongestCityInTheAudience(t *testing.T) {
	body := map[string]string{"body": "{{city}}" + strings.Repeat("a", 150)}
	for _, tc := range []struct {
		rule string
		used int
	}{
		{konyaRule, 155},         // Konya
		{konyaOrSamsunRule, 156}, // Samsun
		{everyoneRule, 160},      // Diyarbakır, the longest seeded city
	} {
		baseURL, c := campaignFor(t, tc.rule)
		v := validateContent(t, baseURL, c.ID, "sms", body)
		if u := v.Usage["body"]; u.Used != tc.used || !v.Valid {
			t.Errorf("rule %s: usage %+v valid %v, want %d septets, valid", tc.rule, u, v.Valid, tc.used)
		}
	}
}

func TestEmailLengthIsCheckedWithTheLongestValuesInTheAudience(t *testing.T) {
	baseURL, c := campaignFor(t, konyaRule)

	subject := "{{city}}" + strings.Repeat("a", 75) // Konya: 5 + 75 = 80
	v := validateContent(t, baseURL, c.ID, "email", map[string]string{"subject": subject, "preheader": "p", "body": "b"})
	if u := v.Usage["subject"]; u.Used != 80 || u.Limit != 80 || !v.Valid {
		t.Errorf("subject usage %+v valid %v, want 80/80 valid", u, v.Valid)
	}
	v = validateContent(t, baseURL, c.ID, "email", map[string]string{"subject": subject + "a", "preheader": "p", "body": "b"})
	if v.Valid {
		t.Errorf("subject of 81 characters after rendering is valid, want invalid")
	}
}

func TestGSM7SMSStaysGSM7ForARecipientInACityWithTurkishLetters(t *testing.T) {
	baseURL, c := campaignFor(t, diyarbakirRule)

	v := validateContent(t, baseURL, c.ID, "sms", map[string]string{"body": "{{city}}" + strings.Repeat("a", 150)})

	if u := v.Usage["body"]; u.Encoding != "gsm7" || u.Used != 160 || !v.Valid {
		t.Errorf("usage %+v valid %v, want GSM-7 160/160 valid", u, v.Valid)
	}
}

func TestStreamedContentReportsUsageForTheCounters(t *testing.T) {
	srv := startAPI(t, httpapi.Options{LLM: llm.NewMock()})
	c := createCampaign(t, srv.URL, exampleBrief)

	events := streamContent(t, srv.URL, c.ID)
	lastDone(t, events)

	var done struct {
		Validation struct {
			SMS   validation `json:"sms"`
			Email validation `json:"email"`
		} `json:"validation"`
	}
	if err := json.Unmarshal(events[len(events)-1].Data, &done); err != nil {
		t.Fatal(err)
	}
	v := done.Validation
	if u := v.SMS.Usage["body"]; u.Encoding != "gsm7" || u.Limit != 160 || u.Used == 0 {
		t.Errorf("sms usage = %+v, want GSM-7 out of 160", u)
	}
	for field, limit := range map[string]int{"subject": 80, "preheader": 120, "body": 1500} {
		if u := v.Email.Usage[field]; u.Limit != limit || u.Used == 0 {
			t.Errorf("email %s usage = %+v, want some of %d", field, u, limit)
		}
	}
}

type savedContent struct {
	Content    map[string]string `json:"content"`
	Validation validation        `json:"validation"`
}

func TestMarketerEditsAreSavedAndValidatedByTheSameRules(t *testing.T) {
	srv, pool := startAPIWithPool(t, httpapi.Options{LLM: llm.NewMock()})
	c := createCampaign(t, srv.URL, exampleBrief)
	lastDone(t, streamContent(t, srv.URL, c.ID))

	var got savedContent
	edit := map[string]string{"body": "Merhaba {{first_name}}, " + cut(turkishSMS, 70)}
	if code := putJSON(t, contentURL(srv.URL, c.ID, "sms"), edit, &got); code != http.StatusOK {
		t.Fatalf("PUT sms: status code = %d, want 200", code)
	}
	if got.Content["body"] != edit["body"] {
		t.Errorf("saved body = %q, want %q", got.Content["body"], edit["body"])
	}
	if u := got.Validation.Usage["body"]; got.Validation.Valid || u.Encoding != "ucs2" {
		t.Errorf("validation = %+v, want an invalid UCS-2 SMS", got.Validation)
	}
	var stored string
	queryRow(t, pool, `SELECT body FROM contents WHERE campaign_id = $1 AND channel = 'sms'`, []any{c.ID}, &stored)
	if stored != edit["body"] {
		t.Errorf("stored body = %q, want the edit", stored)
	}

	email := map[string]string{"subject": "Konu", "preheader": "Ön izleme", "body": "Merhaba {{first_name}}", "cta_text": "Al"}
	if code := putJSON(t, contentURL(srv.URL, c.ID, "email"), email, &got); code != http.StatusOK || !got.Validation.Valid {
		t.Fatalf("PUT email: status code %d, validation %+v; want 200, valid", code, got.Validation)
	}
	var subject, cta string
	queryRow(t, pool, `SELECT subject, cta_text FROM contents WHERE campaign_id = $1 AND channel = 'email'`, []any{c.ID}, &subject, &cta)
	if subject != "Konu" || cta != "Al" {
		t.Errorf("stored email = %q / %q, want the edit", subject, cta)
	}
}

func TestEditsToAnUnknownCampaignOrChannelAreNotFound(t *testing.T) {
	srv := startAPI(t, httpapi.Options{LLM: llm.NewMock()})
	c := createCampaign(t, srv.URL, exampleBrief)

	var body struct{ Error string }
	if code := putJSON(t, contentURL(srv.URL, 999999, "sms"), map[string]string{"body": "x"}, &body); code != http.StatusNotFound {
		t.Errorf("unknown Campaign: status code = %d, want 404", code)
	}
	if code := putJSON(t, contentURL(srv.URL, c.ID, "push"), map[string]string{"body": "x"}, &body); code != http.StatusNotFound {
		t.Errorf("unknown Channel: status code = %d, want 404", code)
	}
	if code := postJSON(t, contentURL(srv.URL, 999999, "sms")+"/validate", map[string]string{"body": "x"}, &body); code != http.StatusNotFound {
		t.Errorf("validate for unknown Campaign: status code = %d, want 404", code)
	}
}

func TestEditsToANonDraftCampaignAreRejected(t *testing.T) {
	srv, pool := startAPIWithPool(t, httpapi.Options{LLM: llm.NewMock()})
	c := createCampaign(t, srv.URL, exampleBrief)
	if _, err := pool.Exec(context.Background(), `UPDATE campaigns SET status = 'approved' WHERE id = $1`, c.ID); err != nil {
		t.Fatal(err)
	}

	var body struct{ Error string }
	if code := putJSON(t, contentURL(srv.URL, c.ID, "sms"), map[string]string{"body": "x"}, &body); code != http.StatusConflict {
		t.Errorf("status code = %d, want 409", code)
	}
}
