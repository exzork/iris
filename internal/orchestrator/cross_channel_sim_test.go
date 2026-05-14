package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/eko/iris-bot/internal/domain"
	"github.com/eko/iris-bot/internal/embedder"
)

type fakeEmbedderForSim struct {
	embeddings map[string][]float32
	err        error
	mu         sync.Mutex
	callCount  int
}

func (f *fakeEmbedderForSim) Embed(ctx context.Context, text string) ([]float32, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.callCount++
	if f.err != nil {
		return nil, f.err
	}
	if v, ok := f.embeddings[text]; ok {
		return v, nil
	}
	// Return a deterministic embedding based on text length
	v := make([]float32, embedder.DefaultDim)
	for i := range v {
		v[i] = float32(len(text)) / float32(embedder.DefaultDim)
	}
	return embedder.L2Normalize(v), nil
}

func (f *fakeEmbedderForSim) Dim() int {
	return embedder.DefaultDim
}

func (f *fakeEmbedderForSim) Close() error {
	return nil
}

func makeCandidateWithEmbedding(guildID, channelID, messageID, userID int64, isBot bool, createdAt time.Time, embedding []float32) *domain.ChannelMessage {
	author := "user"
	return &domain.ChannelMessage{
		GuildID:          guildID,
		ChannelID:        channelID,
		MessageID:        messageID,
		UserID:           userID,
		AuthorName:       &author,
		Content:          "candidate",
		IsBot:            isBot,
		CreatedAt:        createdAt,
		ContentEmbedding: embedding,
	}
}

func TestSimilarityCrossChannel_EmptyCandidates_ReturnsNil(t *testing.T) {
	store := &fakeCandidateStore{messages: []*domain.ChannelMessage{}}
	embedder := embedder.NewFakeEmbedder()

	classifier := NewSimilarityCrossChannelClassifier(SimilarityCrossChannelConfig{
		Store:         store,
		Embedder:      embedder,
		Threshold:     0.55,
		MaxCandidates: 10,
		WindowMinutes: 30,
	})

	event := &domain.DiscordEvent{
		GuildID:   1,
		ChannelID: 100,
		UserID:    42,
		Message:   &domain.DiscordMessage{ID: 777, Content: "test message"},
	}

	selected, err := classifier.Classify(context.Background(), event)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if selected != nil {
		t.Fatalf("expected nil, got %v", selected)
	}
}

func TestSimilarityCrossChannel_FiltersCurrentChannel(t *testing.T) {
	now := time.Now()
	emb := []float32{1.0, 0.0, 0.0, 0.0}
	emb = embedder.L2Normalize(emb)
	messages := []*domain.ChannelMessage{
		makeCandidateWithEmbedding(1, 100, 1, 42, false, now.Add(-5*time.Minute), emb),
		makeCandidateWithEmbedding(1, 200, 2, 42, false, now.Add(-10*time.Minute), emb),
	}

	store := &fakeCandidateStore{messages: messages}
	fakeEmb := &fakeEmbedderForSim{
		embeddings: map[string][]float32{
			"test": emb,
		},
	}

	classifier := NewSimilarityCrossChannelClassifier(SimilarityCrossChannelConfig{
		Store:         store,
		Embedder:      fakeEmb,
		Threshold:     0.0,
		MaxCandidates: 10,
		WindowMinutes: 30,
	})

	event := &domain.DiscordEvent{
		GuildID:   1,
		ChannelID: 100,
		UserID:    42,
		Message:   &domain.DiscordMessage{ID: 777, Content: "test"},
	}

	selected, err := classifier.Classify(context.Background(), event)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(selected) != 1 {
		t.Fatalf("expected 1 candidate (current channel filtered), got %d", len(selected))
	}
	if selected[0].ChannelID != 200 {
		t.Fatalf("expected channel 200, got %d", selected[0].ChannelID)
	}
}

func TestSimilarityCrossChannel_FiltersBotMessages(t *testing.T) {
	now := time.Now()
	emb := []float32{1.0, 0.0, 0.0, 0.0}
	emb = embedder.L2Normalize(emb)
	messages := []*domain.ChannelMessage{
		makeCandidateWithEmbedding(1, 200, 1, 42, true, now.Add(-5*time.Minute), emb),
		makeCandidateWithEmbedding(1, 200, 2, 42, false, now.Add(-10*time.Minute), emb),
	}

	store := &fakeCandidateStore{messages: messages}
	fakeEmb := &fakeEmbedderForSim{
		embeddings: map[string][]float32{
			"test": emb,
		},
	}

	classifier := NewSimilarityCrossChannelClassifier(SimilarityCrossChannelConfig{
		Store:         store,
		Embedder:      fakeEmb,
		Threshold:     0.0,
		MaxCandidates: 10,
		WindowMinutes: 30,
	})

	event := &domain.DiscordEvent{
		GuildID:   1,
		ChannelID: 100,
		UserID:    42,
		Message:   &domain.DiscordMessage{ID: 777, Content: "test"},
	}

	selected, err := classifier.Classify(context.Background(), event)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(selected) != 1 {
		t.Fatalf("expected 1 candidate (bot filtered), got %d", len(selected))
	}
	if selected[0].IsBot {
		t.Fatalf("expected non-bot message")
	}
}

func TestSimilarityCrossChannel_FiltersUnallowedChannelsInIncludeListMode(t *testing.T) {
	now := time.Now()
	emb := []float32{1.0, 0.0, 0.0, 0.0}
	emb = embedder.L2Normalize(emb)
	messages := []*domain.ChannelMessage{
		makeCandidateWithEmbedding(1, 200, 1, 42, false, now.Add(-5*time.Minute), emb),
		makeCandidateWithEmbedding(1, 300, 2, 42, false, now.Add(-10*time.Minute), emb),
	}

	store := &fakeCandidateStore{messages: messages}
	allowed := &fakeAllowQuerier{
		hasAny: true,
		allowed: map[int64]bool{
			200: true,
			300: false,
		},
	}
	fakeEmb := &fakeEmbedderForSim{
		embeddings: map[string][]float32{
			"test": emb,
		},
	}

	classifier := NewSimilarityCrossChannelClassifier(SimilarityCrossChannelConfig{
		Store:         store,
		Allowed:       allowed,
		Embedder:      fakeEmb,
		Threshold:     0.0,
		MaxCandidates: 10,
		WindowMinutes: 30,
	})

	event := &domain.DiscordEvent{
		GuildID:   1,
		ChannelID: 100,
		UserID:    42,
		Message:   &domain.DiscordMessage{ID: 777, Content: "test"},
	}

	selected, err := classifier.Classify(context.Background(), event)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(selected) != 1 {
		t.Fatalf("expected 1 candidate (unallowed filtered), got %d", len(selected))
	}
	if selected[0].ChannelID != 200 {
		t.Fatalf("expected channel 200, got %d", selected[0].ChannelID)
	}
}

func TestSimilarityCrossChannel_OrdersByScoreDescending(t *testing.T) {
	now := time.Now()

	// Create embeddings with known similarities
	// All normalized to unit length
	emb1 := []float32{1.0, 0.0, 0.0, 0.0}
	emb1 = embedder.L2Normalize(emb1)
	emb2 := []float32{0.9, 0.1, 0.0, 0.0}
	emb2 = embedder.L2Normalize(emb2)
	emb3 := []float32{0.5, 0.5, 0.0, 0.0}
	emb3 = embedder.L2Normalize(emb3)

	messages := []*domain.ChannelMessage{
		makeCandidateWithEmbedding(1, 200, 1, 42, false, now.Add(-5*time.Minute), emb1),
		makeCandidateWithEmbedding(1, 300, 2, 42, false, now.Add(-10*time.Minute), emb3),
		makeCandidateWithEmbedding(1, 400, 3, 42, false, now.Add(-15*time.Minute), emb2),
	}

	store := &fakeCandidateStore{messages: messages}
	fakeEmb := &fakeEmbedderForSim{
		embeddings: map[string][]float32{
			"current": emb1,
		},
	}

	classifier := NewSimilarityCrossChannelClassifier(SimilarityCrossChannelConfig{
		Store:         store,
		Embedder:      fakeEmb,
		Threshold:     0.5,
		MaxCandidates: 10,
		WindowMinutes: 30,
	})

	event := &domain.DiscordEvent{
		GuildID:   1,
		ChannelID: 100,
		UserID:    42,
		Message:   &domain.DiscordMessage{ID: 777, Content: "current"},
	}

	selected, err := classifier.Classify(context.Background(), event)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(selected) != 3 {
		t.Fatalf("expected 3 candidates, got %d", len(selected))
	}

	// Verify ordering: highest similarity first
	sim1 := embedder.Cosine(emb1, emb1)
	sim2 := embedder.Cosine(emb1, emb2)
	sim3 := embedder.Cosine(emb1, emb3)

	if sim1 < sim2 || sim2 < sim3 {
		t.Fatalf("test setup error: similarities not in expected order")
	}

	if selected[0].MessageID != 1 {
		t.Fatalf("expected first result messageID 1 (highest sim), got %d", selected[0].MessageID)
	}
	if selected[1].MessageID != 3 {
		t.Fatalf("expected second result messageID 3, got %d", selected[1].MessageID)
	}
	if selected[2].MessageID != 2 {
		t.Fatalf("expected third result messageID 2 (lowest sim), got %d", selected[2].MessageID)
	}
}

func TestSimilarityCrossChannel_TruncatesToMaxCandidates(t *testing.T) {
	now := time.Now()
	emb := []float32{1.0, 0.0, 0.0, 0.0}
	emb = embedder.L2Normalize(emb)
	var messages []*domain.ChannelMessage
	for i := 0; i < 7; i++ {
		messages = append(messages, makeCandidateWithEmbedding(1, 200+int64(i), int64(i+1), 42, false, now.Add(-time.Duration(i)*time.Minute), emb))
	}

	store := &fakeCandidateStore{messages: messages}
	fakeEmb := &fakeEmbedderForSim{
		embeddings: map[string][]float32{
			"test": emb,
		},
	}

	classifier := NewSimilarityCrossChannelClassifier(SimilarityCrossChannelConfig{
		Store:         store,
		Embedder:      fakeEmb,
		Threshold:     0.0,
		MaxCandidates: 3,
		WindowMinutes: 30,
	})

	event := &domain.DiscordEvent{
		GuildID:   1,
		ChannelID: 100,
		UserID:    42,
		Message:   &domain.DiscordMessage{ID: 777, Content: "test"},
	}

	selected, err := classifier.Classify(context.Background(), event)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(selected) != 3 {
		t.Fatalf("expected 3 candidates (truncated to max), got %d", len(selected))
	}
}

func TestSimilarityCrossChannel_NilEmbedderOrStore_NoOp(t *testing.T) {
	// Test nil store
	classifier := NewSimilarityCrossChannelClassifier(SimilarityCrossChannelConfig{
		Store:    nil,
		Embedder: embedder.NewFakeEmbedder(),
	})

	event := &domain.DiscordEvent{
		GuildID:   1,
		ChannelID: 100,
		UserID:    42,
		Message:   &domain.DiscordMessage{ID: 777, Content: "test"},
	}

	selected, err := classifier.Classify(context.Background(), event)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if selected != nil {
		t.Fatalf("expected nil, got %v", selected)
	}

	// Test nil embedder
	store := &fakeCandidateStore{messages: []*domain.ChannelMessage{
		makeCandidateWithEmbedding(1, 200, 1, 42, false, time.Now(), nil),
	}}
	classifier = NewSimilarityCrossChannelClassifier(SimilarityCrossChannelConfig{
		Store:    store,
		Embedder: nil,
	})

	selected, err = classifier.Classify(context.Background(), event)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if selected != nil {
		t.Fatalf("expected nil, got %v", selected)
	}
}

func TestSimilarityCrossChannel_EmbedErrorReturnsErr(t *testing.T) {
	now := time.Now()
	messages := []*domain.ChannelMessage{
		makeCandidateWithEmbedding(1, 200, 1, 42, false, now.Add(-5*time.Minute), nil),
	}

	store := &fakeCandidateStore{messages: messages}
	fakeEmb := &fakeEmbedderForSim{
		err: errors.New("embed failed"),
	}

	classifier := NewSimilarityCrossChannelClassifier(SimilarityCrossChannelConfig{
		Store:         store,
		Embedder:      fakeEmb,
		Threshold:     0.55,
		MaxCandidates: 10,
		WindowMinutes: 30,
	})

	event := &domain.DiscordEvent{
		GuildID:   1,
		ChannelID: 100,
		UserID:    42,
		Message:   &domain.DiscordMessage{ID: 777, Content: "test"},
	}

	selected, err := classifier.Classify(context.Background(), event)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if selected != nil {
		t.Fatalf("expected nil selected, got %v", selected)
	}
}

func TestSimilarityCrossChannel_UsesStoredEmbeddings(t *testing.T) {
	now := time.Now()

	emb1 := []float32{1.0, 0.0, 0.0, 0.0}
	emb1 = embedder.L2Normalize(emb1)
	emb2 := []float32{0.9, 0.1, 0.0, 0.0}
	emb2 = embedder.L2Normalize(emb2)

	messages := []*domain.ChannelMessage{
		makeCandidateWithEmbedding(1, 200, 1, 42, false, now.Add(-5*time.Minute), emb2),
	}

	store := &fakeCandidateStore{messages: messages}
	fakeEmb := &fakeEmbedderForSim{
		embeddings: map[string][]float32{
			"current": emb1,
		},
	}

	classifier := NewSimilarityCrossChannelClassifier(SimilarityCrossChannelConfig{
		Store:         store,
		Embedder:      fakeEmb,
		Threshold:     0.5,
		MaxCandidates: 10,
		WindowMinutes: 30,
	})

	event := &domain.DiscordEvent{
		GuildID:   1,
		ChannelID: 100,
		UserID:    42,
		Message:   &domain.DiscordMessage{ID: 777, Content: "current"},
	}

	selected, err := classifier.Classify(context.Background(), event)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(selected) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(selected))
	}

	// Verify embedder was called exactly once (for current message only)
	fakeEmb.mu.Lock()
	callCount := fakeEmb.callCount
	fakeEmb.mu.Unlock()

	if callCount != 1 {
		t.Fatalf("expected embedder called once, got %d times", callCount)
	}
}
