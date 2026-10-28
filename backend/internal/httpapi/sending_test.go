package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"campaigncomposer/internal/httpapi"
	"campaigncomposer/internal/provider"
	"campaigncomposer/internal/worker"

	"github.com/jackc/pgx/v5/pgxpool"
)

type channelCounts struct {
	Pending int `json:"pending"`
	Sent    int `json:"sent"`
	Failed  int `json:"failed"`
	Total   int `json:"total"`
}

type campaignState struct {
	ID         int64                    `json:"id"`
	Status     string                   `json:"status"`
	Deliveries map[string]channelCounts `json:"deliveries"`
}

func getCampaign(t *testing.T, baseURL string, id int64) campaignState {
	t.Helper()
	var c campaignState
	if code := getJSON(t, baseURL+"/api/campaigns/"+strconv.FormatInt(id, 10), &c); code != http.StatusOK {
		t.Fatalf("GET campaign: status code %d", code)
	}
	return c
}

// startProvider serves the real mock Provider in-process.
func startProvider(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(provider.New())
	t.Cleanup(srv.Close)
	return srv
}

// startWorkers runs n Delivery workers in-process until the test ends.
func startWorkers(t *testing.T, pool *pgxpool.Pool, providerURL string, n int) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{}, n)
	for range n {
		w := worker.New(pool, worker.Options{ProviderURL: providerURL, PollInterval: 10 * time.Millisecond})
		go func() {
			defer func() { done <- struct{}{} }()
			if err := w.Run(ctx); err != nil && ctx.Err() == nil {
				t.Errorf("worker: %v", err)
			}
		}()
	}
	t.Cleanup(func() {
		cancel()
		for range n {
			<-done
		}
	})
}

// waitForStatus polls the Campaign until it reaches status.
func waitForStatus(t *testing.T, baseURL string, id int64, status string) campaignState {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for {
		c := getCampaign(t, baseURL, id)
		if c.Status == status {
			return c
		}
		if time.Now().After(deadline) {
			t.Fatalf("Campaign is %s with %+v, want %s", c.Status, c.Deliveries, status)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

type providerMessage struct {
	Channel        string `json:"channel"`
	IdempotencyKey string `json:"idempotency_key"`
	Requests       int    `json:"requests"`
	Payload        struct {
		To string `json:"to"`
	} `json:"payload"`
}

func providerMessages(t *testing.T, providerURL string) []providerMessage {
	t.Helper()
	var m struct{ Messages []providerMessage }
	getJSON(t, providerURL+"/messages", &m)
	return m.Messages
}

// assertOneRequestPerDelivery checks that the Provider got exactly one
// request for each of the Campaign's Deliveries, keyed by the Delivery id.
func assertOneRequestPerDelivery(t *testing.T, pool *pgxpool.Pool, id int64, msgs []providerMessage) {
	t.Helper()
	want := map[string]string{} // Delivery id -> Channel
	rows, err := pool.Query(context.Background(), `SELECT id, channel FROM deliveries WHERE campaign_id = $1`, id)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var d int64
		var ch string
		if err := rows.Scan(&d, &ch); err != nil {
			t.Fatal(err)
		}
		want[strconv.FormatInt(d, 10)] = ch
	}
	rows.Close()

	if len(msgs) != len(want) {
		t.Errorf("Provider recorded %d messages, want one per Delivery (%d)", len(msgs), len(want))
	}
	for _, m := range msgs {
		ch, ok := want[m.IdempotencyKey]
		switch {
		case !ok:
			t.Errorf("Provider got Idempotency-Key %q, which is no Delivery of the Campaign", m.IdempotencyKey)
		case ch != m.Channel:
			t.Errorf("Delivery %s is %s but went to the %s endpoint", m.IdempotencyKey, ch, m.Channel)
		case m.Requests != 1:
			t.Errorf("Delivery %s was requested %d times, want 1", m.IdempotencyKey, m.Requests)
		}
	}
}

func TestApprovedCampaignIsSentToTheProviderOncePerDeliveryAndCompletes(t *testing.T) {
	baseURL, pool, c := reviewedCampaign(t)
	prov := startProvider(t)
	startWorkers(t, pool, prov.URL, 1)

	if code, a := approve(t, baseURL, c.ID); code != http.StatusOK {
		t.Fatalf("approve: status code %d, body %+v", code, a)
	}
	got := waitForStatus(t, baseURL, c.ID, "completed")

	want := map[string]channelCounts{
		"sms":   {Sent: c.Audience.SMS, Total: c.Audience.SMS},
		"email": {Sent: c.Audience.Email, Total: c.Audience.Email},
	}
	for ch, w := range want {
		if got.Deliveries[ch] != w {
			t.Errorf("%s Deliveries = %+v, want %+v", ch, got.Deliveries[ch], w)
		}
	}
	assertOneRequestPerDelivery(t, pool, c.ID, providerMessages(t, prov.URL))
}

func TestTheProviderReceivesTheFrozenPayload(t *testing.T) {
	baseURL, pool, c := reviewedCampaign(t)
	prov := startProvider(t)
	startWorkers(t, pool, prov.URL, 1)
	if code, a := approve(t, baseURL, c.ID); code != http.StatusOK {
		t.Fatalf("approve: status code %d, body %+v", code, a)
	}
	waitForStatus(t, baseURL, c.ID, "completed")

	rows, err := pool.Query(context.Background(), `SELECT id::text, payload->>'to' FROM deliveries WHERE campaign_id = $1`, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	to := map[string]string{}
	for rows.Next() {
		var id, addr string
		if err := rows.Scan(&id, &addr); err != nil {
			t.Fatal(err)
		}
		to[id] = addr
	}
	rows.Close()
	for _, m := range providerMessages(t, prov.URL) {
		if m.Payload.To != to[m.IdempotencyKey] {
			t.Errorf("Delivery %s went to %q, want %q", m.IdempotencyKey, m.Payload.To, to[m.IdempotencyKey])
		}
	}
}

func TestConcurrentWorkersNeverClaimTheSameDelivery(t *testing.T) {
	baseURL, pool, c := reviewedCampaign(t)
	prov := startProvider(t)
	startWorkers(t, pool, prov.URL, 6)

	if code, a := approve(t, baseURL, c.ID); code != http.StatusOK {
		t.Fatalf("approve: status code %d, body %+v", code, a)
	}
	waitForStatus(t, baseURL, c.ID, "completed")

	// The Provider deduplicates, so a doubly claimed Delivery shows up as a
	// message with two requests rather than as two messages.
	assertOneRequestPerDelivery(t, pool, c.ID, providerMessages(t, prov.URL))
}

func TestAnApprovedCampaignMovesToSendingOnceTheWorkerStarts(t *testing.T) {
	baseURL, pool, c := reviewedCampaign(t)
	if code, a := approve(t, baseURL, c.ID); code != http.StatusOK {
		t.Fatalf("approve: status code %d, body %+v", code, a)
	}

	before := getCampaign(t, baseURL, c.ID)
	if before.Status != "approved" || before.Deliveries["sms"].Pending != c.Audience.SMS {
		t.Fatalf("before the worker: %+v, want approved with %d pending SMS", before, c.Audience.SMS)
	}

	// A Provider that never answers holds the first claims in flight.
	hold := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-hold:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(slow.Close)
	t.Cleanup(func() { close(hold) })
	startWorkers(t, pool, slow.URL, 1)

	waitForStatus(t, baseURL, c.ID, "sending")
}

func TestAnUnknownCampaignIsNotFound(t *testing.T) {
	srv := startAPI(t, httpapi.Options{})
	res, err := http.Get(srv.URL + "/api/campaigns/999999")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status code = %d, want 404", res.StatusCode)
	}
}
