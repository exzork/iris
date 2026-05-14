package orchestrator

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

type mockSender struct {
	mu       sync.Mutex
	messages []string
	errors   []error
}

func (m *mockSender) SendMessage(ctx context.Context, guildID, channelID int64, content string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, content)
	if len(m.errors) > 0 {
		err := m.errors[0]
		m.errors = m.errors[1:]
		return err
	}
	return nil
}

type mockEditSender struct {
	mu       sync.Mutex
	sends    []string
	edits    []struct {
		id      int64
		content string
	}
	sendErrs []error
	editErrs []error
	nextID   int64
}

func (m *mockEditSender) SendMessage(ctx context.Context, guildID, channelID int64, content string) error {
	_, err := m.SendMessageReturningID(ctx, guildID, channelID, content)
	return err
}

func (m *mockEditSender) SendMessageReturningID(ctx context.Context, guildID, channelID int64, content string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sends = append(m.sends, content)
	if len(m.sendErrs) > 0 {
		err := m.sendErrs[0]
		m.sendErrs = m.sendErrs[1:]
		if err != nil {
			return 0, err
		}
	}
	m.nextID++
	return m.nextID, nil
}

func (m *mockEditSender) EditMessage(ctx context.Context, guildID, channelID, messageID int64, content string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.edits = append(m.edits, struct {
		id      int64
		content string
	}{messageID, content})
	if len(m.editErrs) > 0 {
		err := m.editErrs[0]
		m.editErrs = m.editErrs[1:]
		return err
	}
	return nil
}

func TestStreamingSender_EditsActiveOnParagraphBoundary(t *testing.T) {
	mock := &mockEditSender{}
	limiter := NewChannelRateLimiter(100, 5*time.Second)
	sender := NewStreamingSender(mock, limiter, 1, 2)
	ctx := context.Background()

	longText := "This is a long paragraph that needs to be at least 200 bytes to trigger the paragraph boundary flush. "
	for len(longText) < 200 {
		longText += "x"
	}

	sender.Push(ctx, longText)
	sender.Push(ctx, "\n\n")
	sender.Push(ctx, "Second paragraph")
	sender.Flush(ctx)

	if len(mock.sends) != 1 {
		t.Fatalf("expected exactly 1 message send, got %d", len(mock.sends))
	}
	if len(mock.edits) == 0 {
		t.Fatalf("expected at least one edit to append subsequent paragraphs, got 0")
	}
	last := mock.edits[len(mock.edits)-1].content
	if !strings.Contains(last, "Second paragraph") {
		t.Errorf("expected final edit to contain second paragraph, got %q", last)
	}
	if !strings.Contains(last, "\n\n") {
		t.Errorf("expected final edit to contain paragraph boundary, got %q", last)
	}
}

func TestStreamingSender_FlushesRemainder(t *testing.T) {
	mock := &mockEditSender{}
	limiter := NewChannelRateLimiter(100, 5*time.Second)
	sender := NewStreamingSender(mock, limiter, 1, 2)
	ctx := context.Background()

	sender.Push(ctx, "Hello")
	sender.Push(ctx, " world")
	sender.Flush(ctx)

	if len(mock.sends) != 1 {
		t.Fatalf("expected 1 send after flush, got %d", len(mock.sends))
	}
	if mock.sends[0] != "Hello world" {
		t.Errorf("expected 'Hello world', got %q", mock.sends[0])
	}
}

func TestStreamingSender_StartsNewMessageWhenDiscordLimitReached(t *testing.T) {
	mock := &mockEditSender{}
	limiter := NewChannelRateLimiter(100, 5*time.Second)
	sender := NewStreamingSender(mock, limiter, 1, 2)
	ctx := context.Background()

	big := strings.Repeat("x", 1900)
	sender.Push(ctx, big)
	sender.Push(ctx, "\n\n")
	second := strings.Repeat("y", 300)
	sender.Push(ctx, second)
	sender.Push(ctx, "\n\n")
	sender.Flush(ctx)

	if len(mock.sends) < 2 {
		t.Fatalf("expected at least 2 sends when overflowing Discord limit, got %d", len(mock.sends))
	}
	for _, s := range mock.sends {
		if len(s) > DiscordMessageLimit {
			t.Errorf("send exceeds Discord limit: %d > %d", len(s), DiscordMessageLimit)
		}
	}
	for _, e := range mock.edits {
		if len(e.content) > DiscordMessageLimit {
			t.Errorf("edit exceeds Discord limit: %d > %d", len(e.content), DiscordMessageLimit)
		}
	}
}

func TestStreamingSender_HandlesHugeParagraphSplitsAcrossMessages(t *testing.T) {
	mock := &mockEditSender{}
	limiter := NewChannelRateLimiter(100, 5*time.Second)
	sender := NewStreamingSender(mock, limiter, 1, 2)
	ctx := context.Background()

	huge := strings.Repeat("a", 5000)
	sender.Push(ctx, huge+"\n\n")
	sender.Flush(ctx)

	if len(mock.sends) < 3 {
		t.Fatalf("expected 5000-char paragraph to split into >=3 messages, got %d", len(mock.sends))
	}
	for _, s := range mock.sends {
		if len(s) > DiscordMessageLimit {
			t.Errorf("send exceeds limit: %d", len(s))
		}
	}
	for _, e := range mock.edits {
		if len(e.content) > DiscordMessageLimit {
			t.Errorf("edit exceeds limit: %d", len(e.content))
		}
	}
}

func TestStreamingSender_ClosedDropsSilently(t *testing.T) {
	mock := &mockEditSender{}
	limiter := NewChannelRateLimiter(100, 5*time.Second)
	sender := NewStreamingSender(mock, limiter, 1, 2)
	ctx := context.Background()

	sender.Push(ctx, "First")
	sender.Flush(ctx)

	initialSends := len(mock.sends)
	initialEdits := len(mock.edits)

	sender.Push(ctx, "Second")

	if len(mock.sends) != initialSends || len(mock.edits) != initialEdits {
		t.Errorf("expected no new API calls after close")
	}
}

func TestStreamingSender_SentCountMatchesSends(t *testing.T) {
	mock := &mockEditSender{}
	limiter := NewChannelRateLimiter(100, 5*time.Second)
	sender := NewStreamingSender(mock, limiter, 1, 2)
	ctx := context.Background()

	big := strings.Repeat("z", 1900)
	sender.Push(ctx, big)
	sender.Push(ctx, "\n\n")
	sender.Push(ctx, strings.Repeat("w", 1900))
	sender.Push(ctx, "\n\n")
	sender.Flush(ctx)

	if got, want := sender.SentCount(), len(mock.sends); got != want {
		t.Errorf("SentCount=%d want=%d", got, want)
	}
}

func TestStreamingSender_FallbackToSendWhenNoEditor(t *testing.T) {
	mock := &mockSender{}
	limiter := NewChannelRateLimiter(100, 5*time.Second)
	sender := NewStreamingSender(mock, limiter, 1, 2)
	ctx := context.Background()

	big := "first paragraph " + strings.Repeat("x", 200) + "\n\n"
	sender.Push(ctx, big)
	sender.Push(ctx, "second paragraph")
	sender.Flush(ctx)

	if len(mock.messages) == 0 {
		t.Fatal("expected at least one SendMessage call in fallback mode")
	}
}
