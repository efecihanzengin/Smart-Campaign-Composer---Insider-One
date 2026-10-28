package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	oexec "os/exec"
	"os/signal"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"campaigncomposer/internal/provider"
	"campaigncomposer/internal/worker"

	"github.com/jackc/pgx/v5/pgxpool"
)

// The crash tests run a worker as a separate process (this test binary
// re-executed into TestWorkerProcess), so it can be killed with SIGKILL or
// frozen with SIGSTOP the way a crashed or stalled container would be: it
// gets no chance to write anything on the way out.

const workerProcessEnv = "CRASH_TEST_WORKER"

// TestWorkerProcess is not a test: it is the body of a worker process
// started by startWorkerProcess, and is skipped in a normal run.
func TestWorkerProcess(t *testing.T) {
	spec := os.Getenv(workerProcessEnv)
	if spec == "" {
		t.Skip("only runs as a worker process started by a crash test")
	}
	var p struct {
		DatabaseURL string
		Options     worker.Options
	}
	if err := json.Unmarshal([]byte(spec), &p); err != nil {
		t.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)
	defer stop()
	pool, err := pgxpool.New(ctx, p.DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := worker.New(pool, p.Options).Run(ctx); err != nil && ctx.Err() == nil {
		t.Fatal(err)
	}
}

// workerProcess is a worker running in its own process.
type workerProcess struct {
	cmd  *oexec.Cmd
	done chan struct{}
}

// startWorkerProcess runs a worker with opts in a child process against the
// test's database. It is killed when the test ends, if still running.
func startWorkerProcess(t *testing.T, pool *pgxpool.Pool, opts worker.Options) *workerProcess {
	t.Helper()
	u, err := url.Parse(os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + pool.Config().ConnConfig.Database
	spec, _ := json.Marshal(map[string]any{"DatabaseURL": u.String(), "Options": opts})

	cmd := oexec.Command(os.Args[0], "-test.run=^TestWorkerProcess$", "-test.count=1")
	cmd.Env = append(os.Environ(), workerProcessEnv+"="+string(spec))
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	p := &workerProcess{cmd: cmd, done: make(chan struct{})}
	go func() { _ = cmd.Wait(); close(p.done) }()
	t.Cleanup(func() {
		_ = cmd.Process.Signal(syscall.SIGCONT)
		p.kill()
	})
	return p
}

// kill ends the process with SIGKILL and waits for it to exit.
func (p *workerProcess) kill() {
	_ = p.cmd.Process.Kill()
	<-p.done
}

func (p *workerProcess) signal(t *testing.T, sig os.Signal) {
	t.Helper()
	if err := p.cmd.Process.Signal(sig); err != nil {
		t.Fatalf("signal %v: %v", sig, err)
	}
}

// firstRequests records when the first request for each Idempotency-Key
// reached the Provider.
type firstRequests struct {
	mu sync.Mutex
	at map[string]time.Time
}

func (f *firstRequests) record(channel, key string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.at[channel+"/"+key]; !ok {
		f.at[channel+"/"+key] = time.Now()
	}
}

func (f *firstRequests) get(channel, key string) (time.Time, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	at, ok := f.at[channel+"/"+key]
	return at, ok
}

// keys returns the requested Channel/key pairs so far.
func (f *firstRequests) keys() map[string]bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]bool{}
	for k := range f.at {
		out[k] = true
	}
	return out
}

func (f *firstRequests) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.at)
}

// assertZeroDuplicates checks what the Provider received against the
// Campaign's Deliveries: every message belongs to a Delivery and was
// accepted once per Idempotency-Key and once per (Campaign, Recipient,
// Channel), and every `sent` Delivery has its message.
func assertZeroDuplicates(t *testing.T, baseURL string, pool *pgxpool.Pool, id int64, msgs []providerMessage) {
	t.Helper()
	type delivery struct{ channel, state, to string }
	deliveries := map[string]delivery{}
	rows, err := pool.Query(context.Background(),
		`SELECT id::text, channel, state, payload->>'to' FROM deliveries WHERE campaign_id = $1`, id)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var k string
		var d delivery
		if err := rows.Scan(&k, &d.channel, &d.state, &d.to); err != nil {
			t.Fatal(err)
		}
		deliveries[k] = d
	}
	rows.Close()

	byKey := map[string]int{}
	byRecipient := map[string]int{} // Channel + address: one Recipient on one Channel
	for _, m := range msgs {
		byKey[m.Channel+"/"+m.IdempotencyKey]++
		byRecipient[m.Channel+"/"+m.Payload.To]++
		d, ok := deliveries[m.IdempotencyKey]
		switch {
		case !ok:
			t.Errorf("Provider got Idempotency-Key %q, which is no Delivery of the Campaign", m.IdempotencyKey)
		case d.channel != m.Channel || d.to != m.Payload.To:
			t.Errorf("Delivery %s (%s to %s) arrived as %s to %s", m.IdempotencyKey, d.channel, d.to, m.Channel, m.Payload.To)
		}
	}
	for k, n := range byKey {
		if n > 1 {
			t.Errorf("Provider recorded %d messages for %s", n, k)
		}
	}
	for k, n := range byRecipient {
		if n > 1 {
			t.Errorf("Provider recorded %d messages for Recipient %s", n, k)
		}
	}
	for k, d := range deliveries {
		if d.state == "sent" && byKey[d.channel+"/"+k] != 1 {
			t.Errorf("Delivery %s is sent but the Provider has no message for it", k)
		}
	}
}

func TestDeliveriesHeldByAKilledWorkerAreCompletedByAnotherAfterTheirLease(t *testing.T) {
	baseURL, pool, c := reviewedCampaign(t)
	// While holding, every first request is accepted and then held, so the
	// killed worker dies with its requests in flight.
	var holding atomic.Bool
	holding.Store(true)
	first := &firstRequests{at: map[string]time.Time{}}
	prov := startScriptedProvider(t, func(channel, key string, request int) provider.Outcome {
		first.record(channel, key)
		if holding.Load() {
			return provider.Outcome{Delay: time.Minute}
		}
		return provider.Outcome{}
	})
	opts := fastRetries(prov.URL)
	opts.Timeout = time.Second
	opts.Lease = 2 * time.Second
	opts.BatchSize, opts.Concurrency = 10, 10

	if code, a := approve(t, baseURL, c.ID); code != http.StatusOK {
		t.Fatalf("approve: status code %d, body %+v", code, a)
	}
	crashed := startWorkerProcess(t, pool, opts)
	waitUntil(t, "both Channels have a batch in flight", func() bool { return first.count() == 20 })
	crashed.kill()
	holding.Store(false)

	held := campaignDeliveries(t, baseURL, c.ID, "sending")
	if len(held) != 20 {
		t.Fatalf("after the kill %d Deliveries are sending, want the 20 in flight", len(held))
	}
	startWorkersWith(t, pool, opts, 1)
	got := waitForStatus(t, baseURL, c.ID, "completed")

	if got.Deliveries["sms"].Sent != c.Audience.SMS || got.Deliveries["email"].Sent != c.Audience.Email {
		t.Fatalf("Deliveries = %+v, want all sent", got.Deliveries)
	}
	msgs := providerMessages(t, prov.URL)
	assertZeroDuplicates(t, baseURL, pool, c.ID, msgs)
	sent := map[int64]deliveryWithAttempts{}
	for _, ch := range []string{"sms", "email"} {
		for _, d := range campaignDeliveries(t, baseURL, c.ID, "sent&limit=500&channel="+ch) {
			sent[d.ID] = d
		}
	}
	for _, h := range held {
		d := sent[h.ID]
		// The killed worker's Attempt was never logged; the reclaim is the
		// second Attempt, and the Provider replayed its acceptance.
		if d.AttemptCount != 2 || len(d.Attempts) != 1 || d.Attempts[0].Number != 2 || d.Attempts[0].Outcome != "sent" {
			t.Errorf("reclaimed Delivery %d: %d Attempts, log %+v; want Attempt 2 sent", h.ID, d.AttemptCount, d.Attempts)
			continue
		}
		at, _ := first.get(h.Channel, strconv.FormatInt(h.ID, 10))
		if gap := d.Attempts[0].At.Sub(at); gap < opts.Lease-100*time.Millisecond {
			t.Errorf("Delivery %d was reclaimed %v after the killed worker sent it, want no sooner than its lease of %v", h.ID, gap, opts.Lease)
		}
	}
}

// startStoppableWorker runs a worker in-process and returns a func that
// stops it the way SIGTERM does (cancels its context) and waits for Run to
// return.
func startStoppableWorker(t *testing.T, pool *pgxpool.Pool, opts worker.Options) (stop func()) {
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
	var once sync.Once
	stop = func() { once.Do(func() { cancel(); <-done }) }
	t.Cleanup(stop)
	return stop
}

func TestAWorkerStoppedMidSendHandsItsDeliveriesBackWithoutWaitingForTheLease(t *testing.T) {
	baseURL, pool, c := reviewedCampaign(t)
	var holding atomic.Bool
	holding.Store(true)
	first := &firstRequests{at: map[string]time.Time{}}
	prov := startScriptedProvider(t, func(channel, key string, request int) provider.Outcome {
		first.record(channel, key)
		if holding.Load() {
			return provider.Outcome{Delay: time.Minute} // accepted, answer held
		}
		return provider.Outcome{}
	})
	opts := fastRetries(prov.URL)
	opts.Timeout = 10 * time.Second
	opts.Lease = time.Minute // longer than the test may take
	opts.BatchSize, opts.Concurrency = 10, 5

	if code, a := approve(t, baseURL, c.ID); code != http.StatusOK {
		t.Fatalf("approve: status code %d, body %+v", code, a)
	}
	stop := startStoppableWorker(t, pool, opts)
	waitUntil(t, "both Channels have requests in flight", func() bool { return first.count() == 10 })
	stop()
	holding.Store(false)
	inFlight := first.keys()

	if s := campaignDeliveries(t, baseURL, c.ID, "sending"); len(s) != 0 {
		t.Fatalf("after a graceful stop %d Deliveries are still sending, want none", len(s))
	}
	startWorkersWith(t, pool, opts, 1)
	got := waitForStatus(t, baseURL, c.ID, "completed")

	if got.Deliveries["sms"].Sent != c.Audience.SMS || got.Deliveries["email"].Sent != c.Audience.Email {
		t.Fatalf("Deliveries = %+v, want all sent", got.Deliveries)
	}
	assertZeroDuplicates(t, baseURL, pool, c.ID, providerMessages(t, prov.URL))
	// The cut-off request is logged as an Attempt without a response; the
	// Deliveries that were claimed but not yet requested lost no Attempt.
	for _, ch := range []string{"sms", "email"} {
		for _, d := range campaignDeliveries(t, baseURL, c.ID, "sent&limit=10&channel="+ch) {
			if !inFlight[ch+"/"+strconv.FormatInt(d.ID, 10)] {
				continue
			}
			if len(d.Attempts) != 2 || d.Attempts[0].StatusCode != nil || d.Attempts[0].Outcome != "retry" ||
				d.Attempts[0].Error == "" || d.Attempts[1].Outcome != "sent" {
				t.Errorf("Delivery %d Attempts = %+v, want a cut-off retry then sent", d.ID, d.Attempts)
			}
		}
		for _, d := range campaignDeliveries(t, baseURL, c.ID, "sent&limit=500&channel="+ch) {
			if inFlight[ch+"/"+strconv.FormatInt(d.ID, 10)] {
				continue
			}
			if d.AttemptCount != 1 || len(d.Attempts) != 1 {
				t.Errorf("Delivery %d: %d Attempts (%d logged), want 1", d.ID, d.AttemptCount, len(d.Attempts))
			}
		}
	}
}

func TestRestartingTheWorkerMidCampaignUnderTheDefaultFailureRatesSendsEveryMessageOnce(t *testing.T) {
	baseURL, pool, c := reviewedCampaign(t)
	// The Provider's default failure rates, with its timings scaled down
	// alongside the worker's.
	srv := httptest.NewServer(provider.New(provider.Options{
		Rate429: 0.10, Rate5xx: 0.05, RateSlow: 0.03,
		RetryAfter: "0.05", SlowDelay: 500 * time.Millisecond,
		LatencyMin: time.Millisecond, LatencyMax: 20 * time.Millisecond,
		Seed: 15,
	}))
	t.Cleanup(srv.Close)
	opts := fastRetries(srv.URL)
	opts.BackoffBase = 10 * time.Millisecond
	opts.Timeout = 200 * time.Millisecond
	opts.Lease = time.Second
	total := c.Audience.SMS + c.Audience.Email
	sent := func() int {
		got := getCampaign(t, baseURL, c.ID)
		return got.Deliveries["sms"].Sent + got.Deliveries["email"].Sent
	}

	if code, a := approve(t, baseURL, c.ID); code != http.StatusOK {
		t.Fatalf("approve: status code %d, body %+v", code, a)
	}
	// The first worker crashes a third of the way in, the second is stopped
	// gracefully (SIGTERM, as `docker compose restart` does) two thirds in,
	// and a third one finishes.
	first := startWorkerProcess(t, pool, opts)
	waitUntil(t, "a third is sent", func() bool { return sent() >= total/3 })
	first.kill()
	second := startWorkerProcess(t, pool, opts)
	waitUntil(t, "two thirds are sent", func() bool { return sent() >= 2*total/3 })
	second.signal(t, syscall.SIGTERM)
	<-second.done
	startWorkerProcess(t, pool, opts)
	got := waitForStatus(t, baseURL, c.ID, "completed")

	for ch, n := range map[string]int{"sms": c.Audience.SMS, "email": c.Audience.Email} {
		if d := got.Deliveries[ch]; d.Pending != 0 || d.Sent+d.Failed != n {
			t.Errorf("%s Deliveries = %+v, want all %d sent or failed", ch, d, n)
		}
	}
	assertZeroDuplicates(t, baseURL, pool, c.ID, providerMessages(t, srv.URL))
}

func TestABatchPacedSlowerThanItsLeaseIsNotRequestedTwice(t *testing.T) {
	baseURL, pool, c := reviewedCampaign(t)
	if code, a := approve(t, baseURL, c.ID); code != http.StatusOK {
		t.Fatalf("approve: status code %d, body %+v", code, a)
	}
	// 60 SMS at 20/s take three seconds for one worker, three times the
	// lease, so one worker claiming them all would outlast it.
	exec(t, pool, `DELETE FROM deliveries WHERE campaign_id = $1`, c.ID)
	addDeliveries(t, pool, c.ID, "sms", 60)
	prov := startProvider(t)
	opts := worker.Options{ProviderURL: prov.URL, PollInterval: 10 * time.Millisecond,
		RateLimit: 20, Timeout: 200 * time.Millisecond, Lease: time.Second}
	startWorkersWith(t, pool, opts, 2)

	waitForStatus(t, baseURL, c.ID, "completed")
	assertOneRequestPerDelivery(t, pool, c.ID, providerMessages(t, prov.URL))
}

func TestAStalledWorkerWakingUpAfterItsLeaseCannotOverwriteTheReclaimedDelivery(t *testing.T) {
	baseURL, pool, c := reviewedCampaign(t)
	// The stalled worker's requests fail; once it is frozen, the reclaiming
	// worker's requests for the same Deliveries are accepted but answered
	// only after the stalled worker has woken up and recorded its failure.
	var stalled atomic.Bool
	var mu sync.Mutex
	held := map[string]bool{}
	reclaimed := 0
	prov := startScriptedProvider(t, func(channel, key string, request int) provider.Outcome {
		mu.Lock()
		defer mu.Unlock()
		if !stalled.Load() {
			held[channel+"/"+key] = true
			return provider.Outcome{Code: http.StatusServiceUnavailable, Delay: 300 * time.Millisecond}
		}
		if held[channel+"/"+key] {
			reclaimed++
			return provider.Outcome{Delay: time.Second}
		}
		return provider.Outcome{}
	})
	opts := fastRetries(prov.URL)
	opts.MaxAttempts = 1 // the stalled worker's failure is final
	opts.Timeout = time.Second
	opts.Lease = 2 * time.Second
	opts.BatchSize, opts.Concurrency = 10, 10

	if code, a := approve(t, baseURL, c.ID); code != http.StatusOK {
		t.Fatalf("approve: status code %d, body %+v", code, a)
	}
	slow := startWorkerProcess(t, pool, opts)
	waitUntil(t, "both Channels have a batch in flight", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(held) == 20
	})
	slow.signal(t, syscall.SIGSTOP)
	stalled.Store(true)
	time.Sleep(opts.Lease + 200*time.Millisecond)

	reclaim := opts
	reclaim.Timeout = 2 * time.Second
	reclaim.Lease = 4 * time.Second
	startWorkersWith(t, pool, reclaim, 1)
	waitUntil(t, "the held Deliveries are reclaimed", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return reclaimed == 20
	})
	slow.signal(t, syscall.SIGCONT)
	got := waitForStatus(t, baseURL, c.ID, "completed")

	if got.Deliveries["sms"].Sent != c.Audience.SMS || got.Deliveries["email"].Sent != c.Audience.Email {
		t.Errorf("Deliveries = %+v, want all sent: the Provider accepted every one", got.Deliveries)
	}
	assertZeroDuplicates(t, baseURL, pool, c.ID, providerMessages(t, prov.URL))
}
