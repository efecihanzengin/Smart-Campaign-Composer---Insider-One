package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"campaigncomposer/internal/provider"
	"campaigncomposer/internal/worker"

	"github.com/jackc/pgx/v5/pgxpool"
)

// arrivals records when each request reached the Provider, per Channel.
type arrivals struct {
	mu    sync.Mutex
	times map[string][]time.Time
}

func (a *arrivals) record(channel string, at time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.times[channel] = append(a.times[channel], at)
}

func (a *arrivals) of(channel string) []time.Time {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.times[channel])
}

// startRecordingProvider serves the mock Provider behind a recorder. While
// throttle returns true, POSTs are answered 429 before reaching the Provider.
func startRecordingProvider(t *testing.T, throttle func() bool) (*httptest.Server, *arrivals) {
	t.Helper()
	rec := &arrivals{times: map[string][]time.Time{}}
	inner := provider.New()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			rec.record(strings.TrimPrefix(r.URL.Path, "/"), time.Now())
			if throttle != nil && throttle() {
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
		}
		inner.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, rec
}

// startWorker runs one Delivery worker with opts until the test ends.
func startWorker(t *testing.T, pool *pgxpool.Pool, opts worker.Options) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	w := worker.New(pool, opts)
	go func() {
		defer close(done)
		if err := w.Run(ctx); err != nil && ctx.Err() == nil {
			t.Errorf("worker: %v", err)
		}
	}()
	t.Cleanup(func() { cancel(); <-done })
}

// busiestSecond is the most requests that fall within any one-second window
// [t, t+1s).
func busiestSecond(times []time.Time) int {
	slices.SortFunc(times, func(a, b time.Time) int { return a.Compare(b) })
	most, lo := 0, 0
	for hi := range times {
		for times[hi].Sub(times[lo]) >= time.Second {
			lo++
		}
		most = max(most, hi-lo+1)
	}
	return most
}

// countBetween counts the requests in [from, to).
func countBetween(times []time.Time, from, to time.Time) int {
	n := 0
	for _, at := range times {
		if !at.Before(from) && at.Before(to) {
			n++
		}
	}
	return n
}

// addDeliveries gives the approved Campaign n more pending Deliveries on the
// Channel, so a test has enough to send to measure a rate.
func addDeliveries(t *testing.T, pool *pgxpool.Pool, id int64, channel string, n int) {
	t.Helper()
	exec(t, pool, `INSERT INTO deliveries (campaign_id, recipient_id, channel, payload)
		SELECT $1, r.id, $2, jsonb_build_object('to', r.email, 'body', 'x') FROM recipients r
		WHERE NOT EXISTS (SELECT 1 FROM deliveries d WHERE d.campaign_id = $1 AND d.recipient_id = r.id AND d.channel = $2)
		ORDER BY r.id LIMIT $3`, id, channel, n)
}

func TestNoChannelExceedsItsRateLimitAndChannelsDontSlowEachOther(t *testing.T) {
	const limit = 40
	baseURL, pool, c := reviewedCampaign(t)
	if code, a := approve(t, baseURL, c.ID); code != http.StatusOK {
		t.Fatalf("approve: status code %d, body %+v", code, a)
	}
	// Enough of each Channel for about three seconds of sending at the limit.
	exec(t, pool, `DELETE FROM deliveries WHERE campaign_id = $1`, c.ID)
	addDeliveries(t, pool, c.ID, "sms", 3*limit)
	addDeliveries(t, pool, c.ID, "email", 3*limit)
	prov, rec := startRecordingProvider(t, nil)

	start := time.Now()
	startWorker(t, pool, worker.Options{ProviderURL: prov.URL, PollInterval: 10 * time.Millisecond, RateLimit: limit})
	waitForStatus(t, baseURL, c.ID, "completed")
	elapsed := time.Since(start)

	for _, ch := range []string{"sms", "email"} {
		times := rec.of(ch)
		if len(times) != 3*limit {
			t.Errorf("%s: Provider got %d requests, want %d", ch, len(times), 3*limit)
		}
		if most := busiestSecond(times); most > limit {
			t.Errorf("%s: %d requests within one second, want at most %d", ch, most, limit)
		}
	}
	// Each Channel has its own limit, so both send side by side: 120 of each
	// take about three seconds, not the six a shared limit would need.
	if elapsed > 4500*time.Millisecond {
		t.Errorf("sending took %v, want about 3s with independent Channel limits", elapsed)
	}
}

func TestABurstOf429sLowersTheRateAndItRecoversOnceTheyStop(t *testing.T) {
	const limit = 40
	baseURL, pool, c := reviewedCampaign(t)
	if code, a := approve(t, baseURL, c.ID); code != http.StatusOK {
		t.Fatalf("approve: status code %d, body %+v", code, a)
	}
	exec(t, pool, `DELETE FROM deliveries WHERE campaign_id = $1`, c.ID)
	addDeliveries(t, pool, c.ID, "sms", 1000)

	// The Provider throttles for the first second, then accepts everything.
	start := time.Now()
	throttleUntil := start.Add(time.Second)
	prov, rec := startRecordingProvider(t, func() bool { return time.Now().Before(throttleUntil) })
	startWorker(t, pool, worker.Options{ProviderURL: prov.URL, PollInterval: 10 * time.Millisecond, RateLimit: limit})

	time.Sleep(10 * time.Second)
	times := rec.of("sms")

	if n := countBetween(times, throttleUntil, throttleUntil.Add(time.Second)); n > limit/2 {
		t.Errorf("%d requests in the second after the 429s, want at most %d (half the limit)", n, limit/2)
	}
	late := throttleUntil.Add(8 * time.Second)
	if n := countBetween(times, late, late.Add(time.Second)); n < limit*8/10 {
		t.Errorf("%d requests in a second long after the 429s stopped, want the rate back near %d", n, limit)
	}
}
