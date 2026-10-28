package httpapi_test

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"

	"campaigncomposer/internal/httpapi"
	"campaigncomposer/internal/llm"

	"github.com/jackc/pgx/v5/pgxpool"
)

// assertBlocked checks that approval was refused with exactly the given
// reasons (code/channel), each explained, and that nothing was frozen.
func assertBlocked(t *testing.T, pool *pgxpool.Pool, id int64, code int, a approval, want ...string) {
	t.Helper()
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("approve: status code %d, body %+v; want 422", code, a)
	}
	var got []string
	for _, r := range a.Reasons {
		key := r.Code
		if r.Channel != "" {
			key += "/" + r.Channel
		}
		got = append(got, key)
		if strings.TrimSpace(r.Message) == "" {
			t.Errorf("reason %s has no message", key)
		}
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("reasons = %v, want %v", got, want)
	}
	var status string
	var n int
	queryRow(t, pool, `SELECT status, (SELECT count(*) FROM deliveries WHERE campaign_id = $1) FROM campaigns WHERE id = $1`,
		[]any{id}, &status, &n)
	if status != "draft" || n != 0 {
		t.Errorf("after a refused approval: status %q, %d Deliveries; want draft, 0", status, n)
	}
}

func TestApprovalIsRefusedWhileAnEnabledChannelHasInvalidContent(t *testing.T) {
	baseURL, pool, c := reviewedCampaign(t)
	var saved savedContent
	putJSON(t, contentURL(baseURL, c.ID, "sms"), map[string]string{"body": "Merhaba {{soyad}}"}, &saved)

	code, a := approve(t, baseURL, c.ID)

	assertBlocked(t, pool, c.ID, code, a, "invalid_content/sms")
}

func TestApprovalIsRefusedWhenAnEnabledChannelHasNoContent(t *testing.T) {
	srv, pool := startAPIWithPool(t, httpapi.Options{LLM: scriptedContent(`{"channels": ["sms"], "sms": {"body": "Merhaba {{first_name}}"}}`)})
	c := createCampaign(t, srv.URL, exampleBrief)
	lastDone(t, streamContent(t, srv.URL, c.ID))
	var body map[string]any
	putJSON(t, srv.URL+"/api/campaigns/"+strconv.FormatInt(c.ID, 10)+"/channels/email", map[string]bool{"enabled": true}, &body)

	code, a := approve(t, srv.URL, c.ID)

	assertBlocked(t, pool, c.ID, code, a, "invalid_content/email")
}

func TestApprovalIsRefusedWhenAnEnabledChannelsAudienceHasBecomeEmpty(t *testing.T) {
	baseURL, pool, c := reviewedCampaign(t)
	exec(t, pool, `UPDATE recipients SET sms_consent = false`)

	code, a := approve(t, baseURL, c.ID)

	assertBlocked(t, pool, c.ID, code, a, "empty_audience/sms")
}

func TestApprovalIsRefusedWithNoChannelEnabled(t *testing.T) {
	baseURL, pool, c := reviewedCampaign(t)
	var body map[string]any
	for _, ch := range []string{"sms", "email"} {
		putJSON(t, baseURL+"/api/campaigns/"+strconv.FormatInt(c.ID, 10)+"/channels/"+ch, map[string]bool{"enabled": false}, &body)
	}

	code, a := approve(t, baseURL, c.ID)

	assertBlocked(t, pool, c.ID, code, a, "no_channel")
}

func TestApprovalListsEveryReasonItIsRefused(t *testing.T) {
	baseURL, pool, c := reviewedCampaign(t)
	var saved savedContent
	putJSON(t, contentURL(baseURL, c.ID, "sms"), map[string]string{"body": ""}, &saved)
	exec(t, pool, `UPDATE recipients SET email_consent = false`)

	code, a := approve(t, baseURL, c.ID)

	assertBlocked(t, pool, c.ID, code, a, "invalid_content/sms", "empty_audience/email")
}

func TestAChannelWithNoConsentingRecipientsIsDisabledWithAnExplanation(t *testing.T) {
	srv, pool := startAPIWithPool(t, httpapi.Options{LLM: &llm.Mock{
		SegmentFunc: scriptedSegment(`{"rule": ` + konyaRule + `, "unsupported_criteria": []}`).SegmentFunc,
	}})
	exec(t, pool, `UPDATE recipients SET sms_consent = false WHERE city = 'Konya'`)

	var c struct {
		ID               int64             `json:"id"`
		DisabledChannels map[string]string `json:"disabled_channels"`
		EnabledChannels  []string          `json:"enabled_channels"`
	}
	segmentDone(t, streamSegment(t, srv.URL+"/api/campaigns", map[string]string{"brief": exampleBrief}), &c)
	if c.DisabledChannels["sms"] == "" || c.DisabledChannels["email"] != "" || strings.Join(c.EnabledChannels, ",") != "email" {
		t.Errorf("campaign disabled %v, enabled %v; want only SMS disabled, with an explanation", c.DisabledChannels, c.EnabledChannels)
	}

	// Claude writes both Channels, but SMS stays off.
	if done := lastDone(t, streamContent(t, srv.URL, c.ID)); strings.Join(done.EnabledChannels, ",") != "email" {
		t.Errorf("enabled Channels after Content = %v, want [email]", done.EnabledChannels)
	}
	var body struct{ Error string }
	code := putJSON(t, srv.URL+"/api/campaigns/"+strconv.FormatInt(c.ID, 10)+"/channels/sms", map[string]bool{"enabled": true}, &body)
	if code != http.StatusConflict || body.Error == "" {
		t.Errorf("enable SMS: status code %d, error %q; want 409 with an explanation", code, body.Error)
	}

	code, a := approve(t, srv.URL, c.ID)
	if code != http.StatusOK || a.Deliveries.SMS != 0 || a.Deliveries.Email == 0 {
		t.Errorf("approve: status code %d, deliveries %+v; want 200 with email Deliveries only", code, a.Deliveries)
	}
}

func TestApprovalIsRefusedWhileContentIsBeingGenerated(t *testing.T) {
	release := make(chan struct{})
	releaseOnce := sync.OnceFunc(func() { close(release) })
	mock := &llm.Mock{ContentFunc: func(ctx context.Context, req llm.ContentRequest, emit func(string)) error {
		<-release
		return llm.NewMock().Content(ctx, req, emit)
	}}
	srv, pool := startAPIWithPool(t, httpapi.Options{LLM: mock})
	// Cleanups run last-in first-out: the call is released before the server closes.
	t.Cleanup(releaseOnce)
	c := createCampaign(t, srv.URL, exampleBrief)

	// The stream's headers arrive as soon as generation has started.
	res, err := http.Post(srv.URL+"/api/campaigns/"+strconv.FormatInt(c.ID, 10)+"/content", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	code, a := approve(t, srv.URL, c.ID)
	assertBlocked(t, pool, c.ID, code, a, "generating")

	releaseOnce()
	lastDone(t, readSSE(t, res.Body))
	if code, a := approve(t, srv.URL, c.ID); code != http.StatusOK {
		t.Errorf("approve after generation: status code %d, body %+v; want 200", code, a)
	}
}
