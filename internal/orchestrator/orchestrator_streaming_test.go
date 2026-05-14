package orchestrator

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/eko/iris-bot/internal/llm"
)

type fakeStreamLLM struct {
	paragraphs []string
}

func (f *fakeStreamLLM) ChatStream(ctx context.Context, model string, guildID int64, messages []map[string]string, cb llm.StreamCallbacks) (string, error) {
	fullText := ""
	for _, para := range f.paragraphs {
		fullText += para
		cb.OnDelta(para)
	}
	cb.OnDone()
	return fullText, nil
}

type fakeStreamToolsLLM struct {
	response string
}

func (f *fakeStreamToolsLLM) ChatWithToolsStream(ctx context.Context, messages []map[string]string, cfg llm.ChatWithToolsStreamConfig) (string, error) {
	cfg.OnDelta(f.response)
	return f.response, nil
}

type testSender struct {
	mu       sync.Mutex
	messages []string
}

func (t *testSender) SendMessage(ctx context.Context, guildID, channelID int64, content string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.messages = append(t.messages, content)
	return nil
}

func TestHandle_StreamingPath_EditsActiveWhenUnderLimit(t *testing.T) {
	mock := &mockEditSender{}
	limiter := NewChannelRateLimiter(100, 5*time.Second)

	streamingSender := NewStreamingSender(mock, limiter, 1, 2)
	ctx := context.Background()

	para := "This is a long paragraph that needs to exceed 200 bytes to trigger the boundary flush. "
	for len(para) < 250 {
		para += "x"
	}

	streamingSender.Push(ctx, para)
	streamingSender.Push(ctx, "\n\n")
	streamingSender.Push(ctx, para)
	streamingSender.Push(ctx, "\n\n")
	streamingSender.Flush(ctx)

	if len(mock.sends) != 1 {
		t.Fatalf("expected exactly 1 send (paragraphs fit under Discord limit), got %d", len(mock.sends))
	}
}

func TestHandle_StreamingPath_FallsBackWhenDisabled(t *testing.T) {
	sender := &testSender{}
	limiter := NewChannelRateLimiter(100, 5*time.Second)

	streamingSender := NewStreamingSender(sender, limiter, 1, 2)
	ctx := context.Background()

	streamingSender.Push(ctx, "Hello world")
	streamingSender.Flush(ctx)

	if len(sender.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(sender.messages))
	}
	if sender.messages[0] != "Hello world" {
		t.Errorf("expected 'Hello world', got %q", sender.messages[0])
	}
}

func TestStreamingSender_MultipleChannels(t *testing.T) {
	sender := &testSender{}
	limiter := NewChannelRateLimiter(100, 5*time.Second)
	ctx := context.Background()

	sender1 := NewStreamingSender(sender, limiter, 1, 100)
	sender2 := NewStreamingSender(sender, limiter, 1, 200)

	para := "This is a paragraph that will trigger soft threshold. "
	for len(para) < 300 {
		para += "x"
	}

	for i := 0; i < 3; i++ {
		sender1.Push(ctx, para)
		sender1.Push(ctx, "\n\n")
	}

	for i := 0; i < 3; i++ {
		sender2.Push(ctx, para)
		sender2.Push(ctx, "\n\n")
	}

	sender1.Flush(ctx)
	sender2.Flush(ctx)

	if len(sender.messages) < 6 {
		t.Fatalf("expected at least 6 messages, got %d", len(sender.messages))
	}
}

func TestHandle_StreamingPath_CoalescesUnderLimit(t *testing.T) {
	mock := &mockEditSender{}
	limiter := NewChannelRateLimiter(100, 5*time.Second)

	para := "This is a long paragraph that needs to exceed 200 bytes to trigger the boundary flush. "
	for len(para) < 250 {
		para += "x"
	}

	streamLLM := &fakeStreamLLM{
		paragraphs: []string{
			para,
			"\n\n",
			para,
			"\n\n",
			para,
		},
	}

	ctx := context.Background()

	streamingSender := NewStreamingSender(mock, limiter, 1, 2)
	callbacks := llm.StreamCallbacks{
		OnDelta: func(text string) {
			_ = streamingSender.Push(ctx, text)
		},
		OnDone: func() {
			_ = streamingSender.Flush(ctx)
		},
		OnError: func(err error) {},
	}

	resp, err := streamLLM.ChatStream(ctx, "test-model", 1, nil, callbacks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mock.sends) != 1 {
		t.Fatalf("expected exactly 1 send (three paragraphs coalesce via edit), got %d", len(mock.sends))
	}

	if resp == "" {
		t.Errorf("expected non-empty response")
	}
}
