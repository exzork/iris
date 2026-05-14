package canoncheck

import (
	"context"
	"testing"

	ragpkg "github.com/eko/iris-bot/internal/lore/rag"
)

type fakeEmbeddingProvider struct {
	embedding []float32
}

func (f *fakeEmbeddingProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	return f.embedding, nil
}

func TestCheckSupportedClaim(t *testing.T) {
	store := &ragpkg.InMemoryChunkStore{}
	store.Chunks = append(store.Chunks, ragpkg.Chunk{
		ID:        1,
		Title:     "Quest Y Guide",
		URL:       "https://wiki.example.com/quest-y",
		Content:   "Rover appears in Quest Y as a main character",
		Embedding: []float32{1, 0, 0, 0},
	})
	store.Chunks = append(store.Chunks, ragpkg.Chunk{
		ID:        2,
		Title:     "Character Appearances",
		URL:       "https://wiki.example.com/characters",
		Content:   "Quest Y features Rover prominently in the storyline",
		Embedding: []float32{1, 0, 0, 0},
	})

	embedding := []float32{1, 0, 0, 0}
	provider := &fakeEmbeddingProvider{embedding: embedding}

	retriever := &ragpkg.Retriever{Embed: provider, Store: store, MinScore: 0.0}
	verifier := NewVerifier(retriever)

	claim := Claim{Text: "Rover appears in Quest Y"}
	verdict, err := verifier.Check(context.Background(), claim)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if verdict.Status != StatusSupported {
		t.Errorf("expected StatusSupported, got %s", verdict.Status)
	}

	if verdict.Confidence < 0.7 {
		t.Errorf("expected confidence >= 0.7, got %f", verdict.Confidence)
	}

	if len(verdict.Citations) == 0 {
		t.Errorf("expected citations, got none")
	}

	if len(verdict.Snippets) == 0 {
		t.Errorf("expected snippets, got none")
	}

	if verdict.Reason != ReasonSupported {
		t.Errorf("expected reason %q, got %q", ReasonSupported, verdict.Reason)
	}

	t.Logf("Supported Verdict: Status=%s, Confidence=%.2f, Citations=%d, Reason=%s",
		verdict.Status, verdict.Confidence, len(verdict.Citations), verdict.Reason)
}

func TestCheckUnsupportedClaim(t *testing.T) {
	store := &ragpkg.InMemoryChunkStore{}

	embedding := []float32{1, 0, 0, 0}
	provider := &fakeEmbeddingProvider{embedding: embedding}

	retriever := &ragpkg.Retriever{Embed: provider, Store: store, MinScore: 0.0}
	verifier := NewVerifier(retriever)

	claim := Claim{Text: "Nonexistent character appears in nonexistent quest"}
	verdict, err := verifier.Check(context.Background(), claim)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if verdict.Status != StatusUnsupported {
		t.Errorf("expected StatusUnsupported, got %s", verdict.Status)
	}

	if verdict.Confidence != 0.0 {
		t.Errorf("expected confidence 0.0, got %f", verdict.Confidence)
	}

	if len(verdict.Citations) != 0 {
		t.Errorf("expected no citations, got %d", len(verdict.Citations))
	}

	if verdict.Reason != ReasonUnsupported {
		t.Errorf("expected reason %q, got %q", ReasonUnsupported, verdict.Reason)
	}

	t.Logf("Unsupported Verdict: Status=%s, Confidence=%.2f, Reason=%s",
		verdict.Status, verdict.Confidence, verdict.Reason)
}

func TestCheckContradictedClaim(t *testing.T) {
	store := &ragpkg.InMemoryChunkStore{}
	store.Chunks = append(store.Chunks, ragpkg.Chunk{
		ID:        1,
		Title:     "Quest Y Guide",
		URL:       "https://wiki.example.com/quest-y",
		Content:   "Rover tidak muncul di Quest Y",
		Embedding: []float32{1, 0, 0, 0},
	})

	embedding := []float32{1, 0, 0, 0}
	provider := &fakeEmbeddingProvider{embedding: embedding}

	retriever := &ragpkg.Retriever{Embed: provider, Store: store, MinScore: 0.0}
	verifier := NewVerifier(retriever)

	claim := Claim{Text: "Rover appears in Quest Y"}
	verdict, err := verifier.Check(context.Background(), claim)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if verdict.Status != StatusContradicted {
		t.Errorf("expected StatusContradicted, got %s", verdict.Status)
	}

	if verdict.Reason != ReasonContradicted {
		t.Errorf("expected reason %q, got %q", ReasonContradicted, verdict.Reason)
	}

	t.Logf("Contradicted Verdict: Status=%s, Confidence=%.2f, Reason=%s",
		verdict.Status, verdict.Confidence, verdict.Reason)
}

func TestCheckNeedsMoreSources(t *testing.T) {
	store := &ragpkg.InMemoryChunkStore{}
	store.Chunks = append(store.Chunks, ragpkg.Chunk{
		ID:        1,
		Title:     "Characters",
		URL:       "https://wiki.example.com/characters",
		Content:   "Rover is a character",
		Embedding: []float32{1, 0, 0, 0},
	})

	embedding := []float32{1, 0, 0, 0}
	provider := &fakeEmbeddingProvider{embedding: embedding}

	retriever := &ragpkg.Retriever{Embed: provider, Store: store, MinScore: 0.0}
	verifier := NewVerifier(retriever)
	verifier.MinChunks = 2

	claim := Claim{Text: "Rover appears in Quest Y"}
	verdict, err := verifier.Check(context.Background(), claim)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if verdict.Status != StatusNeedsMoreSources {
		t.Errorf("expected StatusNeedsMoreSources, got %s", verdict.Status)
	}

	if verdict.Reason != ReasonNeedsMoreSources {
		t.Errorf("expected reason %q, got %q", ReasonNeedsMoreSources, verdict.Reason)
	}

	t.Logf("NeedsMoreSources Verdict: Status=%s, Confidence=%.2f, Reason=%s",
		verdict.Status, verdict.Confidence, verdict.Reason)
}

func TestCheckEmptyClaim(t *testing.T) {
	store := &ragpkg.InMemoryChunkStore{}

	embedding := []float32{1, 0, 0, 0}
	provider := &fakeEmbeddingProvider{embedding: embedding}

	retriever := &ragpkg.Retriever{Embed: provider, Store: store, MinScore: 0.0}
	verifier := NewVerifier(retriever)

	claim := Claim{Text: ""}
	verdict, err := verifier.Check(context.Background(), claim)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if verdict.Status != StatusUnsupported {
		t.Errorf("expected StatusUnsupported, got %s", verdict.Status)
	}

	if verdict.Reason != "klaim kosong" {
		t.Errorf("expected reason 'klaim kosong', got %q", verdict.Reason)
	}

	t.Logf("Empty Claim Verdict: Status=%s, Reason=%s", verdict.Status, verdict.Reason)
}
