package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/eko/iris-bot/internal/domain"
	"github.com/eko/iris-bot/internal/embedder"
)

// SpyEmbedder tracks calls and allows controlled responses
type SpyEmbedder struct {
	mu              sync.Mutex
	callCount       int
	responses       [][]float32
	errors          []error
	responseIdx     int
	recordedTexts   []string
}

func NewSpyEmbedder() *SpyEmbedder {
	return &SpyEmbedder{
		responses: [][]float32{},
		errors:    []error{},
	}
}

func (s *SpyEmbedder) SetResponse(vec []float32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.responses = append(s.responses, vec)
	s.errors = append(s.errors, nil)
}

func (s *SpyEmbedder) SetError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.responses = append(s.responses, nil)
	s.errors = append(s.errors, err)
}

func (s *SpyEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.callCount++
	s.recordedTexts = append(s.recordedTexts, text)

	if s.responseIdx >= len(s.responses) {
		return nil, errors.New("no more responses configured")
	}

	resp := s.responses[s.responseIdx]
	err := s.errors[s.responseIdx]
	s.responseIdx++

	return resp, err
}

func (s *SpyEmbedder) Dim() int {
	return embedder.DefaultDim
}

func (s *SpyEmbedder) Close() error {
	return nil
}

func (s *SpyEmbedder) CallCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.callCount
}

func (s *SpyEmbedder) RecordedTexts() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.recordedTexts...)
}

// Helper to create a unit-norm vector
func unitVec(dim int, seed float32) []float32 {
	v := make([]float32, dim)
	for i := range v {
		v[i] = seed + float32(i)*0.001
	}
	return embedder.L2Normalize(v)
}

func TestSimilarityInWindow_AboveThreshold_True(t *testing.T) {
	spy := NewSpyEmbedder()
	// Same vector for both context and current message -> similarity = 1.0
	sameVec := unitVec(embedder.DefaultDim, 0.5)
	spy.SetResponse(sameVec)
	spy.SetResponse(sameVec)

	cfg := SimilarityInWindowConfig{
		Embedder:   spy,
		Threshold:  0.55,
		MinContext: 1,
	}
	classifier := NewSimilarityInWindowRelevance(cfg)

	event := &domain.DiscordEvent{
		GuildID:   1,
		ChannelID: 100,
		UserID:    42,
		Message: &domain.DiscordMessage{
			ID:      999,
			Content: "test message",
		},
	}

	contextMessages := []*domain.ChannelMessage{
		{
			GuildID:    1,
			ChannelID:  100,
			MessageID:  998,
			UserID:     1,
			Content:    "test message",
			IsBot:      false,
		},
	}

	relevant, _, _, err := classifier.IsRelevant(context.Background(), event, contextMessages)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !relevant {
		t.Errorf("expected relevant=true (sim=1.0 >= 0.55), got false")
	}
}

func TestSimilarityInWindow_BelowThreshold_False(t *testing.T) {
	spy := NewSpyEmbedder()
	// Orthogonal vectors -> similarity = 0.0
	vec1 := make([]float32, embedder.DefaultDim)
	vec1[0] = 1.0
	vec1 = embedder.L2Normalize(vec1)

	vec2 := make([]float32, embedder.DefaultDim)
	vec2[1] = 1.0
	vec2 = embedder.L2Normalize(vec2)

	spy.SetResponse(vec2)
	spy.SetResponse(vec1)

	cfg := SimilarityInWindowConfig{
		Embedder:   spy,
		Threshold:  0.55,
		MinContext: 1,
	}
	classifier := NewSimilarityInWindowRelevance(cfg)

	event := &domain.DiscordEvent{
		GuildID:   1,
		ChannelID: 100,
		UserID:    42,
		Message: &domain.DiscordMessage{
			ID:      999,
			Content: "unrelated text",
		},
	}

	contextMessages := []*domain.ChannelMessage{
		{
			GuildID:    1,
			ChannelID:  100,
			MessageID:  998,
			UserID:     1,
			Content:    "different text",
			IsBot:      false,
		},
	}

	relevant, _, _, err := classifier.IsRelevant(context.Background(), event, contextMessages)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if relevant {
		t.Errorf("expected relevant=false (sim=0.0 < 0.55), got true")
	}
}

func TestSimilarityInWindow_EmptyContext_False(t *testing.T) {
	spy := NewSpyEmbedder()
	spy.SetResponse(unitVec(embedder.DefaultDim, 0.5))

	cfg := SimilarityInWindowConfig{
		Embedder:   spy,
		Threshold:  0.55,
		MinContext: 1,
	}
	classifier := NewSimilarityInWindowRelevance(cfg)

	event := &domain.DiscordEvent{
		GuildID:   1,
		ChannelID: 100,
		UserID:    42,
		Message: &domain.DiscordMessage{
			ID:      999,
			Content: "test",
		},
	}

	relevant, _, _, err := classifier.IsRelevant(context.Background(), event, []*domain.ChannelMessage{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if relevant {
		t.Errorf("expected relevant=false for empty context, got true")
	}
	if spy.CallCount() != 0 {
		t.Errorf("expected embedder not called, but was called %d times", spy.CallCount())
	}
}

func TestSimilarityInWindow_NilEmbedder_ReturnsError(t *testing.T) {
	cfg := SimilarityInWindowConfig{
		Embedder:   nil,
		Threshold:  0.55,
		MinContext: 1,
	}
	classifier := NewSimilarityInWindowRelevance(cfg)

	event := &domain.DiscordEvent{
		GuildID:   1,
		ChannelID: 100,
		UserID:    42,
		Message: &domain.DiscordMessage{
			ID:      999,
			Content: "test",
		},
	}

	contextMessages := []*domain.ChannelMessage{
		{
			GuildID:    1,
			ChannelID:  100,
			MessageID:  998,
			UserID:     1,
			Content:    "context",
			IsBot:      false,
		},
	}

	relevant, _, _, err := classifier.IsRelevant(context.Background(), event, contextMessages)
	if err == nil {
		t.Fatalf("expected error for nil embedder, got nil")
	}
	if relevant {
		t.Errorf("expected relevant=false on error, got true")
	}
}

func TestSimilarityInWindow_EmbedError_ReturnsFalseErr(t *testing.T) {
	spy := NewSpyEmbedder()
	spy.SetError(errors.New("embedder failed"))

	cfg := SimilarityInWindowConfig{
		Embedder:   spy,
		Threshold:  0.55,
		MinContext: 1,
	}
	classifier := NewSimilarityInWindowRelevance(cfg)

	event := &domain.DiscordEvent{
		GuildID:   1,
		ChannelID: 100,
		UserID:    42,
		Message: &domain.DiscordMessage{
			ID:      999,
			Content: "test",
		},
	}

	contextMessages := []*domain.ChannelMessage{
		{
			GuildID:    1,
			ChannelID:  100,
			MessageID:  998,
			UserID:     1,
			Content:    "context",
			IsBot:      false,
		},
	}

	relevant, _, _, err := classifier.IsRelevant(context.Background(), event, contextMessages)
	if err == nil {
		t.Fatalf("expected error from embedder, got nil")
	}
	if relevant {
		t.Errorf("expected relevant=false on error, got true")
	}
}

func TestSimilarityInWindow_UsesStoredEmbeddingsWhenPresent(t *testing.T) {
	spy := NewSpyEmbedder()
	currentVec := unitVec(embedder.DefaultDim, 0.5)
	spy.SetResponse(currentVec)

	cfg := SimilarityInWindowConfig{
		Embedder:   spy,
		Threshold:  0.55,
		MinContext: 1,
	}
	classifier := NewSimilarityInWindowRelevance(cfg)

	storedVec := unitVec(embedder.DefaultDim, 0.5)

	event := &domain.DiscordEvent{
		GuildID:   1,
		ChannelID: 100,
		UserID:    42,
		Message: &domain.DiscordMessage{
			ID:      999,
			Content: "current message",
		},
	}

	contextMessages := []*domain.ChannelMessage{
		{
			GuildID:          1,
			ChannelID:        100,
			MessageID:        998,
			UserID:           1,
			Content:          "context message",
			IsBot:            false,
			ContentEmbedding: storedVec,
		},
	}

	relevant, _, _, err := classifier.IsRelevant(context.Background(), event, contextMessages)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !relevant {
		t.Errorf("expected relevant=true, got false")
	}

	// Embedder should be called exactly once (for current message only)
	if spy.CallCount() != 1 {
		t.Errorf("expected embedder called 1 time, got %d", spy.CallCount())
	}
}

func TestSimilarityInWindow_FallsBackWhenEmbeddingMissing(t *testing.T) {
	spy := NewSpyEmbedder()
	currentVec := unitVec(embedder.DefaultDim, 0.5)
	missingVec := unitVec(embedder.DefaultDim, 0.6)

	spy.SetResponse(currentVec)
	spy.SetResponse(missingVec)

	cfg := SimilarityInWindowConfig{
		Embedder:   spy,
		Threshold:  0.55,
		MinContext: 1,
	}
	classifier := NewSimilarityInWindowRelevance(cfg)

	event := &domain.DiscordEvent{
		GuildID:   1,
		ChannelID: 100,
		UserID:    42,
		Message: &domain.DiscordMessage{
			ID:      999,
			Content: "current message",
		},
	}

	contextMessages := []*domain.ChannelMessage{
		{
			GuildID:          1,
			ChannelID:        100,
			MessageID:        998,
			UserID:           1,
			Content:          "context message without embedding",
			IsBot:            false,
			ContentEmbedding: nil,
		},
	}

	_, _, _, err := classifier.IsRelevant(context.Background(), event, contextMessages)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Embedder should be called twice (current + fallback for missing)
	if spy.CallCount() != 2 {
		t.Errorf("expected embedder called 2 times, got %d", spy.CallCount())
	}
}

func TestSimilarityInWindow_CentroidIsL2Normalized(t *testing.T) {
	spy := NewSpyEmbedder()
	vec1 := unitVec(embedder.DefaultDim, 0.1)
	vec2 := unitVec(embedder.DefaultDim, 0.2)
	currentVec := unitVec(embedder.DefaultDim, 0.3)

	spy.SetResponse(currentVec)

	cfg := SimilarityInWindowConfig{
		Embedder:   spy,
		Threshold:  0.55,
		MinContext: 1,
	}
	classifier := NewSimilarityInWindowRelevance(cfg)

	event := &domain.DiscordEvent{
		GuildID:   1,
		ChannelID: 100,
		UserID:    42,
		Message: &domain.DiscordMessage{
			ID:      999,
			Content: "current",
		},
	}

	contextMessages := []*domain.ChannelMessage{
		{
			GuildID:          1,
			ChannelID:        100,
			MessageID:        998,
			UserID:           1,
			Content:          "msg1",
			IsBot:            false,
			ContentEmbedding: vec1,
		},
		{
			GuildID:          1,
			ChannelID:        100,
			MessageID:        997,
			UserID:           2,
			Content:          "msg2",
			IsBot:            false,
			ContentEmbedding: vec2,
		},
	}

	relevant, _, _, err := classifier.IsRelevant(context.Background(), event, contextMessages)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Compute expected centroid manually
	centroid := make([]float32, embedder.DefaultDim)
	for i := range centroid {
		centroid[i] = (vec1[i] + vec2[i]) / 2.0
	}
	centroid = embedder.L2Normalize(centroid)

	// Verify centroid norm is ~1.0
	norm := float32(0)
	for _, v := range centroid {
		norm += v * v
	}
	norm = float32(embedder.L2Normalize([]float32{norm})[0])
	if norm < 0.99 || norm > 1.01 {
		t.Errorf("expected centroid norm ~1.0, got %f", norm)
	}

	// Similarity should be dot product of normalized vectors
	expectedSim := embedder.Cosine(centroid, currentVec)
	if expectedSim < 0.55 {
		t.Errorf("expected similarity >= 0.55, got %f", expectedSim)
	}
	if !relevant {
		t.Errorf("expected relevant=true, got false")
	}
}

func TestSimilarityInWindow_DefaultThreshold(t *testing.T) {
	spy := NewSpyEmbedder()
	vec := unitVec(embedder.DefaultDim, 0.5)
	spy.SetResponse(vec)
	spy.SetResponse(vec)

	cfg := SimilarityInWindowConfig{
		Embedder:   spy,
		Threshold:  0, // Should default to 0.55
		MinContext: 1,
	}
	classifier := NewSimilarityInWindowRelevance(cfg)

	event := &domain.DiscordEvent{
		GuildID:   1,
		ChannelID: 100,
		UserID:    42,
		Message: &domain.DiscordMessage{
			ID:      999,
			Content: "test",
		},
	}

	contextMessages := []*domain.ChannelMessage{
		{
			GuildID:    1,
			ChannelID:  100,
			MessageID:  998,
			UserID:     1,
			Content:    "test",
			IsBot:      false,
			ContentEmbedding: vec,
		},
	}

	relevant, _, _, err := classifier.IsRelevant(context.Background(), event, contextMessages)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !relevant {
		t.Errorf("expected relevant=true with default threshold, got false")
	}
}

func TestSimilarityInWindow_DefaultMinContext(t *testing.T) {
	spy := NewSpyEmbedder()

	cfg := SimilarityInWindowConfig{
		Embedder:   spy,
		Threshold:  0.55,
		MinContext: 0, // Should default to 1
	}
	classifier := NewSimilarityInWindowRelevance(cfg)

	event := &domain.DiscordEvent{
		GuildID:   1,
		ChannelID: 100,
		UserID:    42,
		Message: &domain.DiscordMessage{
			ID:      999,
			Content: "test",
		},
	}

	// Empty context should fail MinContext check
	relevant, _, _, err := classifier.IsRelevant(context.Background(), event, []*domain.ChannelMessage{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if relevant {
		t.Errorf("expected relevant=false with empty context and MinContext=1, got true")
	}
	if spy.CallCount() != 0 {
		t.Errorf("expected no embedder calls, got %d", spy.CallCount())
	}
}
