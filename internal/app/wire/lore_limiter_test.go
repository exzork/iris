package wire

import (
	"context"
	"testing"
	"time"

	"github.com/eko/iris-bot/internal/lorethread"
)

func TestHourlyLimiter_PerHourReset(t *testing.T) {
	start := time.Date(2026, 5, 13, 10, 15, 0, 0, time.UTC)
	clock := lorethread.NewFakeClock(start)
	limiter := NewHourlyLimiter(2, clock)
	ctx := context.Background()

	guildID := int64(123)

	if !limiter.Allow(ctx, guildID) {
		t.Fatal("expected first allow to pass")
	}
	if !limiter.Allow(ctx, guildID) {
		t.Fatal("expected second allow to pass")
	}
	if limiter.Allow(ctx, guildID) {
		t.Fatal("expected third allow in same hour to be blocked")
	}

	clock.Advance(30 * time.Minute) // still same hour (10:45)
	if limiter.Allow(ctx, guildID) {
		t.Fatal("expected cap to remain blocked within same hour")
	}

	clock.Advance(15 * time.Minute) // next hour (11:00)
	if !limiter.Allow(ctx, guildID) {
		t.Fatal("expected new hour to reset cap")
	}
}
