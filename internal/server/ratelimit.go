package server

import (
	"math"
	"sync"
	"time"
)

// limiter is a per-key token bucket: each key gets `burst` tokens up front and
// earns `rate` more per second, up to the burst size. A request costs one token.
//
// Token buckets allow short bursts (a page load firing several requests) while
// still capping the long-run rate, which is why they are the usual choice.
type limiter struct {
	rate  float64 // tokens per second
	burst float64
	now   func() time.Time // injectable so tests don't sleep

	mu      sync.Mutex // guards buckets; handlers run on many goroutines at once
	buckets map[string]*bucket
}

type bucket struct {
	tokens float64
	last   time.Time
}

func newLimiter(rate float64, burst int) *limiter {
	return &limiter{rate: rate, burst: float64(burst), now: time.Now, buckets: map[string]*bucket{}}
}

// Allow spends a token for key. When none is left it reports how long until one is.
func (l *limiter) Allow(key string) (ok bool, retryAfter time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	b, found := l.buckets[key]
	if !found {
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[key] = b
	}
	b.tokens = math.Min(l.burst, b.tokens+now.Sub(b.last).Seconds()*l.rate)
	b.last = now

	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	wait := time.Duration((1 - b.tokens) / l.rate * float64(time.Second))
	return false, wait
}

// Sweep forgets keys that have been idle long enough to have fully refilled, so
// the map cannot grow without bound as new IPs arrive.
func (l *limiter) Sweep(idle time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := l.now().Add(-idle)
	for k, b := range l.buckets {
		if b.last.Before(cutoff) {
			delete(l.buckets, k)
		}
	}
}

// Run sweeps every interval until stop is closed. Start it with `go l.Run(...)`.
func (l *limiter) Run(interval, idle time.Duration, stop <-chan struct{}) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			l.Sweep(idle)
		case <-stop:
			return
		}
	}
}
