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
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"campaigncomposer/internal/content"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

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
	// reclaim it (ADR-0006). It must be well above Timeout, or requests
	// still in flight would be reclaimed: a Lease under twice the Timeout is
	// raised to that.
	Lease time.Duration
	// Timeout bounds one Provider request.
	Timeout time.Duration
	// BackoffBase is the wait before the retry after a first failed Attempt;
	// it doubles with every further Attempt (with jitter), up to MaxBackoff.
	// A 429's Retry-After wins when it is longer.
	BackoffBase time.Duration
	MaxBackoff  time.Duration
	// MaxAttempts is how many Attempts a Delivery gets before it is failed.
	MaxAttempts int
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
	if o.BackoffBase <= 0 {
		o.BackoffBase = 500 * time.Millisecond
	}
	if o.MaxBackoff <= 0 {
		o.MaxBackoff = time.Minute
	}
	if o.MaxAttempts <= 0 {
		o.MaxAttempts = 5
	}
	if o.RateLimit <= 0 {
		o.RateLimit = 100
	}
	if o.Lease < 2*o.Timeout {
		o.Lease = 2 * o.Timeout
	}
	return o
}

// startWindow is how long after its claim a Delivery's request may still
// start. A request takes at most Timeout, so one started within the window
// is answered and recorded before the lease expires, with a quarter of the
// rest of the lease to spare for the database writes. A Delivery whose turn
// comes later (its batch was paced slower than planned) is handed back
// instead of being sent under a lease that is about to run out.
func (o Options) startWindow() time.Duration {
	return (o.Lease - o.Timeout) * 3 / 4
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
	if opts.Lease > 0 && opts.Lease < 2*opts.withDefaults().Timeout {
		log.Printf("worker: lease %v is under twice the Provider timeout; using %v", opts.Lease, opts.withDefaults().Lease)
	}
	opts = opts.withDefaults()
	buckets := make(map[string]*Bucket, len(content.Channels))
	for _, ch := range content.Channels {
		buckets[ch] = NewBucket(opts.RateLimit * paceHeadroom)
	}
	return &Worker{
		pool:    pool,
		opts:    opts,
		client:  &http.Client{Timeout: opts.Timeout},
		buckets: buckets,
	}
}

// Run sends until ctx is cancelled. On the way out, claimed Deliveries are
// handed back to `pending` (see send and release); a worker that dies
// without that chance leaves them `sending` until their lease expires and
// another worker reclaims them.
func (w *Worker) Run(ctx context.Context) error {
	var wg sync.WaitGroup
	errs := make(chan error, len(content.Channels))
	for _, ch := range content.Channels {
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
	// attempt is the number of the Attempt this claim makes (1-based).
	attempt int
}

// drainBatch claims one batch, sends it, completes the Campaigns that have
// nothing left to send and cancels what cancelled Campaigns will never send.
// It returns how many Deliveries it claimed.
func (w *Worker) drainBatch(ctx context.Context, channel string) (int, error) {
	startBy := time.Now().Add(w.opts.startWindow())
	batch, err := w.claim(ctx, channel, w.batchSize(channel))
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
			wait, cancel := context.WithDeadline(ctx, startBy)
			err := w.buckets[channel].Wait(wait)
			cancel()
			if err != nil || time.Now().After(startBy) {
				// Shutting down, or the lease would run out mid-request:
				// hand the Delivery back unsent.
				w.release(context.WithoutCancel(ctx), d)
				return
			}
			w.send(ctx, channel, d)
		}()
	}
	wg.Wait()
	return len(batch), w.settleCampaigns(context.WithoutCancel(ctx))
}

// batchSize is how many Deliveries to claim: BatchSize, but no more than the
// Channel's bucket lets through in half the start window at its current
// rate, so a batch slowed by 429s (#14) is still sent within its lease.
func (w *Worker) batchSize(channel string) int {
	fit := int(w.buckets[channel].Rate() * w.opts.startWindow().Seconds() / 2)
	return max(1, min(w.opts.BatchSize, fit))
}

// claim moves up to n due Deliveries of the Channel to `sending`
// under a lease, and moves their Campaigns from `approved` to `sending`.
// Campaigns that are `approved` or `sending` are claimed from. A cancelled
// Campaign (#16) gets no new claims, except for Deliveries that were in
// flight when it was cancelled and whose outcome is unknown (ADR-0006): those
// left `sending` past their lease, and those whose request got no answer.
// They are sent again with the same Idempotency-Key, so the Provider replays
// the original result and the final counts are true.
//
// The claim takes FOR SHARE on each Campaign it claims from. A cancel that
// commits while the claim runs therefore either waits for the claim (whose
// Deliveries are then in flight and finish), or is seen by it: the share lock
// re-reads the Campaign and drops its Deliveries. Once a cancel has
// committed, no Delivery that was never handed to the Provider is claimed.
//
// The approved -> sending move runs as its own statement after the claim
// commits: two claims holding the share lock on the same Campaign (one per
// Channel) would deadlock if each then updated it.
//
// Deliveries left `sending` past their lease (their worker died or stalled)
// are claimed again like due `pending` ones. The claim raises
// attempt_count, which is what tells the stalled worker, should it wake up,
// that the Delivery is no longer its own (see send).
func (w *Worker) claim(ctx context.Context, channel string, n int) ([]delivery, error) {
	rows, err := w.pool.Query(ctx, `
		WITH due AS (
			SELECT d.id FROM deliveries d
			JOIN campaigns c ON c.id = d.campaign_id
			WHERE d.channel = $1
				AND ((d.state = 'pending' AND d.next_attempt_at <= now())
					OR (d.state = 'sending' AND d.lease_expires_at < now()))
				AND (c.status IN ('approved', 'sending')
					OR (c.status = 'cancelled' AND (d.state = 'sending' OR delivery_outcome_unknown(d.id))))
			ORDER BY d.next_attempt_at, d.id
			LIMIT $2
			FOR UPDATE OF d SKIP LOCKED
			FOR SHARE OF c
		)
		UPDATE deliveries d SET state = 'sending', attempt_count = d.attempt_count + 1,
			lease_expires_at = now() + make_interval(secs => $3), updated_at = now()
		FROM due WHERE d.id = due.id
		RETURNING d.id, d.campaign_id, d.payload, d.attempt_count`,
		channel, n, w.opts.Lease.Seconds())
	if err != nil {
		return nil, err
	}
	batch, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (delivery, error) {
		var d delivery
		return d, r.Scan(&d.id, &d.campaignID, &d.payload, &d.attempt)
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

// send makes one Attempt and records it in the Attempt log together with
// what it means for the Delivery (see classify): `sent`, back to `pending`
// with a later next-attempt time, or `failed`. The outcome is recorded even
// if ctx is cancelled after the Provider answered.
func (w *Worker) send(ctx context.Context, channel string, d delivery) {
	started := time.Now()
	res, err := w.post(ctx, channel, d)
	latency := time.Since(started)
	out := classify(res, err, d.attempt, w.opts.MaxAttempts)
	// Shutting down cut the request off. It may have reached the Provider,
	// so it is logged as an Attempt, and the Delivery goes straight back to
	// `pending` for the next worker instead of waiting out its lease; the
	// Idempotency-Key makes the repeat harmless.
	cutOff := err != nil && ctx.Err() != nil
	if cutOff {
		out = outcomeRetry
		err = fmt.Errorf("cut off by worker shutdown: %w", err)
	}

	var statusCode *int
	errText := ""
	if err != nil {
		errText = err.Error()
	} else {
		statusCode = &res.code
		if out != outcomeSent {
			errText = fmt.Sprintf("status %d: %s", res.code, res.body)
		}
	}

	// The Attempt is logged whatever happens: it was a real request. The
	// Delivery is only updated while this claim still holds it: once its
	// lease expired and another worker reclaimed it, attempt_count has moved
	// on, and that worker's outcome is the one that counts.
	db := context.WithoutCancel(ctx)
	txErr := pgx.BeginFunc(db, w.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(db, `INSERT INTO attempts (delivery_id, number, status_code, latency_ms, error, outcome, started_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			d.id, d.attempt, statusCode, latency.Milliseconds(), errText, string(out), started); err != nil {
			return err
		}
		var err error
		switch out {
		case outcomeSent:
			_, err = tx.Exec(db, `UPDATE deliveries SET state = 'sent', lease_expires_at = NULL, updated_at = now()
				WHERE id = $1 AND state = 'sending' AND attempt_count = $2`, d.id, d.attempt)
		case outcomeFailed:
			_, err = tx.Exec(db, `UPDATE deliveries SET state = 'failed', lease_expires_at = NULL, updated_at = now()
				WHERE id = $1 AND state = 'sending' AND attempt_count = $2`, d.id, d.attempt)
		default:
			delay := w.backoff(d.attempt)
			if cutOff {
				delay = 0
			}
			if res.retryAfter > delay {
				delay = res.retryAfter
			}
			_, err = tx.Exec(db, `UPDATE deliveries SET state = 'pending', lease_expires_at = NULL,
				next_attempt_at = now() + make_interval(secs => $3), updated_at = now()
				WHERE id = $1 AND state = 'sending' AND attempt_count = $2`, d.id, d.attempt, delay.Seconds())
		}
		return err
	})
	if txErr != nil {
		log.Printf("worker: record Attempt %d of Delivery %d: %v", d.attempt, d.id, txErr)
	}
}

// release hands a claimed Delivery back to `pending` without an Attempt, so
// its claim does not count against MaxAttempts. Like send's updates it only
// applies while this claim still holds the Delivery.
func (w *Worker) release(ctx context.Context, d delivery) {
	if _, err := w.pool.Exec(ctx, `UPDATE deliveries SET state = 'pending', attempt_count = attempt_count - 1,
			lease_expires_at = NULL, updated_at = now()
		WHERE id = $1 AND state = 'sending' AND attempt_count = $2`, d.id, d.attempt); err != nil {
		log.Printf("worker: release Delivery %d: %v", d.id, err)
	}
}

// outcome is what an Attempt means for its Delivery.
type outcome string

const (
	outcomeSent   outcome = "sent"
	outcomeRetry  outcome = "retry"
	outcomeFailed outcome = "failed"
)

// classify decides an Attempt's outcome. A 2xx is sent. A 429, a 5xx and a
// request that got no response (timeout, connection error) are retried until
// the Delivery has had maxAttempts; any other status is a permanent failure.
// Retrying after a timeout is safe because the Provider replays the original
// result for the same Idempotency-Key (ADR-0006).
func classify(res response, err error, attempt, maxAttempts int) outcome {
	retryable := err != nil || res.code == http.StatusTooManyRequests || res.code/100 == 5
	switch {
	case err == nil && res.code/100 == 2:
		return outcomeSent
	case retryable && attempt < maxAttempts:
		return outcomeRetry
	}
	return outcomeFailed
}

// backoff is the wait after the given failed Attempt: BackoffBase doubled
// per earlier Attempt, capped at MaxBackoff, with "equal jitter" (a random
// point in the upper half) so retries of a throttled batch spread out.
func (w *Worker) backoff(attempt int) time.Duration {
	d := w.opts.BackoffBase
	for i := 1; i < attempt && d < w.opts.MaxBackoff; i++ {
		d *= 2
	}
	d = min(d, w.opts.MaxBackoff)
	half := d / 2
	return half + time.Duration(rand.Int64N(int64(half)+1))
}

// response is what the Provider answered.
type response struct {
	code       int
	retryAfter time.Duration
	body       string // the start of the body, for the Attempt log
}

// post hands the Delivery's frozen payload to the Provider.
func (w *Worker) post(ctx context.Context, channel string, d delivery) (response, error) {
	url := strings.TrimRight(w.opts.ProviderURL, "/") + "/" + channel
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(d.payload))
	if err != nil {
		return response{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", strconv.FormatInt(d.id, 10))
	res, err := w.client.Do(req)
	if err != nil {
		return response{}, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusTooManyRequests {
		w.buckets[channel].Observe429()
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 200))
	if err != nil {
		return response{}, err // A body cut off by the timeout is a timeout.
	}
	_, _ = io.Copy(io.Discard, res.Body)
	return response{
		code:       res.StatusCode,
		retryAfter: parseRetryAfter(res.Header.Get("Retry-After"), time.Now()),
		body:       strings.TrimSpace(string(body)),
	}, nil
}

// parseRetryAfter reads a Retry-After header: delay-seconds (fractions are
// accepted, which the mock Provider uses to keep tests fast) or an HTTP date.
func parseRetryAfter(v string, now time.Time) time.Duration {
	if v == "" {
		return 0
	}
	if s, err := strconv.ParseFloat(v, 64); err == nil && s > 0 {
		return time.Duration(s * float64(time.Second))
	}
	if t, err := http.ParseTime(v); err == nil && t.After(now) {
		return t.Sub(now)
	}
	return 0
}

// settleCampaigns moves every Campaign with no Delivery left `pending` or
// `sending` to `completed`, and cancels the `pending` Deliveries of cancelled
// Campaigns that will not be sent: those the api's cancel skipped, and those
// in flight at cancel that came back without being handed over.
func (w *Worker) settleCampaigns(ctx context.Context) error {
	if _, err := w.pool.Exec(ctx, `UPDATE campaigns c SET status = 'completed'
		WHERE c.status IN ('approved', 'sending')
			AND EXISTS (SELECT 1 FROM deliveries d WHERE d.campaign_id = c.id)
			AND NOT EXISTS (SELECT 1 FROM deliveries d WHERE d.campaign_id = c.id AND d.state IN ('pending', 'sending'))`); err != nil {
		return err
	}
	_, err := w.pool.Exec(ctx, `SELECT cancel_unsent_deliveries()`)
	return err
}
