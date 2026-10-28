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

	"github.com/jackc/pgx/v5/pgxpool"
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
	inner := provider.New(provider.Options{})
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
	// Every Delivery not yet claimed is cancelled with the Campaign.
	pendingAtCancel := map[string]bool{}
	rows, err := pool.Query(context.Background(),
		`SELECT id::text FROM deliveries WHERE campaign_id = $1 AND state = 'cancelled'`, c.ID)
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
		want := channelCounts{Cancelled: n - batch, Sent: batch, Total: n}
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
		t.Errorf("Provider accepted %d Deliveries of a Campaign cancelled before sending, want 0", len(msgs))
	}
	got := getCampaign(t, baseURL, c.ID)
	if got.Status != "cancelled" {
		t.Errorf("status = %s, want cancelled", got.Status)
	}
	want := map[string]channelCounts{
		"sms":   {Cancelled: c.Audience.SMS, Total: c.Audience.SMS},
		"email": {Cancelled: c.Audience.Email, Total: c.Audience.Email},
	}
	for ch, w := range want {
		if got.Deliveries[ch] != w {
			t.Errorf("%s Deliveries = %+v, want %+v", ch, got.Deliveries[ch], w)
		}
	}
}

// requestLog counts the Provider requests per Channel and Idempotency-Key.
type requestLog struct {
	mu sync.Mutex
	n  map[string]int
}

func (l *requestLog) record(channel, key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.n[channel+"/"+key]++
}

// keys is how many Deliveries have reached the Provider at least once.
func (l *requestLog) keys() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.n)
}

func (l *requestLog) requests(channel, key string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.n[channel+"/"+key]
}

// assertFinalCounts waits until no Delivery of the cancelled Campaign is
// pending or sending, then checks its per-Channel counts.
func assertFinalCounts(t *testing.T, baseURL string, c campaign, sent, failed int) {
	t.Helper()
	var got campaignState
	waitUntil(t, "every Delivery has its final state", func() bool {
		got = getCampaign(t, baseURL, c.ID)
		return got.Deliveries["sms"].Pending+got.Deliveries["email"].Pending == 0
	})
	if got.Status != "cancelled" {
		t.Errorf("status = %s, want cancelled", got.Status)
	}
	for ch, n := range map[string]int{"sms": c.Audience.SMS, "email": c.Audience.Email} {
		want := channelCounts{Sent: sent, Failed: failed, Cancelled: n - sent - failed, Total: n}
		if got.Deliveries[ch] != want {
			t.Errorf("%s Deliveries = %+v, want %+v", ch, got.Deliveries[ch], want)
		}
	}
}

// deliveryStates maps each of the Campaign's Delivery ids to its state.
func deliveryStates(t *testing.T, pool *pgxpool.Pool, id int64) map[string]string {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT id::text, state FROM deliveries WHERE campaign_id = $1`, id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, state string
		if err := rows.Scan(&k, &state); err != nil {
			t.Fatal(err)
		}
		out[k] = state
	}
	return out
}

func TestADeliveryInFlightAtCancelThatTimedOutAfterTheProviderAcceptedItEndsSent(t *testing.T) {
	baseURL, pool, c := reviewedCampaign(t)
	// Every first request is accepted but answered only after the worker has
	// given up on it, so the in-flight Deliveries have no known outcome.
	log := &requestLog{n: map[string]int{}}
	prov := startScriptedProvider(t, func(channel, key string, request int) provider.Outcome {
		log.record(channel, key)
		return provider.Outcome{Delay: 3 * time.Second}
	})
	const batch = 5
	opts := fastRetries(prov.URL)
	opts.Timeout = 500 * time.Millisecond
	opts.BatchSize, opts.Concurrency = batch, batch
	if code, a := approve(t, baseURL, c.ID); code != http.StatusOK {
		t.Fatalf("approve: status code %d, body %+v", code, a)
	}
	startWorker(t, pool, opts)
	waitUntil(t, "both Channels have a batch in flight", func() bool { return log.keys() == 2*batch })

	if code, got := cancel(t, baseURL, c.ID); code != http.StatusOK {
		t.Fatalf("cancel: status code %d, body %+v", code, got)
	}

	// The Recipients got their message, so the final counts say sent.
	assertFinalCounts(t, baseURL, c, batch, 0)
	msgs := providerMessages(t, prov.URL)
	if len(msgs) != 2*batch {
		t.Errorf("Provider accepted %d Deliveries, want the %d in flight at cancel", len(msgs), 2*batch)
	}
	assertZeroDuplicates(t, baseURL, pool, c.ID, msgs)
}

func TestADeliveryInFlightAtCancelThatTheProviderRejectedIsCancelledWithoutAnotherAttempt(t *testing.T) {
	baseURL, pool, c := reviewedCampaign(t)
	log := &requestLog{n: map[string]int{}}
	prov := startScriptedProvider(t, func(channel, key string, request int) provider.Outcome {
		log.record(channel, key)
		return provider.Outcome{Code: http.StatusServiceUnavailable, Delay: 500 * time.Millisecond}
	})
	const batch = 5
	opts := fastRetries(prov.URL)
	opts.Timeout = 2 * time.Second
	opts.BatchSize, opts.Concurrency = batch, batch
	if code, a := approve(t, baseURL, c.ID); code != http.StatusOK {
		t.Fatalf("approve: status code %d, body %+v", code, a)
	}
	startWorker(t, pool, opts)
	waitUntil(t, "both Channels have a batch in flight", func() bool { return log.keys() == 2*batch })

	if code, got := cancel(t, baseURL, c.ID); code != http.StatusOK {
		t.Fatalf("cancel: status code %d, body %+v", code, got)
	}

	// The Provider answered, so nothing was handed over: no retry after cancel.
	assertFinalCounts(t, baseURL, c, 0, 0)
	time.Sleep(200 * time.Millisecond) // many poll intervals for a stray retry
	for k, state := range deliveryStates(t, pool, c.ID) {
		for _, ch := range []string{"sms", "email"} {
			if n := log.requests(ch, k); n > 1 {
				t.Errorf("Delivery %s (%s) was requested %d times, want no retry after cancel", k, state, n)
			}
		}
	}
}

func TestDeliveriesHeldByAKilledWorkerWhenTheCampaignIsCancelledAreReclaimedToTheirOutcome(t *testing.T) {
	baseURL, pool, c := reviewedCampaign(t)
	// While holding, every first request is accepted and then held, so the
	// killed worker dies with its requests in flight.
	var holding atomic.Bool
	holding.Store(true)
	log := &requestLog{n: map[string]int{}}
	prov := startScriptedProvider(t, func(channel, key string, request int) provider.Outcome {
		log.record(channel, key)
		if holding.Load() {
			return provider.Outcome{Delay: time.Minute}
		}
		return provider.Outcome{}
	})
	const batch = 10
	opts := fastRetries(prov.URL)
	opts.Timeout = time.Second
	opts.Lease = 2 * time.Second
	opts.BatchSize, opts.Concurrency = batch, batch
	if code, a := approve(t, baseURL, c.ID); code != http.StatusOK {
		t.Fatalf("approve: status code %d, body %+v", code, a)
	}
	crashed := startWorkerProcess(t, pool, opts)
	waitUntil(t, "both Channels have a batch in flight", func() bool { return log.keys() == 2*batch })
	crashed.kill()
	holding.Store(false)
	if code, got := cancel(t, baseURL, c.ID); code != http.StatusOK {
		t.Fatalf("cancel: status code %d, body %+v", code, got)
	}

	startWorkersWith(t, pool, opts, 1)

	// The held Deliveries were accepted; once their lease expires they are
	// reclaimed, the Provider replays the acceptance, and nothing is left
	// `sending`.
	assertFinalCounts(t, baseURL, c, batch, 0)
	assertZeroDuplicates(t, baseURL, pool, c.ID, providerMessages(t, prov.URL))
}
