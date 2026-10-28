package worker

import (
	"context"
	"math"
	"sync"
	"time"
)

const (
	// cooldown is how long one cut of the rate lasts: 429s within a second
	// of a cut count as the same burst, and recovery starts a second after
	// the last 429.
	cooldown = time.Second
	// minRate is the floor the rate is never halved below.
	minRate = 1.0
	// recoverySteps is how many seconds a full climb from zero would take:
	// the rate recovers by limit/recoverySteps per second.
	recoverySteps = 5
)

// Bucket is a token bucket that paces one Channel's Provider requests. It
// holds at most one token, so requests are spread evenly at the rate and
// never burst above it. A burst of 429s halves the rate (at most once per
// second); once the 429s stop for a second, the rate climbs back towards the
// limit by a fifth of the limit per second.
type Bucket struct {
	limit float64
	now   func() time.Time

	mu      sync.Mutex
	tokens  float64
	last    time.Time // when tokens was last refilled
	base    float64   // the rate set by the last cut (the limit before any)
	lastCut time.Time
	last429 time.Time
}

// NewBucket returns a Bucket allowing limit requests per second. now is the
// clock; nil means time.Now.
func NewBucket(limit float64, now func() time.Time) *Bucket {
	if now == nil {
		now = time.Now
	}
	return &Bucket{limit: limit, now: now, tokens: 1, last: now(), base: limit}
}

// Wait blocks until the caller may make one request, or ctx ends.
func (b *Bucket) Wait(ctx context.Context) error {
	d := b.Reserve()
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Reserve takes a token and returns how long the caller must wait before
// using it.
func (b *Bucket) Reserve() time.Duration {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now()
	rate := b.rateAt(now)
	b.tokens = math.Min(1, b.tokens+now.Sub(b.last).Seconds()*rate)
	b.last = now
	b.tokens--
	if b.tokens >= 0 {
		return 0
	}
	return time.Duration(math.Round(-b.tokens / rate * float64(time.Second)))
}

// Observe429 reports that the Provider throttled a request on this Channel.
func (b *Bucket) Observe429() {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now()
	if b.lastCut.IsZero() || now.Sub(b.lastCut) >= cooldown {
		b.base = math.Max(minRate, b.rateAt(now)/2)
		b.lastCut = now
	}
	b.last429 = now
}

// Rate is the current rate in requests per second.
func (b *Bucket) Rate() float64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.rateAt(b.now())
}

func (b *Bucket) rateAt(now time.Time) float64 {
	if b.last429.IsZero() {
		return b.base
	}
	quiet := now.Sub(b.last429) - cooldown
	if quiet < 0 {
		return b.base
	}
	return math.Min(b.limit, b.base+b.limit/recoverySteps*quiet.Seconds())
}
