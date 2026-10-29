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

	"github.com/jackc/pgx/v5/pgxpool"
)

type approval struct {
	ID         int64  `json:"id"`
	Status     string `json:"status"`
	Deliveries struct {
		SMS   int `json:"sms"`
		Email int `json:"email"`
	} `json:"deliveries"`
	Error   string `json:"error"`
	Reasons []struct {
		Code    string `json:"code"`
		Channel string `json:"channel"`
		Message string `json:"message"`
	} `json:"reasons"`
}

func approve(t *testing.T, baseURL string, id int64) (int, approval) {
	t.Helper()
	var a approval
	code := postJSON(t, baseURL+"/api/campaigns/"+strconv.FormatInt(id, 10)+"/approve", nil, &a)
	return code, a
}

// reviewedCampaign creates the example Campaign and streams its Content, as a
// Marketer does before approving.
func reviewedCampaign(t *testing.T) (baseURL string, pool *pgxpool.Pool, c campaign) {
	t.Helper()
	srv, pool := startAPIWithPool(t, httpapi.Options{LLM: llm.NewMock()})
	c = createCampaign(t, srv.URL, exampleBrief)
	lastDone(t, streamContent(t, srv.URL, c.ID))
	return srv.URL, pool, c
}

func deliveryCount(t *testing.T, pool *pgxpool.Pool, id int64, channel string) int {
	t.Helper()
	var n int
	queryRow(t, pool, `SELECT count(*) FROM deliveries WHERE campaign_id = $1 AND channel = $2`, []any{id, channel}, &n)
	return n
}

func exec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

func TestApprovalFreezesTheAudienceIntoOnePendingDeliveryPerRecipientAndChannel(t *testing.T) {
	baseURL, pool, c := reviewedCampaign(t)

	code, a := approve(t, baseURL, c.ID)

	if code != http.StatusOK || a.Status != "approved" {
		t.Fatalf("approve: status code %d, body %+v; want 200, approved", code, a)
	}
	if a.Deliveries.SMS != c.Audience.SMS || a.Deliveries.Email != c.Audience.Email {
		t.Errorf("deliveries = %+v, want the Audience %+v", a.Deliveries, c.Audience)
	}
	if n := deliveryCount(t, pool, c.ID, "sms"); n != c.Audience.SMS {
		t.Errorf("sms Deliveries = %d, want %d", n, c.Audience.SMS)
	}
	if n := deliveryCount(t, pool, c.ID, "email"); n != c.Audience.Email {
		t.Errorf("email Deliveries = %d, want %d", n, c.Audience.Email)
	}
	var notPending, both int
	queryRow(t, pool, `SELECT count(*) FILTER (WHERE state <> 'pending') FROM deliveries WHERE campaign_id = $1`, []any{c.ID}, &notPending)
	if notPending != 0 {
		t.Errorf("%d Deliveries are not pending", notPending)
	}
	// A Recipient who consented to both Channels gets one Delivery on each.
	queryRow(t, pool, `SELECT count(*) FROM (SELECT recipient_id FROM deliveries WHERE campaign_id = $1
		GROUP BY recipient_id HAVING count(*) = 2) t`, []any{c.ID}, &both)
	if both == 0 {
		t.Errorf("no Recipient has both an SMS and an email Delivery")
	}
	var status string
	queryRow(t, pool, `SELECT status FROM campaigns WHERE id = $1`, []any{c.ID}, &status)
	if status != "approved" {
		t.Errorf("stored status = %q, want approved", status)
	}
}

func TestDeliveriesCarryTheContentRenderedForTheirRecipient(t *testing.T) {
	baseURL, pool, c := reviewedCampaign(t)
	exec(t, pool, `UPDATE contents SET body = 'Merhaba {{first_name}} ({{city}})' WHERE campaign_id = $1 AND channel = 'sms'`, c.ID)
	exec(t, pool, `UPDATE contents SET subject = '{{first_name}}, indirim', body = 'Selam {{first_name}}' WHERE campaign_id = $1 AND channel = 'email'`, c.ID)
	// A Recipient with Turkish letters in their name, in both Audiences.
	id := audienceMember(t, pool, "r.sms_consent AND r.email_consent")
	exec(t, pool, `UPDATE recipients SET first_name = 'Şükrü' WHERE id = $1`, id)

	if code, a := approve(t, baseURL, c.ID); code != http.StatusOK {
		t.Fatalf("approve: status code %d, body %+v", code, a)
	}

	var phone, address string
	var smsPayload, emailPayload json.RawMessage
	queryRow(t, pool, `SELECT phone, email FROM recipients WHERE id = $1`, []any{id}, &phone, &address)
	queryRow(t, pool, `SELECT payload FROM deliveries WHERE campaign_id = $1 AND recipient_id = $2 AND channel = 'sms'`,
		[]any{c.ID, id}, &smsPayload)
	queryRow(t, pool, `SELECT payload FROM deliveries WHERE campaign_id = $1 AND recipient_id = $2 AND channel = 'email'`,
		[]any{c.ID, id}, &emailPayload)

	var sms struct{ To, Body string }
	if err := json.Unmarshal(smsPayload, &sms); err != nil {
		t.Fatal(err)
	}
	// The SMS is GSM-7, so Ş and İ are transliterated and ü, being GSM-7, is kept (ADR-0002).
	if sms.To != phone || sms.Body != "Merhaba Sükrü (Istanbul)" {
		t.Errorf("sms payload = %+v, want to %s, body %q", sms, phone, "Merhaba Sükrü (Istanbul)")
	}
	var email struct{ To, Subject, HTML string }
	if err := json.Unmarshal(emailPayload, &email); err != nil {
		t.Fatal(err)
	}
	if email.To != address || email.Subject != "Şükrü, indirim" || !strings.Contains(email.HTML, "Selam Şükrü") {
		t.Errorf("email payload = %+v, want to %s, subject and HTML rendered for Şükrü", email, address)
	}
}

// audienceMember is a Recipient in the example Brief's Segment Rule who
// satisfies consent, an SQL condition over recipients r.
func audienceMember(t *testing.T, pool *pgxpool.Pool, consent string) int64 {
	t.Helper()
	var id int64
	queryRow(t, pool, `SELECT r.id FROM recipients r WHERE `+consent+` AND r.city = 'İstanbul'
		AND EXISTS (SELECT 1 FROM events e WHERE e.recipient_id = r.id AND e.type = 'cart_add' AND e.occurred_at >= now() - interval '30 days')
		AND NOT EXISTS (SELECT 1 FROM events e WHERE e.recipient_id = r.id AND e.type = 'purchase' AND e.occurred_at >= now() - interval '30 days')
		ORDER BY r.id LIMIT 1`, nil, &id)
	return id
}

func TestConsentWithdrawnAfterReviewExcludesTheRecipient(t *testing.T) {
	baseURL, pool, c := reviewedCampaign(t)
	id := audienceMember(t, pool, "r.sms_consent")
	exec(t, pool, `UPDATE recipients SET sms_consent = false WHERE id = $1`, id)

	code, a := approve(t, baseURL, c.ID)

	if code != http.StatusOK || a.Deliveries.SMS != c.Audience.SMS-1 {
		t.Fatalf("approve: status code %d, deliveries %+v; want 200 and %d SMS", code, a.Deliveries, c.Audience.SMS-1)
	}
	var n int
	queryRow(t, pool, `SELECT count(*) FROM deliveries WHERE campaign_id = $1 AND recipient_id = $2 AND channel = 'sms'`,
		[]any{c.ID, id}, &n)
	if n != 0 {
		t.Errorf("the Recipient who withdrew SMS consent has %d SMS Deliveries, want 0", n)
	}
}

func TestRecipientChangesAfterApprovalDoNotChangeTheDeliveries(t *testing.T) {
	baseURL, pool, c := reviewedCampaign(t)
	if code, a := approve(t, baseURL, c.ID); code != http.StatusOK {
		t.Fatalf("approve: status code %d, body %+v", code, a)
	}
	id := audienceMember(t, pool, "r.sms_consent")
	var before json.RawMessage
	queryRow(t, pool, `SELECT payload FROM deliveries WHERE campaign_id = $1 AND recipient_id = $2 AND channel = 'sms'`,
		[]any{c.ID, id}, &before)

	exec(t, pool, `UPDATE recipients SET first_name = 'Değişti', phone = '+900000000000', sms_consent = false WHERE id = $1`, id)
	// Recipients who now match the Segment Rule too.
	exec(t, pool, `UPDATE recipients SET city = 'İstanbul', sms_consent = true WHERE id IN (
		SELECT recipient_id FROM events WHERE type = 'cart_add' AND occurred_at >= now() - interval '10 days'
		EXCEPT SELECT recipient_id FROM events WHERE type = 'purchase')`)

	var after json.RawMessage
	queryRow(t, pool, `SELECT payload FROM deliveries WHERE campaign_id = $1 AND recipient_id = $2 AND channel = 'sms'`,
		[]any{c.ID, id}, &after)
	if string(after) != string(before) {
		t.Errorf("payload changed from %s to %s", before, after)
	}
	if n := deliveryCount(t, pool, c.ID, "sms"); n != c.Audience.SMS {
		t.Errorf("sms Deliveries = %d, want the frozen %d", n, c.Audience.SMS)
	}
}

func TestApprovingTwiceCreatesNoExtraDeliveries(t *testing.T) {
	baseURL, pool, c := reviewedCampaign(t)
	if code, a := approve(t, baseURL, c.ID); code != http.StatusOK {
		t.Fatalf("first approve: status code %d, body %+v", code, a)
	}

	code, a := approve(t, baseURL, c.ID)

	if code != http.StatusConflict || a.Error == "" {
		t.Errorf("second approve: status code %d, body %+v; want 409 with an error", code, a)
	}
	if n := deliveryCount(t, pool, c.ID, "sms") + deliveryCount(t, pool, c.ID, "email"); n != c.Audience.SMS+c.Audience.Email {
		t.Errorf("Deliveries = %d, want %d", n, c.Audience.SMS+c.Audience.Email)
	}
}

func TestConcurrentApprovalsCreateEachDeliveryOnce(t *testing.T) {
	baseURL, pool, c := reviewedCampaign(t)

	const n = 8
	codes := make(chan int, n)
	for range n {
		go func() {
			res, err := http.Post(baseURL+"/api/campaigns/"+strconv.FormatInt(c.ID, 10)+"/approve", "application/json", nil)
			if err != nil {
				codes <- 0
				return
			}
			res.Body.Close()
			codes <- res.StatusCode
		}()
	}
	ok := 0
	for range n {
		switch code := <-codes; code {
		case http.StatusOK:
			ok++
		case http.StatusConflict:
		default:
			t.Errorf("approve: status code %d, want 200 or 409", code)
		}
	}
	if ok != 1 {
		t.Errorf("%d approvals succeeded, want exactly 1", ok)
	}
	if got := deliveryCount(t, pool, c.ID, "sms") + deliveryCount(t, pool, c.ID, "email"); got != c.Audience.SMS+c.Audience.Email {
		t.Errorf("Deliveries = %d, want %d", got, c.Audience.SMS+c.Audience.Email)
	}
}
