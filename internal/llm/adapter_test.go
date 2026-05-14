package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAdapter_ChatImplementsPort(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "Response from adapter",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	adapter := NewAdapter(&Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "gpt-4",
	})

	ctx := context.Background()
	messages := []map[string]string{
		{"role": "user", "content": "Hello"},
	}

	response, err := adapter.Chat(ctx, 123, messages)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if response != "Response from adapter" {
		t.Errorf("expected 'Response from adapter', got %s", response)
	}
}

func TestAdapter_CallToolImplementsPort(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "Tool result",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	adapter := NewAdapter(&Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "gpt-4",
	})

	ctx := context.Background()
	result, err := adapter.CallTool(ctx, 123, "search_meme", map[string]interface{}{
		"query": "funny cat",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "Tool result" {
		t.Errorf("expected 'Tool result', got %s", result)
	}
}

func TestAdapter_EmptyMessages(t *testing.T) {
	adapter := NewAdapter(&Config{
		APIKey:  "test-key",
		BaseURL: "http://localhost",
		Model:   "gpt-4",
	})

	ctx := context.Background()
	_, err := adapter.Chat(ctx, 123, []map[string]string{})

	if err == nil {
		t.Fatal("expected error for empty messages")
	}
}

func TestAdapter_EmptyToolName(t *testing.T) {
	adapter := NewAdapter(&Config{
		APIKey:  "test-key",
		BaseURL: "http://localhost",
		Model:   "gpt-4",
	})

	ctx := context.Background()
	_, err := adapter.CallTool(ctx, 123, "", map[string]interface{}{})

	if err == nil {
		t.Fatal("expected error for empty tool name")
	}
}
