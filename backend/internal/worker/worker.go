// Package worker sends Deliveries to the Provider. Postgres is the queue
// (ADR-0001): each Channel's loop claims due `pending` Deliveries with
// `SELECT … FOR UPDATE SKIP LOCKED`, so any number of workers can run side by
// side without claiming the same Delivery, and every request carries the
// Delivery id as its Idempotency-Key (ADR-0006).
package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Channels are the Channels the worker sends on, each with its own loop.
var Channels = []string{"sms", "email"}

// Options configures a Worker. Zero values take the defaults below.
type Options struct {
	ProviderURL string
	// PollInterval is how long a Channel's loop sleeps when nothing is due.
	PollInterval time.Duration
	// BatchSize is how many Deliveries one claim takes.
	BatchSize int
	// Concurrency is how many requests a Channel has in flight at once.
	Concurrency int
	// Lease is how long a claimed Delivery is held before another worker may
	// reclaim it (reclaiming arrives with #15).
	Lease time.Duration
	// Timeout bounds one Provider request.
	Timeout time.Duration
	// RetryDelay is how long a Delivery whose Attempt did not succeed waits
	// before it is due again. Backoff, attempt limits and permanent failures
	// arrive with #13.
	RetryDelay time.Duration
	// RateLimit is the most Provider requests per second on each Channel,
	// shared by the Worker's sending on that Channel (#14). A burst of 429s
	// lowers it for a while; see Bucket.
	RateLimit float64
}

func (o Options) withDefaults() Options {
	if o.PollInterval <= 0 {
		o.PollInterval = 200 * time.Millisecond
	}
	if o.BatchSize <= 0 {
		o.BatchSize = 100
	}
	if o.Concurrency <= 0 {
		o.Concurrency = 16
	}
	if o.Lease <= 0 {
		o.Lease = 30 * time.Second
	}
	if o.Timeout <= 0 {
		o.Timeout = 5 * time.Second
	}
	if o.RetryDelay <= 0 {
		o.RetryDelay = time.Second
	}
	if o.RateLimit <= 0 {
		o.RateLimit = 100
	}
	return o
}

// paceHeadroom paces requests 2% under RateLimit. Pacing exactly at the
// limit puts limit+1 requests exactly one second apart, so a few ms of
// network jitter would let the Provider see limit+1 within a second; 2%
// leaves about 20ms of slack.
const paceHeadroom = 0.98

// Worker claims and sends Deliveries.
type Worker struct {
	pool   *pgxpool.Pool
	opts   Options
	client *http.Client
	// buckets holds one token bucket per Channel; the map is not written
	// after New.
	buckets map[string]*Bucket
}

// New returns a Worker sending through the Provider at opts.ProviderURL.
func New(pool *pgxpool.Pool, opts Options) *Worker {
	opts = opts.withDefaults()
	buckets := make(map[string]*Bucket, len(Channels))
	for _, ch := range Channels {
		buckets[ch] = NewBucket(opts.RateLimit*paceHeadroom, nil)
	}
	return &Worker{
		pool:    pool,
		opts:    opts,
		client:  &http.Client{Timeout: opts.Timeout},
		buckets: buckets,
	}
}

// Run sends until ctx is cancelled. A request cut off by cancellation leaves
// its Delivery in `sending`, to be reclaimed once its lease expires.
func (w *Worker) Run(ctx context.Context) error {
	var wg sync.WaitGroup
	errs := make(chan error, len(Channels))
	for _, ch := range Channels {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := w.runChannel(ctx, ch); err != nil && ctx.Err() == nil {
				errs <- fmt.Errorf("%s: %w", ch, err)
			}
		}()
	}
	wg.Wait()
	close(errs)
	return <-errs
}

// runChannel is one Channel's loop: claim a batch, send it, repeat; sleep
// when nothing is due. Database errors are logged and retried, so a Postgres
// blip doesn't stop sending.
func (w *Worker) runChannel(ctx context.Context, channel string) error {
	for ctx.Err() == nil {
		n, err := w.drainBatch(ctx, channel)
		if err != nil && ctx.Err() == nil {
			log.Printf("worker %s: %v", channel, err)
		}
		if n == 0 || err != nil {
			select {
			case <-ctx.Done():
			case <-time.After(w.opts.PollInterval):
			}
		}
	}
	return nil
}

type delivery struct {
	id         int64
	campaignID int64
	payload    json.RawMessage
}

// drainBatch claims one batch, sends it, and completes the Campaigns that
// have nothing left to send. It returns how many Deliveries it claimed.
func (w *Worker) drainBatch(ctx context.Context, channel string) (int, error) {
	batch, err := w.claim(ctx, channel)
	if err != nil {
		return 0, fmt.Errorf("claim: %w", err)
	}
	sem := make(chan struct{}, w.opts.Concurrency)
	var wg sync.WaitGroup
	for _, d := range batch {
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer func() { <-sem; wg.Done() }()
			// Shutting down: the claimed Delivery waits out its lease.
			if w.buckets[channel].Wait(ctx) != nil {
				return
			}
			w.send(ctx, channel, d)
		}()
	}
	wg.Wait()
	return len(batch), w.completeCampaigns(context.WithoutCancel(ctx))
}

// claim moves up to BatchSize due Deliveries of the Channel to `sending`
// under a lease, and moves their Campaigns from `approved` to `sending`.
// Only Campaigns that are `approved` or `sending` are claimed from, so a
// cancelled Campaign (#16) gets no new claims.
//
// The claim takes FOR SHARE on each Campaign it claims from. A cancel that
// commits while the claim runs therefore either waits for the claim (whose
// Deliveries are then in flight and finish), or is seen by it: the share lock
// re-reads the Campaign and drops its Deliveries. Once a cancel has
// committed, no Delivery still `pending` is claimed.
//
// The approved -> sending move runs as its own statement after the claim
// commits: two claims holding the share lock on the same Campaign (one per
// Channel) would deadlock if each then updated it.
func (w *Worker) claim(ctx context.Context, channel string) ([]delivery, error) {
	rows, err := w.pool.Query(ctx, `
		WITH due AS (
			SELECT d.id FROM deliveries d
			JOIN campaigns c ON c.id = d.campaign_id
			WHERE d.channel = $1 AND d.state = 'pending' AND d.next_attempt_at <= now()
				AND c.status IN ('approved', 'sending')
			ORDER BY d.next_attempt_at, d.id
			LIMIT $2
			FOR UPDATE OF d SKIP LOCKED
			FOR SHARE OF c
		)
		UPDATE deliveries d SET state = 'sending', attempt_count = d.attempt_count + 1,
			lease_expires_at = now() + make_interval(secs => $3), updated_at = now()
		FROM due WHERE d.id = due.id
		RETURNING d.id, d.campaign_id, d.payload`,
		channel, w.opts.BatchSize, w.opts.Lease.Seconds())
	if err != nil {
		return nil, err
	}
	batch, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (delivery, error) {
		var d delivery
		return d, r.Scan(&d.id, &d.campaignID, &d.payload)
	})
	if err != nil || len(batch) == 0 {
		return batch, err
	}
	ids := make([]int64, 0, len(batch))
	for _, d := range batch {
		ids = append(ids, d.campaignID)
	}
	// The claim has committed, so the batch is sent even if this fails; the
	// next claim moves the Campaign then.
	if _, err := w.pool.Exec(context.WithoutCancel(ctx),
		`UPDATE campaigns SET status = 'sending' WHERE status = 'approved' AND id = ANY($1)`, ids); err != nil {
		log.Printf("worker: move Campaigns to sending: %v", err)
	}
	return batch, nil
}

// send makes one Attempt. A 2xx moves the Delivery to `sent`; anything else
// returns it to `pending` after RetryDelay. The outcome is recorded even if
// ctx is cancelled after the Provider answered.
func (w *Worker) send(ctx context.Context, channel string, d delivery) {
	code, err := w.post(ctx, channel, d)
	if ctx.Err() != nil && err != nil {
		return // Shutting down: the lease hands the Delivery to the next worker.
	}
	db := context.WithoutCancel(ctx)
	if err == nil && code/100 == 2 {
		if _, err := w.pool.Exec(db, `UPDATE deliveries SET state = 'sent', lease_expires_at = NULL, updated_at = now()
			WHERE id = $1 AND state = 'sending'`, d.id); err != nil {
			log.Printf("worker: mark Delivery %d sent: %v", d.id, err)
		}
		return
	}
	// Classifying the outcome (retryable or permanent), backoff and the
	// Attempt log arrive with #13.
	log.Printf("worker: Delivery %d: status %d, error %v; retrying", d.id, code, err)
	if _, err := w.pool.Exec(db, `UPDATE deliveries SET state = 'pending', lease_expires_at = NULL,
		next_attempt_at = now() + make_interval(secs => $2), updated_at = now()
		WHERE id = $1 AND state = 'sending'`, d.id, w.opts.RetryDelay.Seconds()); err != nil {
		log.Printf("worker: release Delivery %d: %v", d.id, err)
	}
}

// post hands the Delivery's frozen payload to the Provider.
func (w *Worker) post(ctx context.Context, channel string, d delivery) (int, error) {
	url := strings.TrimRight(w.opts.ProviderURL, "/") + "/" + channel
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(d.payload))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", strconv.FormatInt(d.id, 10))
	res, err := w.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, res.Body)
	if res.StatusCode == http.StatusTooManyRequests {
		w.buckets[channel].Observe429()
	}
	return res.StatusCode, nil
}

// completeCampaigns moves every Campaign with no Delivery left `pending` or
// `sending` to `completed`.
func (w *Worker) completeCampaigns(ctx context.Context) error {
	_, err := w.pool.Exec(ctx, `UPDATE campaigns c SET status = 'completed'
		WHERE c.status IN ('approved', 'sending')
			AND EXISTS (SELECT 1 FROM deliveries d WHERE d.campaign_id = c.id)
			AND NOT EXISTS (SELECT 1 FROM deliveries d WHERE d.campaign_id = c.id AND d.state IN ('pending', 'sending'))`)
	return err
}
