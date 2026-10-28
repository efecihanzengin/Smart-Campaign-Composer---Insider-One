package worker_test

import (
	"testing"
	"time"

	"campaigncomposer/internal/worker"
)

// These tests run on the real clock and only look at what happens within a
// few milliseconds; how the rate falls under a stream of 429s and recovers
// over seconds is covered through HTTP in httpapi's rate-limit tests.

// near reports whether got is within a few milliseconds of want, the time
// the test itself takes between takes.
func near(got, want time.Duration) bool {
	d := got - want
	return d > -5*time.Millisecond && d < 5*time.Millisecond
}

func TestBucketPacesTakesAtTheRate(t *testing.T) {
	b := worker.NewBucket(10)
	// Taken back to back, the n-th take waits (n-1) tenths of a second: no
	// burst beyond one token.
	for i, want := range []time.Duration{0, 100 * time.Millisecond, 200 * time.Millisecond, 300 * time.Millisecond} {
		if got := b.Reserve(); !near(got, want) {
			t.Errorf("take %d waits %v, want %v", i+1, got, want)
		}
	}
}

func TestBucketRate(t *testing.T) {
	tests := []struct {
		name string
		run  func(b *worker.Bucket)
		want float64
	}{
		{"starts at the limit", func(b *worker.Bucket) {}, 100},
		{"a 429 halves it", func(b *worker.Bucket) { b.Observe429() }, 50},
		{"a burst of 429s halves it once", func(b *worker.Bucket) {
			for range 20 {
				b.Observe429()
			}
		}, 50},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := worker.NewBucket(100)
			tt.run(b)
			if got := b.Rate(); got != tt.want {
				t.Errorf("rate = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBucketPacesAtTheLoweredRate(t *testing.T) {
	b := worker.NewBucket(10)
	b.Observe429()
	b.Reserve()
	if got := b.Reserve(); !near(got, 200*time.Millisecond) {
		t.Errorf("second take at 5/s waits %v, want 200ms", got)
	}
}
