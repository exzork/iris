package wire

import (
	"context"
	"sync"
	"time"

	"github.com/eko/iris-bot/internal/lorethread"
)

type hourlyBucket struct {
	hour  time.Time
	count int
}

// HourlyLimiter enforces an in-memory per-guild hourly cap.
type HourlyLimiter struct {
	capPerHour int
	clock      lorethread.Clock

	mu      sync.Mutex
	buckets map[int64]hourlyBucket
}

// NewHourlyLimiter creates a thread-safe in-memory limiter.
func NewHourlyLimiter(capPerHour int, clock lorethread.Clock) *HourlyLimiter {
	if capPerHour <= 0 {
		capPerHour = 1
	}
	if clock == nil {
		clock = &lorethread.RealClock{}
	}

	return &HourlyLimiter{
		capPerHour: capPerHour,
		clock:      clock,
		buckets:    make(map[int64]hourlyBucket),
	}
}

// Allow returns true when the guild is still under its current hour cap.
func (l *HourlyLimiter) Allow(ctx context.Context, guildID int64) bool {
	_ = ctx

	nowHour := l.clock.Now().UTC().Truncate(time.Hour)

	l.mu.Lock()
	defer l.mu.Unlock()

	bucket := l.buckets[guildID]
	if bucket.hour.IsZero() || !bucket.hour.Equal(nowHour) {
		bucket = hourlyBucket{hour: nowHour, count: 0}
	}

	if bucket.count >= l.capPerHour {
		l.buckets[guildID] = bucket
		return false
	}

	bucket.count++
	l.buckets[guildID] = bucket
	return true
}

// Reset clears all limiter state.
func (l *HourlyLimiter) Reset(ctx context.Context, guildID int64) error {
	_ = ctx
	_ = guildID

	l.mu.Lock()
	defer l.mu.Unlock()

	l.buckets = make(map[int64]hourlyBucket)
	return nil
}
