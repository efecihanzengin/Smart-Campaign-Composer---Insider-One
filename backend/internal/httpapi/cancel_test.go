package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"campaigncomposer/internal/provider"
	"campaigncomposer/internal/worker"
)

type cancellation struct {
	ID     int64  `json:"id"`
	Status string `json:"status"`
	Error  string `json:"error"`
}

func cancel(t *testing.T, baseURL string, id int64) (int, cancellation) {
	t.Helper()
	var c cancellation
	code := postJSON(t, baseURL+"/api/campaigns/"+strconv.FormatInt(id, 10)+"/cancel", nil, &c)
	return code, c
}

func TestCancellingADraftCampaignCancelsItAndItCanNoLongerBeApproved(t *testing.T) {
	baseURL, _, c := reviewedCampaign(t)

	code, got := cancel(t, baseURL, c.ID)

	if code != http.StatusOK || got.Status != "cancelled" || got.ID != c.ID {
		t.Fatalf("cancel: status code %d, body %+v; want 200, cancelled", code, got)
	}
	if s := getCampaign(t, baseURL, c.ID); s.Status != "cancelled" {
		t.Errorf("GET campaign status = %s, want cancelled", s.Status)
	}
	if code, a := approve(t, baseURL, c.ID); code != http.StatusConflict {
		t.Errorf("approve after cancel: status code %d, body %+v; want 409", code, a)
	}
}

func TestOnlyDraftApprovedOrSendingCampaignsCanBeCancelled(t *testing.T) {
	baseURL, pool, c := reviewedCampaign(t)
	if code, _ := cancel(t, baseURL, c.ID); code != http.StatusOK {
		t.Fatalf("first cancel: status code %d, want 200", code)
	}
	if code, got := cancel(t, baseURL, c.ID); code != http.StatusConflict {
		t.Errorf("cancelling a cancelled Campaign: status code %d, body %+v; want 409", code, got)
	}

	done := createCampaign(t, baseURL, exampleBrief)
	lastDone(t, streamContent(t, baseURL, done.ID))
	if code, a := approve(t, baseURL, done.ID); code != http.StatusOK {
		t.Fatalf("approve: status code %d, body %+v", code, a)
	}
	startWorkers(t, pool, startProvider(t).URL, 1)
	waitForStatus(t, baseURL, done.ID, "completed")
	if code, got := cancel(t, baseURL, done.ID); code != http.StatusConflict {
		t.Errorf("cancelling a completed Campaign: status code %d, body %+v; want 409", code, got)
	}
	if s := getCampaign(t, baseURL, done.ID); s.Status != "completed" {
		t.Errorf("status after refused cancel = %s, want completed", s.Status)
	}

	if code, _ := cancel(t, baseURL, 999999); code != http.StatusNotFound {
		t.Errorf("cancelling an unknown Campaign: status code %d, want 404", code)
	}
}

// startGatedProvider serves the mock Provider, but holds every POST until
// release is called. arrived counts the POSTs that reached the gate.
func startGatedProvider(t *testing.T) (srv *httptest.Server, arrived *atomic.Int64, release func()) {
	t.Helper()
	gate := make(chan struct{})
	arrived = &atomic.Int64{}
	inner := provider.New()
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			arrived.Add(1)
			select {
			case <-gate:
			case <-r.Context().Done():
				return
			}
		}
		inner.ServeHTTP(w, r)
	}))
	var once sync.Once
	release = func() { once.Do(func() { close(gate) }) }
	t.Cleanup(srv.Close)
	t.Cleanup(release)
	return srv, arrived, release
}

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting until %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestCancellingMidSendStopsNewClaimsAndLetsInFlightDeliveriesFinish(t *testing.T) {
	baseURL, pool, c := reviewedCampaign(t)
	if code, a := approve(t, baseURL, c.ID); code != http.StatusOK {
		t.Fatalf("approve: status code %d, body %+v", code, a)
	}
	prov, arrived, release := startGatedProvider(t)
	const batch = 5 // per Channel; the gate holds each Channel's first batch in flight
	startWorker(t, pool, worker.Options{ProviderURL: prov.URL, PollInterval: 10 * time.Millisecond,
		BatchSize: batch, Concurrency: batch})
	waitUntil(t, "both Channels have a batch in flight", func() bool { return arrived.Load() == 2*batch })
	if s := getCampaign(t, baseURL, c.ID); s.Status != "sending" {
		t.Fatalf("status before cancel = %s, want sending", s.Status)
	}

	if code, got := cancel(t, baseURL, c.ID); code != http.StatusOK || got.Status != "cancelled" {
		t.Fatalf("cancel: status code %d, body %+v; want 200, cancelled", code, got)
	}
	pendingAtCancel := map[string]bool{}
	rows, err := pool.Query(context.Background(),
		`SELECT id::text FROM deliveries WHERE campaign_id = $1 AND state = 'pending'`, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		pendingAtCancel[id] = true
	}
	rows.Close()
	release()

	total := c.Audience.SMS + c.Audience.Email
	waitUntil(t, "the in-flight Deliveries finish", func() bool {
		s := getCampaign(t, baseURL, c.ID)
		return s.Deliveries["sms"].Sent+s.Deliveries["email"].Sent == 2*batch
	})
	time.Sleep(300 * time.Millisecond) // many poll intervals for a stray claim

	for _, m := range providerMessages(t, prov.URL) {
		if pendingAtCancel[m.IdempotencyKey] {
			t.Errorf("Provider got Delivery %s, which was pending when the Campaign was cancelled", m.IdempotencyKey)
		}
	}
	got := getCampaign(t, baseURL, c.ID)
	if got.Status != "cancelled" {
		t.Errorf("status = %s, want cancelled", got.Status)
	}
	for ch, n := range map[string]int{"sms": c.Audience.SMS, "email": c.Audience.Email} {
		want := channelCounts{Pending: n - batch, Sent: batch, Total: n}
		if got.Deliveries[ch] != want {
			t.Errorf("%s Deliveries = %+v, want %+v", ch, got.Deliveries[ch], want)
		}
	}
	if len(pendingAtCancel) != total-2*batch {
		t.Errorf("%d Deliveries pending at cancel, want %d", len(pendingAtCancel), total-2*batch)
	}
}

func TestCancellingAnApprovedCampaignMeansNothingIsSent(t *testing.T) {
	baseURL, pool, c := reviewedCampaign(t)
	if code, a := approve(t, baseURL, c.ID); code != http.StatusOK {
		t.Fatalf("approve: status code %d, body %+v", code, a)
	}

	if code, got := cancel(t, baseURL, c.ID); code != http.StatusOK || got.Status != "cancelled" {
		t.Fatalf("cancel: status code %d, body %+v; want 200, cancelled", code, got)
	}
	prov := startProvider(t)
	startWorkers(t, pool, prov.URL, 2)
	time.Sleep(300 * time.Millisecond) // many poll intervals

	if msgs := providerMessages(t, prov.URL); len(msgs) != 0 {
		t.Errorf("Provider got %d messages for a Campaign cancelled before sending, want 0", len(msgs))
	}
	got := getCampaign(t, baseURL, c.ID)
	if got.Status != "cancelled" {
		t.Errorf("status = %s, want cancelled", got.Status)
	}
	want := map[string]channelCounts{
		"sms":   {Pending: c.Audience.SMS, Total: c.Audience.SMS},
		"email": {Pending: c.Audience.Email, Total: c.Audience.Email},
	}
	for ch, w := range want {
		if got.Deliveries[ch] != w {
			t.Errorf("%s Deliveries = %+v, want %+v", ch, got.Deliveries[ch], w)
		}
	}
}
