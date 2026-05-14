package memory

import (
	"context"
	"errors"
	"testing"

	"github.com/eko/iris-bot/internal/domain"
	"github.com/eko/iris-bot/internal/embedder"
	"github.com/eko/iris-bot/internal/repository"
)

type fakeRecallStore struct {
	results []*repository.RecallResult
	err     error
	calls   int
	lastGID int64
	lastThr float64
	lastK   int
}

func (f *fakeRecallStore) RecallByVector(ctx context.Context, guildID int64, query []float32, threshold float64, topK int) ([]*repository.RecallResult, error) {
	f.calls++
	f.lastGID = guildID
	f.lastThr = threshold
	f.lastK = topK
	if f.err != nil {
		return nil, f.err
	}
	return f.results, nil
}

func mkRecall(id int64, guildID int64, content string, sim float64) *repository.RecallResult {
	return &repository.RecallResult{
		Message: &domain.ChannelMessage{
			ID:        id,
			GuildID:   guildID,
			MessageID: id,
			Content:   content,
		},
		Similarity: sim,
	}
}

func TestGuildRecallService_ReturnsNilWhenDisabled(t *testing.T) {
	store := &fakeRecallStore{}
	svc, err := NewGuildRecallService(embedder.NewFakeEmbedder(), store, GuildRecallConfig{Enabled: false, Threshold: 0.5, TopK: 5})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	got, err := svc.Recall(context.Background(), 123, "hello world")
	if err != nil {
		t.Fatalf("recall err: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil when disabled, got %v", got)
	}
	if store.calls != 0 {
		t.Fatalf("store should not be called when disabled")
	}
}

func TestGuildRecallService_RejectsMissingGuild(t *testing.T) {
	store := &fakeRecallStore{}
	svc, err := NewGuildRecallService(embedder.NewFakeEmbedder(), store, GuildRecallConfig{Enabled: true, Threshold: 0.5, TopK: 5})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	_, err = svc.Recall(context.Background(), 0, "q")
	if !errors.Is(err, ErrMissingGuildID) {
		t.Fatalf("want ErrMissingGuildID, got %v", err)
	}
	if store.calls != 0 {
		t.Fatalf("store should not be called on missing guild")
	}
}

func TestGuildRecallService_PassesThresholdAndTopK(t *testing.T) {
	store := &fakeRecallStore{
		results: []*repository.RecallResult{
			mkRecall(1, 42, "relevant", 0.9),
		},
	}
	svc, err := NewGuildRecallService(embedder.NewFakeEmbedder(), store, GuildRecallConfig{Enabled: true, Threshold: 0.75, TopK: 3})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	got, err := svc.Recall(context.Background(), 42, "hello lore")
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 result, got %d", len(got))
	}
	if store.lastGID != 42 {
		t.Fatalf("guild passthrough failed: %v", store.lastGID)
	}
	if store.lastThr != 0.75 {
		t.Fatalf("threshold passthrough failed: %v", store.lastThr)
	}
	if store.lastK != 3 {
		t.Fatalf("topK passthrough failed: %v", store.lastK)
	}
}

func TestGuildRecallService_EmbedderFailureDegradesGracefully(t *testing.T) {
	store := &fakeRecallStore{}
	svc, err := NewGuildRecallService(&errorEmbedder{}, store, GuildRecallConfig{Enabled: true, Threshold: 0.5, TopK: 3})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	got, err := svc.Recall(context.Background(), 1, "q")
	if err != nil {
		t.Fatalf("recall should not error on embed failure: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil recall on embed failure, got %v", got)
	}
	if store.calls != 0 {
		t.Fatalf("store should not be called after embed failure")
	}
}

func TestGuildRecallService_EmptyQueryReturnsNil(t *testing.T) {
	store := &fakeRecallStore{}
	svc, err := NewGuildRecallService(embedder.NewFakeEmbedder(), store, GuildRecallConfig{Enabled: true, Threshold: 0.5, TopK: 3})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	got, err := svc.Recall(context.Background(), 1, "   ")
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	if got != nil {
		t.Fatalf("empty query should yield nil")
	}
	if store.calls != 0 {
		t.Fatalf("store should not be called for empty query")
	}
}

type errorEmbedder struct{}

func (errorEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	return nil, errors.New("embedder offline")
}
func (errorEmbedder) Dim() int     { return repository.ExpectedEmbeddingDim }
func (errorEmbedder) Close() error { return nil }
