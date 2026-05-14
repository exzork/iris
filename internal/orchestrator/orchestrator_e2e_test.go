package orchestrator

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/eko/iris-bot/internal/domain"
	"github.com/eko/iris-bot/internal/router"
	"github.com/eko/iris-bot/internal/testutil"
)

type fakeContextStore struct {
	messages []*domain.ChannelMessage
}

func (f *fakeContextStore) ListRecent(ctx context.Context, guildID, channelID int64, limit int) ([]*domain.ChannelMessage, error) {
	return f.messages, nil
}

func (f *fakeContextStore) GetByID(ctx context.Context, guildID, messageID int64) (*domain.ChannelMessage, error) {
	for _, msg := range f.messages {
		if msg.ID == messageID {
			return msg, nil
		}
	}
	return nil, nil
}

func (f *fakeContextStore) ListByUserAcrossChannels(ctx context.Context, guildID int64, userID int64, sinceMinutes int, limit int) ([]*domain.ChannelMessage, error) {
	return nil, nil
}

type fakeTypingRecorder struct {
	typingSends int
}

func (f *fakeTypingRecorder) SendTyping(ctx context.Context, guildID, channelID int64) error {
	f.typingSends++
	return nil
}

type fakeSendRecorder struct {
	mu          sync.Mutex
	chunks      []string
	typingSends int
}

func (f *fakeSendRecorder) SendMessage(ctx context.Context, guildID, channelID int64, content string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.chunks = append(f.chunks, content)
	return nil
}

func (f *fakeSendRecorder) SendTyping(ctx context.Context, guildID, channelID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.typingSends++
	return nil
}

func (f *fakeSendRecorder) Chunks() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	result := make([]string, len(f.chunks))
	copy(result, f.chunks)
	return result
}

func (f *fakeSendRecorder) ChunkCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.chunks)
}

func TestE2EDebugAuditAndContext(t *testing.T) {
	ctx := context.Background()

	contextStore := &fakeContextStore{
		messages: make([]*domain.ChannelMessage, 0),
	}

	for i := 0; i < 12; i++ {
		msg := &domain.ChannelMessage{
			ID:        int64(100 + i),
			GuildID:   1,
			ChannelID: 1,
			UserID:    int64(200 + i),
			AuthorName: func() *string {
				s := "User" + string(rune(48+i))
				return &s
			}(),
			Content:       "context message " + string(rune(48+i)),
			TriggerSource: "observe",
			CreatedAt:     time.Now().Add(-time.Duration(12-i) * time.Minute),
		}
		contextStore.messages = append(contextStore.messages, msg)
	}

	llm := testutil.NewFakeLLMClient()
	llm.ChatResponses[""] = "This is a test response from the LLM."
	llm.SimulateLatency = 100 * time.Millisecond

	sendRecorder := &fakeSendRecorder{}

	router := &stubRouter{decision: router.Respond(router.ReasonMention)}

	orch := New(Config{
		Router:       router,
		LLM:          llm,
		Discord:      sendRecorder,
		QueueSize:    10,
		WorkerCount:  1,
		EnqueueLimit: 50 * time.Millisecond,
		DedupeTTL:    30 * time.Second,
		TypingAfter:  100 * time.Millisecond,
		TypingRepeat: 500 * time.Millisecond,
		JobTimeout:   10 * time.Second,
		SystemPrompt: "You are a helpful assistant.",
	})

	orch.Start()
	defer orch.Stop()

	event := &domain.DiscordEvent{
		Type:      "message_mention",
		GuildID:   1,
		ChannelID: 1,
		UserID:    42,
		Message: &domain.DiscordMessage{
			ID:      hashID("test-msg-1"),
			GuildID: 1,
			Content: "Hello, what do you think?",
		},
		CreatedAt: time.Now(),
	}

	err := orch.Enqueue(ctx, event)
	if err != nil {
		t.Fatalf("failed to enqueue event: %v", err)
	}

	waitUntil(t, 2*time.Second, func() bool {
		return sendRecorder.ChunkCount() > 0
	})

	if sendRecorder.ChunkCount() == 0 {
		t.Error("expected at least one message sent")
	}

	for _, chunk := range sendRecorder.Chunks() {
		if len(chunk) > 2000 {
			t.Errorf("chunk exceeds 2000 chars: %d", len(chunk))
		}
	}

	if sendRecorder.typingSends == 0 {
		t.Error("expected at least one typing indicator sent")
	}

	if orch.processed.Load() == 0 {
		t.Error("expected at least one message processed")
	}
}
