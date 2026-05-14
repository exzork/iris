package orchestrator

import (
	"context"
	"testing"
	"time"
)

func TestRateLimiter_AllowsWithinWindow(t *testing.T) {
	limiter := NewChannelRateLimiter(5, 5*time.Second)
	ctx := context.Background()
	channelID := int64(123)

	for i := 0; i < 5; i++ {
		err := limiter.Wait(ctx, channelID)
		if err != nil {
			t.Fatalf("Wait %d failed: %v", i, err)
		}
	}
}

func TestRateLimiter_BlocksOnBurst(t *testing.T) {
	limiter := NewChannelRateLimiter(5, 5*time.Second)
	ctx := context.Background()
	channelID := int64(123)

	timestamps := make([]time.Time, 0)

	for i := 0; i < 7; i++ {
		start := time.Now()
		err := limiter.Wait(ctx, channelID)
		if err != nil {
			t.Fatalf("Wait %d failed: %v", i, err)
		}
		timestamps = append(timestamps, start)
	}

	if len(timestamps) != 7 {
		t.Fatalf("expected 7 timestamps, got %d", len(timestamps))
	}

	for windowStart := 0; windowStart < len(timestamps); windowStart++ {
		windowEnd := windowStart + 5
		if windowEnd > len(timestamps) {
			windowEnd = len(timestamps)
		}

		count := 0
		for i := windowStart; i < windowEnd; i++ {
			if timestamps[i].Sub(timestamps[windowStart]) <= 5*time.Second {
				count++
			}
		}

		if count > 5 {
			t.Errorf("window starting at %d has %d sends in 5s, expected <= 5", windowStart, count)
		}
	}
}

func TestRateLimiter_PerChannelIndependent(t *testing.T) {
	limiter := NewChannelRateLimiter(5, 5*time.Second)
	ctx := context.Background()
	ch1 := int64(100)
	ch2 := int64(200)

	for i := 0; i < 5; i++ {
		err := limiter.Wait(ctx, ch1)
		if err != nil {
			t.Fatalf("ch1 Wait %d failed: %v", i, err)
		}
	}

	for i := 0; i < 5; i++ {
		err := limiter.Wait(ctx, ch2)
		if err != nil {
			t.Fatalf("ch2 Wait %d failed: %v", i, err)
		}
	}
}

func TestRateLimiter_ContextCancellation(t *testing.T) {
	limiter := NewChannelRateLimiter(1, 5*time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	channelID := int64(123)

	err := limiter.Wait(ctx, channelID)
	if err != nil {
		t.Fatalf("first Wait failed: %v", err)
	}

	cancel()

	err = limiter.Wait(ctx, channelID)
	if err == nil {
		t.Fatal("expected context cancellation error, got nil")
	}
}

func TestRateLimiter_RespectsMaxWait(t *testing.T) {
	limiter := NewChannelRateLimiter(1, 30*time.Second)
	limiter.maxWait = 100 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	channelID := int64(123)

	err := limiter.Wait(ctx, channelID)
	if err != nil {
		t.Fatalf("first Wait failed: %v", err)
	}

	start := time.Now()
	err = limiter.Wait(ctx, channelID)
	elapsed := time.Since(start)

	if err != ErrRateLimitExceeded {
		t.Fatalf("expected ErrRateLimitExceeded, got %v", err)
	}

	if elapsed < 80*time.Millisecond || elapsed > 150*time.Millisecond {
		t.Errorf("expected wait ~100ms, got %v", elapsed)
	}
}
