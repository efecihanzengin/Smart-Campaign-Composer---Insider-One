package worker_test

import (
	"testing"
	"time"

	"campaigncomposer/internal/worker"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newBucket(rate float64) (*worker.Bucket, *clock) {
	c := &clock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	return worker.NewBucket(rate, c.now), c
}

func TestBucketPacesTakesAtTheRate(t *testing.T) {
	b, _ := newBucket(10)
	// Without time passing, the n-th take waits (n-1) tenths of a second: no
	// burst beyond one token.
	for i, want := range []time.Duration{0, 100 * time.Millisecond, 200 * time.Millisecond, 300 * time.Millisecond} {
		if got := b.Reserve(); got != want {
			t.Errorf("take %d waits %v, want %v", i+1, got, want)
		}
	}
}

func TestBucketDoesNotSaveUpIdleTimeAsABurst(t *testing.T) {
	b, c := newBucket(10)
	c.advance(time.Minute)
	if got := b.Reserve(); got != 0 {
		t.Errorf("first take after idling waits %v, want 0", got)
	}
	if got := b.Reserve(); got != 100*time.Millisecond {
		t.Errorf("second take after idling waits %v, want 100ms", got)
	}
}

func TestBucketRate(t *testing.T) {
	tests := []struct {
		name string
		run  func(b *worker.Bucket, c *clock)
		want float64
	}{
		{"starts at the limit", func(b *worker.Bucket, c *clock) {}, 100},
		{"a 429 halves it", func(b *worker.Bucket, c *clock) { b.Observe429() }, 50},
		{"a burst of 429s within a second halves it once", func(b *worker.Bucket, c *clock) {
			for range 20 {
				b.Observe429()
				c.advance(10 * time.Millisecond)
			}
		}, 50},
		{"429s that keep coming keep halving it", func(b *worker.Bucket, c *clock) {
			for range 3 {
				b.Observe429()
				c.advance(time.Second)
			}
		}, 12.5},
		{"never drops below 1/s", func(b *worker.Bucket, c *clock) {
			for range 20 {
				b.Observe429()
				c.advance(time.Second)
			}
			b.Observe429()
		}, 1},
		{"does not recover while 429s are recent", func(b *worker.Bucket, c *clock) {
			b.Observe429()
			c.advance(900 * time.Millisecond)
		}, 50},
		{"recovers gradually once 429s stop", func(b *worker.Bucket, c *clock) {
			b.Observe429()
			c.advance(time.Second) // quiet for a second: recovery starts
			c.advance(time.Second) // a fifth of the limit per second
		}, 70},
		{"recovers no higher than the limit", func(b *worker.Bucket, c *clock) {
			b.Observe429()
			c.advance(time.Minute)
		}, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, c := newBucket(100)
			tt.run(b, c)
			if got := b.Rate(); got != tt.want {
				t.Errorf("rate = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBucketPacesAtTheLoweredRate(t *testing.T) {
	b, _ := newBucket(10)
	b.Observe429()
	b.Reserve()
	if got := b.Reserve(); got != 200*time.Millisecond {
		t.Errorf("second take at 5/s waits %v, want 200ms", got)
	}
}
