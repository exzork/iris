package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestQA_ChatCompletionScenario(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"id":      "chatcmpl-qa-001",
			"object":  "chat.completion",
			"created": 1234567890,
			"model":   "gpt-4",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "Kamu adalah I.R.I.S dari Wuthering Waves. Siap membantu dengan informasi lore.",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{
				"prompt_tokens":     15,
				"completion_tokens": 20,
				"total_tokens":      35,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	adapter := NewAdapter(&Config{
		APIKey:      "sk-test-key",
		BaseURL:     server.URL,
		Model:       "gpt-4",
		Temperature: 0.7,
		MaxTokens:   2048,
		Timeout:     30 * time.Second,
		MaxRetries:  3,
		RetryDelay:  1 * time.Second,
	})

	ctx := context.Background()
	messages := []map[string]string{
		{"role": "system", "content": "You are I.R.I.S from Wuthering Waves"},
		{"role": "user", "content": "Siapa kamu?"},
	}

	response, err := adapter.Chat(ctx, 123456789, messages)
	if err != nil {
		t.Fatalf("QA Chat Completion failed: %v", err)
	}

	if response == "" {
		t.Fatal("QA Chat Completion: empty response")
	}

	t.Logf("✓ QA Chat Completion: Got response: %s", response)
}

func TestQA_ToolCallParsingScenario(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "Searching for memes...",
						"tool_calls": []map[string]interface{}{
							{
								"id":   "call_meme_001",
								"type": "function",
								"function": map[string]interface{}{
									"name":      "search_meme",
									"arguments": `{"query":"funny cat","limit":5}`,
								},
							},
						},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewClient(&Config{
		APIKey:  "sk-test-key",
		BaseURL: server.URL,
		Model:   "gpt-4",
	})

	ctx := context.Background()
	messages := []map[string]string{
		{"role": "user", "content": "Find a funny meme"},
	}

	toolCalls, err := client.ParseToolCalls(ctx, 123456789, messages)
	if err != nil {
		t.Fatalf("QA Tool Call Parsing failed: %v", err)
	}

	if len(toolCalls) != 1 {
		t.Fatalf("QA Tool Call Parsing: expected 1 tool call, got %d", len(toolCalls))
	}

	if toolCalls[0].ToolName != "search_meme" {
		t.Fatalf("QA Tool Call Parsing: expected tool name 'search_meme', got %s", toolCalls[0].ToolName)
	}

	if toolCalls[0].Arguments["query"] != "funny cat" {
		t.Fatalf("QA Tool Call Parsing: expected query 'funny cat', got %v", toolCalls[0].Arguments["query"])
	}

	if toolCalls[0].Arguments["limit"] != float64(5) {
		t.Fatalf("QA Tool Call Parsing: expected limit 5, got %v", toolCalls[0].Arguments["limit"])
	}

	t.Logf("✓ QA Tool Call Parsing: Successfully parsed tool call with name=%s, query=%s, limit=%v",
		toolCalls[0].ToolName, toolCalls[0].Arguments["query"], toolCalls[0].Arguments["limit"])
}

func TestQA_MalformedResponseHandlingScenario(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"invalid": "structure", "missing": "choices"}`))
	}))
	defer server.Close()

	client := NewClient(&Config{
		APIKey:  "sk-test-key",
		BaseURL: server.URL,
		Model:   "gpt-4",
	})

	ctx := context.Background()
	messages := []map[string]string{
		{"role": "user", "content": "Hello"},
	}

	_, err := client.Chat(ctx, 123456789, messages)
	if err == nil {
		t.Fatal("QA Malformed Response: expected error but got none")
	}

	t.Logf("✓ QA Malformed Response: Correctly rejected malformed response with error: %v", err)
}

func TestQA_TimeoutHandlingScenario(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"content": "response"}},
			},
		})
	}))
	defer server.Close()

	client := NewClient(&Config{
		APIKey:  "sk-test-key",
		BaseURL: server.URL,
		Model:   "gpt-4",
		Timeout: 500 * time.Millisecond,
	})

	ctx := context.Background()
	messages := []map[string]string{
		{"role": "user", "content": "Hello"},
	}

	_, err := client.Chat(ctx, 123456789, messages)
	if err == nil {
		t.Fatal("QA Timeout: expected timeout error but got none")
	}

	t.Logf("✓ QA Timeout: Correctly handled timeout with error: %v", err)
}

func TestQA_429RetryBackoffScenario(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]interface{}{
					"message": "rate limit exceeded",
					"type":    "server_error",
				},
			})
			return
		}

		response := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "Success after retry",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewClient(&Config{
		APIKey:     "sk-test-key",
		BaseURL:    server.URL,
		Model:      "gpt-4",
		MaxRetries: 2,
		RetryDelay: 100 * time.Millisecond,
	})

	ctx := context.Background()
	messages := []map[string]string{
		{"role": "user", "content": "Hello"},
	}

	response, err := client.Chat(ctx, 123456789, messages)
	if err != nil {
		t.Fatalf("QA 429 Retry: unexpected error: %v", err)
	}

	if response != "Success after retry" {
		t.Fatalf("QA 429 Retry: expected 'Success after retry', got %s", response)
	}

	if callCount != 2 {
		t.Fatalf("QA 429 Retry: expected 2 calls (1 retry), got %d", callCount)
	}

	t.Logf("✓ QA 429 Retry: Successfully retried after rate limit (calls=%d)", callCount)
}
