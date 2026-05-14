package rag

import (
	"context"
	"testing"
)

type FakeEmbedder struct {
	embedding []float32
	err       error
}

func (f *FakeEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	return f.embedding, f.err
}

func TestRetrieveEmbedsAndSorts(t *testing.T) {
	store := &InMemoryChunkStore{
		Chunks: []Chunk{
			{ID: 1, Title: "Chunk1", URL: "http://a", Content: "content1", Embedding: []float32{0, 1, 0, 0}},
			{ID: 2, Title: "Chunk2", URL: "http://b", Content: "content2", Embedding: []float32{1, 0, 0, 0}},
			{ID: 3, Title: "Chunk3", URL: "http://c", Content: "content3", Embedding: []float32{0, 0, 1, 0}},
		},
	}

	retriever := &Retriever{
		Embed:    &FakeEmbedder{embedding: []float32{1, 0, 0, 0}},
		Store:    store,
		MinScore: 0.0,
	}

	result, err := retriever.Retrieve(context.Background(), "test query", 5)
	if err != nil {
		t.Fatalf("Retrieve failed: %v", err)
	}

	if len(result) != 3 {
		t.Fatalf("expected 3 results, got %d", len(result))
	}

	if result[0].ID != 2 {
		t.Fatalf("expected first result to be Chunk2 (ID=2), got ID=%d", result[0].ID)
	}

	if result[0].Score < 0.99 {
		t.Fatalf("expected score close to 1.0, got %f", result[0].Score)
	}
}

func TestRetrieveRespectsMinScore(t *testing.T) {
	store := &InMemoryChunkStore{
		Chunks: []Chunk{
			{ID: 1, Title: "Chunk1", URL: "http://a", Content: "content1", Embedding: []float32{1, 0, 0, 0}},
			{ID: 2, Title: "Chunk2", URL: "http://b", Content: "content2", Embedding: []float32{0, 1, 0, 0}},
		},
	}

	retriever := &Retriever{
		Embed:    &FakeEmbedder{embedding: []float32{1, 0, 0, 0}},
		Store:    store,
		MinScore: 0.9,
	}

	result, err := retriever.Retrieve(context.Background(), "test query", 5)
	if err != nil {
		t.Fatalf("Retrieve failed: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 result above threshold, got %d", len(result))
	}

	if result[0].ID != 1 {
		t.Fatalf("expected result to be Chunk1 (ID=1), got ID=%d", result[0].ID)
	}
}

func TestRetrieveEmptyQueryReturnsNil(t *testing.T) {
	store := &InMemoryChunkStore{
		Chunks: []Chunk{
			{ID: 1, Title: "Chunk1", URL: "http://a", Content: "content1", Embedding: []float32{1, 0, 0, 0}},
		},
	}

	retriever := &Retriever{
		Embed:    &FakeEmbedder{embedding: []float32{1, 0, 0, 0}},
		Store:    store,
		MinScore: 0.0,
	}

	result, err := retriever.Retrieve(context.Background(), "", 5)
	if err != nil {
		t.Fatalf("Retrieve failed: %v", err)
	}

	if result != nil {
		t.Fatalf("expected nil for empty query, got %v", result)
	}
}

func TestRetrieveTopK(t *testing.T) {
	store := &InMemoryChunkStore{
		Chunks: []Chunk{
			{ID: 1, Title: "Chunk1", URL: "http://a", Content: "content1", Embedding: []float32{1, 0, 0, 0}},
			{ID: 2, Title: "Chunk2", URL: "http://b", Content: "content2", Embedding: []float32{0.9, 0.1, 0, 0}},
			{ID: 3, Title: "Chunk3", URL: "http://c", Content: "content3", Embedding: []float32{0.8, 0.2, 0, 0}},
		},
	}

	retriever := &Retriever{
		Embed:    &FakeEmbedder{embedding: []float32{1, 0, 0, 0}},
		Store:    store,
		MinScore: 0.0,
	}

	result, err := retriever.Retrieve(context.Background(), "test query", 2)
	if err != nil {
		t.Fatalf("Retrieve failed: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 results (topK=2), got %d", len(result))
	}
}
