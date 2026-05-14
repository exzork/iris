package browser

import (
	"sync"
	"time"
)

// Limiter enforces per-host rate limiting using a token bucket algorithm.
type Limiter struct {
	mu       sync.Mutex
	buckets  map[string]*bucket
	interval time.Duration
	burst    int
}

type bucket struct {
	tokens    int
	lastReset time.Time
}

// NewLimiter returns a limiter with `burst` tokens per `interval` per host.
func NewLimiter(interval time.Duration, burst int) *Limiter {
	return &Limiter{
		buckets:  make(map[string]*bucket),
		interval: interval,
		burst:    burst,
	}
}

// Allow returns true if a token was consumed for host, false if empty.
func (l *Limiter) Allow(host string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	b, ok := l.buckets[host]
	if !ok {
		b = &bucket{tokens: l.burst - 1, lastReset: now}
		l.buckets[host] = b
		return true
	}

	elapsed := now.Sub(b.lastReset)
	if elapsed >= l.interval {
		b.tokens = l.burst - 1
		b.lastReset = now
		return true
	}

	if b.tokens > 0 {
		b.tokens--
		return true
	}

	return false
}
