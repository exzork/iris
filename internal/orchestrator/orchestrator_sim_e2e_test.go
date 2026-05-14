package orchestrator

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eko/iris-bot/internal/domain"
	"github.com/eko/iris-bot/internal/embedder"
	"github.com/eko/iris-bot/internal/router"
	"github.com/eko/iris-bot/internal/testutil"
)

type spyEmbedder struct {
	mu       sync.Mutex
	embedMap map[string][]float32
}

func newSpyEmbedder() *spyEmbedder {
	return &spyEmbedder{
		embedMap: make(map[string][]float32),
	}
}

func (s *spyEmbedder) registerEmbedding(text string, vec []float32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.embedMap[text] = vec
}

func (s *spyEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if vec, ok := s.embedMap[text]; ok {
		return vec, nil
	}
	return embedder.NewFakeEmbedder().Embed(ctx, text)
}

func (s *spyEmbedder) Dim() int {
	return embedder.DefaultDim
}

func (s *spyEmbedder) Close() error {
	return nil
}

type fakeConversationRefresher struct {
	refreshCalls atomic.Int32
}

func (f *fakeConversationRefresher) RefreshConversation(ctx context.Context, guildID, channelID int64) error {
	f.refreshCalls.Add(1)
	return nil
}

func (f *fakeConversationRefresher) Active(ctx context.Context, guildID, channelID int64, since time.Time) (bool, error) {
	return false, nil
}

func (f *fakeConversationRefresher) Clear(ctx context.Context, guildID, channelID int64) error {
	return nil
}

func (f *fakeConversationRefresher) Refresh(ctx context.Context, guildID, channelID int64, until time.Time, ttl time.Duration) error {
	f.refreshCalls.Add(1)
	return nil
}

type simTestContextStore struct {
	messages []*domain.ChannelMessage
}

func (f *simTestContextStore) ListRecent(ctx context.Context, guildID, channelID int64, limit int) ([]*domain.ChannelMessage, error) {
	return f.messages, nil
}

func (f *simTestContextStore) GetByID(ctx context.Context, guildID, messageID int64) (*domain.ChannelMessage, error) {
	for _, msg := range f.messages {
		if msg.ID == messageID {
			return msg, nil
		}
	}
	return nil, nil
}

func (f *simTestContextStore) ListByUserAcrossChannels(ctx context.Context, guildID int64, userID int64, sinceMinutes int, limit int) ([]*domain.ChannelMessage, error) {
	return nil, nil
}

func TestE2ESimilarityInWindowRelevance(t *testing.T) {
	ctx := context.Background()

	spy := newSpyEmbedder()

	similarVec := make([]float32, embedder.DefaultDim)
	for i := range similarVec {
		similarVec[i] = 0.1
	}
	similarVec = embedder.L2Normalize(similarVec)

	dissimilarVec := make([]float32, embedder.DefaultDim)
	for i := range dissimilarVec {
		dissimilarVec[i] = -0.1
	}
	dissimilarVec = embedder.L2Normalize(dissimilarVec)

	spy.registerEmbedding("hello iris", similarVec)
	spy.registerEmbedding("follow up on that", similarVec)
	spy.registerEmbedding("pizza recipe", dissimilarVec)

	llm := testutil.NewFakeLLMClient()
	llm.ChatResponses[""] = "Test response"

	sendRecorder := &fakeSendRecorder{}

	routerStub := &stubRouter{decision: router.Respond(router.ReasonActiveConversation)}

	contextStore := &simTestContextStore{
		messages: []*domain.ChannelMessage{
			{
				ID:                 1,
				GuildID:            1,
				ChannelID:          1,
				UserID:             99,
				Content:            "prior context message",
				ContentEmbedding:   similarVec,
				TriggerSource:      "observe",
				CreatedAt:          time.Now().Add(-5 * time.Minute),
			},
		},
	}

	inWindow := NewSimilarityInWindowRelevance(SimilarityInWindowConfig{
		Embedder:   spy,
		Threshold:  0.55,
		MinContext: 1,
	})

	orch := New(Config{
		Router:            routerStub,
		LLM:               llm,
		Discord:           sendRecorder,
		ContextStore:      contextStore,
		InWindowRelevance: inWindow,
		QueueSize:         10,
		WorkerCount:       1,
		EnqueueLimit:      50 * time.Millisecond,
		DedupeTTL:         30 * time.Second,
		TypingAfter:       100 * time.Millisecond,
		TypingRepeat:      500 * time.Millisecond,
		JobTimeout:        10 * time.Second,
		SystemPrompt:      "You are a helpful assistant.",
	})

	orch.Start()
	defer orch.Stop()

	event1 := &domain.DiscordEvent{
		Type:      "message_mention",
		GuildID:   1,
		ChannelID: 1,
		UserID:    42,
		Message: &domain.DiscordMessage{
			ID:      hashID("test-msg-1"),
			GuildID: 1,
			Content: "hello iris",
		},
		CreatedAt: time.Now(),
	}

	err := orch.Enqueue(ctx, event1)
	if err != nil {
		t.Fatalf("failed to enqueue event1: %v", err)
	}

	waitUntil(t, 2*time.Second, func() bool {
		return sendRecorder.ChunkCount() > 0
	})

	if sendRecorder.ChunkCount() == 0 {
		t.Error("event1: expected LLM call and reply for mention")
	}

	initialChunks := sendRecorder.ChunkCount()

	event2 := &domain.DiscordEvent{
		Type:      "message_create",
		GuildID:   1,
		ChannelID: 1,
		UserID:    42,
		Message: &domain.DiscordMessage{
			ID:      hashID("test-msg-2"),
			GuildID: 1,
			Content: "follow up on that",
		},
		CreatedAt: time.Now(),
	}

	err = orch.Enqueue(ctx, event2)
	if err != nil {
		t.Fatalf("failed to enqueue event2: %v", err)
	}

	waitUntil(t, 2*time.Second, func() bool {
		return sendRecorder.ChunkCount() > initialChunks
	})

	if sendRecorder.ChunkCount() <= initialChunks {
		t.Error("event2: expected LLM call for similar message (high similarity)")
	}

	event3 := &domain.DiscordEvent{
		Type:      "message_create",
		GuildID:   1,
		ChannelID: 1,
		UserID:    42,
		Message: &domain.DiscordMessage{
			ID:      hashID("test-msg-3"),
			GuildID: 1,
			Content: "pizza recipe",
		},
		CreatedAt: time.Now(),
	}

	err = orch.Enqueue(ctx, event3)
	if err != nil {
		t.Fatalf("failed to enqueue event3: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	finalChunks := sendRecorder.ChunkCount()

	if finalChunks > initialChunks+1 {
		t.Errorf("event3: expected NO LLM call for dissimilar message, but got %d chunks (was %d)", finalChunks, initialChunks+1)
	}
}
