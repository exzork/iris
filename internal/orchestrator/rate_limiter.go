package orchestrator

import (
	"context"
	"errors"
	"sync"
	"time"
)

var ErrRateLimitExceeded = errors.New("rate limit exceeded: max wait time exceeded")

type channelBucket struct {
	timestamps []time.Time
}

type ChannelRateLimiter struct {
	maxPerWindow int
	window       time.Duration
	perChannel   map[int64]*channelBucket
	mu           sync.Mutex
	maxWait      time.Duration
}

func NewChannelRateLimiter(maxPerWindow int, window time.Duration) *ChannelRateLimiter {
	return &ChannelRateLimiter{
		maxPerWindow: maxPerWindow,
		window:       window,
		perChannel:   make(map[int64]*channelBucket),
		maxWait:      10 * time.Second,
	}
}

func (l *ChannelRateLimiter) Wait(ctx context.Context, channelID int64) error {
	deadline := time.Now().Add(l.maxWait)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}

	for {
		l.mu.Lock()
		bucket, ok := l.perChannel[channelID]
		if !ok {
			bucket = &channelBucket{}
			l.perChannel[channelID] = bucket
		}

		now := time.Now()

		// Evict timestamps older than window
		cutoff := now.Add(-l.window)
		var kept []time.Time
		for _, ts := range bucket.timestamps {
			if ts.After(cutoff) {
				kept = append(kept, ts)
			}
		}
		bucket.timestamps = kept

		// Check if we can send now
		if len(bucket.timestamps) < l.maxPerWindow {
			bucket.timestamps = append(bucket.timestamps, now)
			l.mu.Unlock()
			return nil
		}

		// Compute sleep duration: oldest timestamp + window - now
		oldestTS := bucket.timestamps[0]
		sleepDuration := oldestTS.Add(l.window).Sub(now)
		l.mu.Unlock()

		// Check if we've exceeded maxWait deadline
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return ErrRateLimitExceeded
		}

		// Clamp sleep to the smaller of sleepDuration or remaining time
		wait := sleepDuration
		if remaining < wait {
			wait = remaining
		}

		// Wait or check context
		select {
		case <-time.After(wait):
			// Loop and try again
			continue
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
