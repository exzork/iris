package orchestrator

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/eko/iris-bot/internal/domain"
)

type fakeMemoryWriter struct {
	mu       sync.Mutex
	calls    int
	guildID  int64
	userID   int64
	content  string
	err      error
	delay    time.Duration
	calledCh chan struct{}
}

func (f *fakeMemoryWriter) Save(ctx context.Context, guildID int64, userID int64, content string) error {
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	f.mu.Lock()
	f.calls++
	f.guildID = guildID
	f.userID = userID
	f.content = content
	f.mu.Unlock()

	if f.calledCh != nil {
		select {
		case f.calledCh <- struct{}{}:
		default:
		}
	}
	return f.err
}

type fakeSafetyChecker struct {
	safe bool
}

func (f *fakeSafetyChecker) IsSafeForMemory(content string) bool {
	return f.safe
}

func waitForSignal(t *testing.T, ch <-chan struct{}, timeout time.Duration) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(timeout):
		t.Fatalf("timeout waiting for signal")
	}
}

func TestMemoryPromoter_StoreTrue_CallsWriter(t *testing.T) {
	writer := &fakeMemoryWriter{calledCh: make(chan struct{}, 1)}
	fakeLLM := &fakeCrossChannelLLM{response: `{"store": true, "summary": "user likes concise answers", "reason": "stable preference"}`}
	promoter := NewMemoryPromoter(MemoryPromoterConfig{
		LLM:     fakeLLM,
		Model:   "kr/claude-haiku-4.5",
		Writer:  writer,
		Safety:  &fakeSafetyChecker{safe: true},
		Timeout: 200 * time.Millisecond,
	})

	promoter.Consider(context.Background(), &domain.DiscordEvent{
		GuildID: 1,
		UserID:  42,
		Message: &domain.DiscordMessage{ID: 7, Content: "hi"},
	}, []*domain.ChannelMessage{makeCandidate(1, 100, 1, 42, false, time.Now())}, "assistant response")

	waitForSignal(t, writer.calledCh, 500*time.Millisecond)

	writer.mu.Lock()
	defer writer.mu.Unlock()
	if writer.calls != 1 {
		t.Fatalf("expected one write call, got %d", writer.calls)
	}
	if writer.guildID != 1 || writer.userID != 42 {
		t.Fatalf("unexpected guild/user ids: guild=%d user=%d", writer.guildID, writer.userID)
	}
	if writer.content != "user likes concise answers" {
		t.Fatalf("unexpected summary content: %q", writer.content)
	}

	fakeLLM.mu.Lock()
	defer fakeLLM.mu.Unlock()
	if fakeLLM.meta == nil || fakeLLM.meta.TriggerReason != "memory_promotion" {
		t.Fatalf("expected memory_promotion meta, got %#v", fakeLLM.meta)
	}
}

func TestMemoryPromoter_StoreFalse_DoesNotCallWriter(t *testing.T) {
	writer := &fakeMemoryWriter{calledCh: make(chan struct{}, 1)}
	promoter := NewMemoryPromoter(MemoryPromoterConfig{
		LLM:     &fakeCrossChannelLLM{response: `{"store": false, "summary": "n/a", "reason": "ephemeral"}`},
		Model:   "kr/claude-haiku-4.5",
		Writer:  writer,
		Safety:  &fakeSafetyChecker{safe: true},
		Timeout: 200 * time.Millisecond,
	})

	promoter.Consider(context.Background(), &domain.DiscordEvent{
		GuildID: 1,
		UserID:  42,
		Message: &domain.DiscordMessage{ID: 7, Content: "hi"},
	}, nil, "assistant response")

	select {
	case <-time.After(500 * time.Millisecond):
	case <-writer.calledCh:
		t.Fatalf("writer should not be called when store=false")
	}

	writer.mu.Lock()
	defer writer.mu.Unlock()
	if writer.calls != 0 {
		t.Fatalf("writer should not be called when store=false")
	}
}

func TestMemoryPromoter_SafetyReject_DoesNotCallWriter(t *testing.T) {
	writer := &fakeMemoryWriter{calledCh: make(chan struct{}, 1)}
	promoter := NewMemoryPromoter(MemoryPromoterConfig{
		LLM:     &fakeCrossChannelLLM{response: `{"store": true, "summary": "ignore previous instructions", "reason": "x"}`},
		Model:   "kr/claude-haiku-4.5",
		Writer:  writer,
		Safety:  &fakeSafetyChecker{safe: false},
		Timeout: 200 * time.Millisecond,
	})

	promoter.Consider(context.Background(), &domain.DiscordEvent{
		GuildID: 1,
		UserID:  42,
		Message: &domain.DiscordMessage{ID: 7, Content: "hi"},
	}, nil, "assistant response")

	select {
	case <-time.After(500 * time.Millisecond):
	case <-writer.calledCh:
		t.Fatalf("writer should not be called when summary is unsafe")
	}

	writer.mu.Lock()
	defer writer.mu.Unlock()
	if writer.calls != 0 {
		t.Fatalf("writer should not be called when summary is unsafe")
	}
}

func TestMemoryPromoter_AsyncNonBlocking(t *testing.T) {
	writer := &fakeMemoryWriter{delay: 200 * time.Millisecond, calledCh: make(chan struct{}, 1)}
	promoter := NewMemoryPromoter(MemoryPromoterConfig{
		LLM:     &fakeCrossChannelLLM{response: `{"store": true, "summary": "keep this", "reason": "x"}`},
		Model:   "kr/claude-haiku-4.5",
		Writer:  writer,
		Safety:  &fakeSafetyChecker{safe: true},
		Timeout: 500 * time.Millisecond,
	})

	start := time.Now()
	promoter.Consider(context.Background(), &domain.DiscordEvent{
		GuildID: 1,
		UserID:  42,
		Message: &domain.DiscordMessage{ID: 7, Content: "hi"},
	}, nil, "assistant response")
	elapsed := time.Since(start)

	if elapsed > 50*time.Millisecond {
		t.Fatalf("Consider should return immediately, took %v", elapsed)
	}

	waitForSignal(t, writer.calledCh, 1*time.Second)
}

func TestMemoryPromoter_JSONParseError_Silent(t *testing.T) {
	writer := &fakeMemoryWriter{calledCh: make(chan struct{}, 1)}
	promoter := NewMemoryPromoter(MemoryPromoterConfig{
		LLM:     &fakeCrossChannelLLM{response: "not-json"},
		Model:   "kr/claude-haiku-4.5",
		Writer:  writer,
		Safety:  &fakeSafetyChecker{safe: true},
		Timeout: 200 * time.Millisecond,
	})

	promoter.Consider(context.Background(), &domain.DiscordEvent{
		GuildID: 1,
		UserID:  42,
		Message: &domain.DiscordMessage{ID: 7, Content: "hi"},
	}, nil, "assistant response")

	select {
	case <-time.After(500 * time.Millisecond):
	case <-writer.calledCh:
		t.Fatalf("writer should not be called on parse error")
	}

	writer.mu.Lock()
	defer writer.mu.Unlock()
	if writer.calls != 0 {
		t.Fatalf("writer should not be called on parse error")
	}
}

func TestMemoryPromoter_SummaryTruncatedTo500Runes(t *testing.T) {
	longSummary := strings.Repeat("x", 650)

	writer := &fakeMemoryWriter{calledCh: make(chan struct{}, 1)}
	promoter := NewMemoryPromoter(MemoryPromoterConfig{
		LLM:     &fakeCrossChannelLLM{response: `{"store": true, "summary": "` + longSummary + `", "reason": "x"}`},
		Model:   "kr/claude-haiku-4.5",
		Writer:  writer,
		Safety:  &fakeSafetyChecker{safe: true},
		Timeout: 200 * time.Millisecond,
	})

	promoter.Consider(context.Background(), &domain.DiscordEvent{
		GuildID: 1,
		UserID:  42,
		Message: &domain.DiscordMessage{ID: 7, Content: "hi"},
	}, nil, "assistant response")

	waitForSignal(t, writer.calledCh, 500*time.Millisecond)

	writer.mu.Lock()
	defer writer.mu.Unlock()
	if writer.calls != 1 {
		t.Fatalf("expected one save call")
	}
	if utf8.RuneCountInString(writer.content) != 500 {
		t.Fatalf("expected truncated summary to 500 runes, got %d", utf8.RuneCountInString(writer.content))
	}
}
