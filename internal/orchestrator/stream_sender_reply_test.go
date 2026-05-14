package orchestrator

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

type mockReplyEditSender struct {
	mu       sync.Mutex
	sends    []string
	edits    []struct {
		id      int64
		content string
	}
	replies  []recordedReply
	nextID   int64
}

type recordedReply struct {
	replyToMessageID   int64
	mentionRepliedUser bool
	content            string
}

func (m *mockReplyEditSender) SendMessage(ctx context.Context, guildID, channelID int64, content string) error {
	_, err := m.SendMessageReturningID(ctx, guildID, channelID, content)
	return err
}

func (m *mockReplyEditSender) SendMessageReturningID(ctx context.Context, guildID, channelID int64, content string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sends = append(m.sends, content)
	m.nextID++
	return m.nextID, nil
}

func (m *mockReplyEditSender) EditMessage(ctx context.Context, guildID, channelID, messageID int64, content string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.edits = append(m.edits, struct {
		id      int64
		content string
	}{messageID, content})
	return nil
}

func (m *mockReplyEditSender) ReplyMessageReturningID(ctx context.Context, guildID, channelID, replyToMessageID int64, content string, mentionRepliedUser bool) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.replies = append(m.replies, recordedReply{
		replyToMessageID:   replyToMessageID,
		mentionRepliedUser: mentionRepliedUser,
		content:            content,
	})
	m.nextID++
	return m.nextID, nil
}

func TestStreamingSender_FirstMessageRepliesWithPing(t *testing.T) {
	mock := &mockReplyEditSender{}
	limiter := NewChannelRateLimiter(100, 5*time.Second)
	sender := NewStreamingSender(mock, limiter, 1, 2)
	sender.WithReply(99999, true)
	ctx := context.Background()

	if err := sender.Push(ctx, "first paragraph here\n\n"); err != nil {
		t.Fatalf("Push: %v", err)
	}
	if err := sender.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()

	if len(mock.replies) != 1 {
		t.Fatalf("expected exactly 1 reply send, got %d", len(mock.replies))
	}
	got := mock.replies[0]
	if got.replyToMessageID != 99999 {
		t.Errorf("replyToMessageID = %d, want 99999", got.replyToMessageID)
	}
	if !got.mentionRepliedUser {
		t.Errorf("expected ping enabled")
	}
	if len(mock.sends) > 0 {
		t.Errorf("first message should be a reply, not a regular send; got %d regular sends", len(mock.sends))
	}
}

func TestStreamingSender_FollowUpsDoNotPingAgain(t *testing.T) {
	mock := &mockReplyEditSender{}
	limiter := NewChannelRateLimiter(100, 5*time.Second)
	sender := NewStreamingSender(mock, limiter, 1, 2)
	sender.WithReply(54321, true)
	ctx := context.Background()

	long := strings.Repeat("a", DiscordMessageLimit) + strings.Repeat("b", 200)
	if err := sender.Push(ctx, long+"\n\n"); err != nil {
		t.Fatalf("Push: %v", err)
	}
	if err := sender.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()

	if len(mock.replies) != 1 {
		t.Fatalf("expected exactly 1 reply (only the first message), got %d", len(mock.replies))
	}
	if len(mock.sends) == 0 {
		t.Errorf("expected follow-up regular sends after first reply")
	}
}

func TestStreamingSender_FallbackToPlainSendWhenNoReplyInterface(t *testing.T) {
	mock := &mockSender{}
	limiter := NewChannelRateLimiter(100, 5*time.Second)
	sender := NewStreamingSender(mock, limiter, 1, 2)
	sender.WithReply(11111, true)
	ctx := context.Background()

	if err := sender.Push(ctx, "fallback content\n\n"); err != nil {
		t.Fatalf("Push: %v", err)
	}
	if err := sender.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()

	if len(mock.messages) == 0 {
		t.Fatalf("expected at least one plain send when reply interface absent")
	}
	if mock.messages[0] != "fallback content\n\n" {
		t.Errorf("plain fallback content mismatch: %q", mock.messages[0])
	}
}

func TestStreamingSender_WithReplyAfterSentIsNoOp(t *testing.T) {
	mock := &mockReplyEditSender{}
	limiter := NewChannelRateLimiter(100, 5*time.Second)
	sender := NewStreamingSender(mock, limiter, 1, 2)
	ctx := context.Background()

	if err := sender.Push(ctx, "before reply config\n\n"); err != nil {
		t.Fatalf("Push: %v", err)
	}
	if err := sender.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	sender.WithReply(77777, true)

	if err := sender.Push(ctx, "after reply config\n\n"); err != nil {
		t.Fatalf("Push: %v", err)
	}
	if err := sender.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if len(mock.replies) != 0 {
		t.Errorf("WithReply called mid-stream must not produce reply sends; got %+v", mock.replies)
	}
}

func TestStreamingSender_OutboundTransformAppliedToFirstReply(t *testing.T) {
	mock := &mockReplyEditSender{}
	limiter := NewChannelRateLimiter(100, 5*time.Second)
	sender := NewStreamingSender(mock, limiter, 1, 2)
	sender.WithReply(99999, true)

	knownIDs := map[int64]bool{
		123456789012345678: true,
	}
	sender.WithOutboundTransform(func(s string) string {
		return scrubRawUserIDs(s, knownIDs)
	})

	ctx := context.Background()

	if err := sender.Push(ctx, "User 123456789012345678 is here\n\n"); err != nil {
		t.Fatalf("Push: %v", err)
	}
	if err := sender.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()

	if len(mock.replies) != 1 {
		t.Fatalf("expected exactly 1 reply, got %d", len(mock.replies))
	}
	got := mock.replies[0].content
	expected := "User <@123456789012345678> is here\n\n"
	if got != expected {
		t.Errorf("reply content mismatch:\n  got:      %q\n  expected: %q", got, expected)
	}
}

func TestStreamingSender_OutboundTransformAppliedToEdits(t *testing.T) {
	mock := &mockReplyEditSender{}
	limiter := NewChannelRateLimiter(100, 5*time.Second)
	sender := NewStreamingSender(mock, limiter, 1, 2)
	sender.WithReply(99999, true)

	knownIDs := map[int64]bool{
		987654321098765432: true,
	}
	sender.WithOutboundTransform(func(s string) string {
		return scrubRawUserIDs(s, knownIDs)
	})

	ctx := context.Background()

	if err := sender.Push(ctx, "First chunk with 987654321098765432"); err != nil {
		t.Fatalf("Push: %v", err)
	}
	if err := sender.Push(ctx, " and more text"); err != nil {
		t.Fatalf("Push: %v", err)
	}
	if err := sender.Push(ctx, " and even more\n\n"); err != nil {
		t.Fatalf("Push: %v", err)
	}
	if err := sender.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()

	if len(mock.replies) == 0 {
		t.Fatalf("expected at least one reply, got %d", len(mock.replies))
	}

	replyContent := mock.replies[0].content
	if !strings.Contains(replyContent, "<@987654321098765432>") {
		t.Errorf("reply content should contain scrubbed mention, got: %q", replyContent)
	}
	if strings.Contains(replyContent, "987654321098765432 and") {
		t.Errorf("reply content should not contain raw id, got: %q", replyContent)
	}
}

func TestStreamingSender_OutboundTransformPreservesExistingMentions(t *testing.T) {
	mock := &mockReplyEditSender{}
	limiter := NewChannelRateLimiter(100, 5*time.Second)
	sender := NewStreamingSender(mock, limiter, 1, 2)
	sender.WithReply(99999, true)

	knownIDs := map[int64]bool{
		111111111111111111: true,
	}
	sender.WithOutboundTransform(func(s string) string {
		return scrubRawUserIDs(s, knownIDs)
	})

	ctx := context.Background()

	if err := sender.Push(ctx, "Mention <@111111111111111111> stays as-is\n\n"); err != nil {
		t.Fatalf("Push: %v", err)
	}
	if err := sender.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()

	if len(mock.replies) != 1 {
		t.Fatalf("expected exactly 1 reply, got %d", len(mock.replies))
	}
	got := mock.replies[0].content
	expected := "Mention <@111111111111111111> stays as-is\n\n"
	if got != expected {
		t.Errorf("reply content mismatch:\n  got:      %q\n  expected: %q", got, expected)
	}
}

func TestStreamingSender_NilOutboundTransformDoesNotCrash(t *testing.T) {
	mock := &mockReplyEditSender{}
	limiter := NewChannelRateLimiter(100, 5*time.Second)
	sender := NewStreamingSender(mock, limiter, 1, 2)
	sender.WithReply(99999, true)

	ctx := context.Background()

	if err := sender.Push(ctx, "Content with 123456789012345678 raw id\n\n"); err != nil {
		t.Fatalf("Push: %v", err)
	}
	if err := sender.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()

	if len(mock.replies) != 1 {
		t.Fatalf("expected exactly 1 reply, got %d", len(mock.replies))
	}
	got := mock.replies[0].content
	expected := "Content with 123456789012345678 raw id\n\n"
	if got != expected {
		t.Errorf("reply content should be unchanged when transform is nil:\n  got:      %q\n  expected: %q", got, expected)
	}
}

func TestStreamingSender_OutboundTransformChainable(t *testing.T) {
	mock := &mockReplyEditSender{}
	limiter := NewChannelRateLimiter(100, 5*time.Second)

	knownIDs := map[int64]bool{
		222222222222222222: true,
	}

	sender := NewStreamingSender(mock, limiter, 1, 2).
		WithReply(99999, true).
		WithOutboundTransform(func(s string) string {
			return scrubRawUserIDs(s, knownIDs)
		})

	ctx := context.Background()

	if err := sender.Push(ctx, "User 222222222222222222 here\n\n"); err != nil {
		t.Fatalf("Push: %v", err)
	}
	if err := sender.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()

	if len(mock.replies) != 1 {
		t.Fatalf("expected exactly 1 reply, got %d", len(mock.replies))
	}
	got := mock.replies[0].content
	if !strings.Contains(got, "<@222222222222222222>") {
		t.Errorf("chained transform should work, got: %q", got)
	}
}
