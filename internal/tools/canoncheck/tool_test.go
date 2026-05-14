package canoncheck

import (
	"context"
	"encoding/json"
	"testing"

	ragpkg "github.com/eko/iris-bot/internal/lore/rag"
)

func TestToolSchemaContract(t *testing.T) {
	store := &ragpkg.InMemoryChunkStore{}
	embedding := []float32{1, 0, 0, 0}
	provider := &fakeEmbeddingProvider{embedding: embedding}
	retriever := &ragpkg.Retriever{Embed: provider, Store: store, MinScore: 0.0}
	verifier := NewVerifier(retriever)
	tool := New(verifier)

	schema := tool.Schema()

	if schema.Name != "canon_check" {
		t.Errorf("expected name 'canon_check', got %q", schema.Name)
	}

	if schema.Description == "" {
		t.Errorf("expected non-empty description")
	}

	if len(schema.Fields) != 2 {
		t.Errorf("expected 2 fields, got %d", len(schema.Fields))
	}

	claimField := schema.Fields[0]
	if claimField.Name != "claim" || !claimField.Required {
		t.Errorf("claim field misconfigured: name=%q, required=%v", claimField.Name, claimField.Required)
	}

	queryField := schema.Fields[1]
	if queryField.Name != "query" || queryField.Required {
		t.Errorf("query field misconfigured: name=%q, required=%v", queryField.Name, queryField.Required)
	}
}

func TestToolRunReturnsJSONVerdict(t *testing.T) {
	store := &ragpkg.InMemoryChunkStore{}
	store.Chunks = append(store.Chunks, ragpkg.Chunk{
		ID:        1,
		Title:     "Quest Y",
		URL:       "https://wiki.example.com/quest-y",
		Content:   "Rover appears in Quest Y",
		Embedding: []float32{1, 0, 0, 0},
	})
	store.Chunks = append(store.Chunks, ragpkg.Chunk{
		ID:        2,
		Title:     "Characters",
		URL:       "https://wiki.example.com/characters",
		Content:   "Rover is featured in Quest Y",
		Embedding: []float32{1, 0, 0, 0},
	})

	embedding := []float32{1, 0, 0, 0}
	provider := &fakeEmbeddingProvider{embedding: embedding}
	retriever := &ragpkg.Retriever{Embed: provider, Store: store, MinScore: 0.0}
	verifier := NewVerifier(retriever)
	tool := New(verifier)

	args := map[string]interface{}{
		"claim": "Rover appears in Quest Y",
	}

	result, err := tool.Run(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var verdict map[string]interface{}
	err = json.Unmarshal([]byte(result), &verdict)
	if err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	if verdict["status"] != "supported" {
		t.Errorf("expected status 'supported', got %v", verdict["status"])
	}

	if _, ok := verdict["confidence"]; !ok {
		t.Errorf("expected confidence field in result")
	}

	if _, ok := verdict["reason"]; !ok {
		t.Errorf("expected reason field in result")
	}

	if _, ok := verdict["citations"]; !ok {
		t.Errorf("expected citations field in result")
	}
}

func TestToolRunMissingClaimError(t *testing.T) {
	store := &ragpkg.InMemoryChunkStore{}
	embedding := []float32{1, 0, 0, 0}
	provider := &fakeEmbeddingProvider{embedding: embedding}
	retriever := &ragpkg.Retriever{Embed: provider, Store: store, MinScore: 0.0}
	verifier := NewVerifier(retriever)
	tool := New(verifier)

	args := map[string]interface{}{}

	_, err := tool.Run(context.Background(), args)
	if err == nil {
		t.Errorf("expected error for missing claim, got nil")
	}
}

func TestToolRunPassesQueryHint(t *testing.T) {
	store := &ragpkg.InMemoryChunkStore{}
	store.Chunks = append(store.Chunks, ragpkg.Chunk{
		ID:        1,
		Title:     "Quest Y",
		URL:       "https://wiki.example.com/quest-y",
		Content:   "Rover appears in Quest Y",
		Embedding: []float32{1, 0, 0, 0},
	})

	embedding := []float32{1, 0, 0, 0}
	provider := &fakeEmbeddingProvider{embedding: embedding}
	retriever := &ragpkg.Retriever{Embed: provider, Store: store, MinScore: 0.0}
	verifier := NewVerifier(retriever)
	tool := New(verifier)

	args := map[string]interface{}{
		"claim": "Rover appears in Quest Y",
		"query": "custom search query",
	}

	result, err := tool.Run(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var verdict map[string]interface{}
	err = json.Unmarshal([]byte(result), &verdict)
	if err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	if verdict["status"] == "" {
		t.Errorf("expected non-empty status")
	}

	t.Logf("Tool result with query hint: %s", result)
}
