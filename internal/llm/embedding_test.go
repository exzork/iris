package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbeddingClient_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected Authorization header, got %s", r.Header.Get("Authorization"))
		}

		body, _ := io.ReadAll(r.Body)
		var req map[string]interface{}
		json.Unmarshal(body, &req)

		if req["model"] != "text-embedding-3-small" {
			t.Errorf("expected model text-embedding-3-small, got %v", req["model"])
		}

		// Return embedding with correct dimension (1536)
		embedding := make([]float32, 1536)
		for i := range embedding {
			embedding[i] = 0.1
		}

		response := map[string]interface{}{
			"object": "list",
			"data": []map[string]interface{}{
				{
					"object":    "embedding",
					"embedding": embedding,
					"index":     0,
				},
			},
			"model": "text-embedding-3-small",
			"usage": map[string]interface{}{
				"prompt_tokens": 10,
				"total_tokens":  10,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewEmbeddingClient(&EmbeddingConfig{
		APIKey:    "test-key",
		BaseURL:   server.URL,
		Model:     "text-embedding-3-small",
		Dimension: 1536,
	})

	ctx := context.Background()
	embedding, err := client.Embed(ctx, "test text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(embedding) != 1536 {
		t.Errorf("expected embedding dimension 1536, got %d", len(embedding))
	}

	if embedding[0] != 0.1 {
		t.Errorf("expected first value 0.1, got %f", embedding[0])
	}
}

func TestEmbeddingClient_DimensionMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return embedding with wrong dimension (768 instead of 1536)
		embedding := make([]float32, 768)
		for i := range embedding {
			embedding[i] = 0.1
		}

		response := map[string]interface{}{
			"object": "list",
			"data": []map[string]interface{}{
				{
					"object":    "embedding",
					"embedding": embedding,
					"index":     0,
				},
			},
			"model": "text-embedding-3-small",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewEmbeddingClient(&EmbeddingConfig{
		APIKey:    "test-key",
		BaseURL:   server.URL,
		Model:     "text-embedding-3-small",
		Dimension: 1536,
	})

	ctx := context.Background()
	_, err := client.Embed(ctx, "test text")
	if err == nil {
		t.Fatal("expected error for dimension mismatch")
	}
	if !strings.Contains(err.Error(), "dimension mismatch") {
		t.Errorf("expected 'dimension mismatch' error, got %v", err)
	}
}

func TestEmbeddingClient_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"object": "list",
			"data":   []map[string]interface{}{},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewEmbeddingClient(&EmbeddingConfig{
		APIKey:    "test-key",
		BaseURL:   server.URL,
		Model:     "text-embedding-3-small",
		Dimension: 1536,
	})

	ctx := context.Background()
	_, err := client.Embed(ctx, "test text")
	if err == nil {
		t.Fatal("expected error for empty response")
	}
	if !strings.Contains(err.Error(), "no embeddings") {
		t.Errorf("expected 'no embeddings' error, got %v", err)
	}
}

func TestEmbeddingClient_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"error": map[string]interface{}{
				"message": "Invalid API key",
				"type":    "invalid_request_error",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewEmbeddingClient(&EmbeddingConfig{
		APIKey:    "invalid-key",
		BaseURL:   server.URL,
		Model:     "text-embedding-3-small",
		Dimension: 1536,
	})

	ctx := context.Background()
	_, err := client.Embed(ctx, "test text")
	if err == nil {
		t.Fatal("expected error for API error")
	}
}
