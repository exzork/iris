package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeToolExecutor struct {
	calls []struct {
		name string
		args map[string]interface{}
	}
	results map[string]string
	errors  map[string]error
}

func (f *fakeToolExecutor) Execute(ctx context.Context, name string, args map[string]interface{}) (string, error) {
	f.calls = append(f.calls, struct {
		name string
		args map[string]interface{}
	}{name, args})

	if err, ok := f.errors[name]; ok {
		return "", err
	}
	if result, ok := f.results[name]; ok {
		return result, nil
	}
	return "", fmt.Errorf("unknown tool: %s", name)
}

func TestChatWithTools_NoTools_FallsThroughToText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]interface{}
		json.Unmarshal(body, &req)

		// Verify no tools in request
		if tools, ok := req["tools"]; ok && tools != nil {
			t.Errorf("expected no tools in request, got %v", tools)
		}

		response := map[string]interface{}{
			"id":      "chatcmpl-123",
			"object":  "chat.completion",
			"created": 1234567890,
			"model":   "gpt-4",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "This is plain text response",
					},
					"finish_reason": "stop",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewClient(&Config{
		APIKey:      "test-key",
		BaseURL:     server.URL,
		Model:       "gpt-4",
		Temperature: 0.7,
		MaxTokens:   100,
	})

	ctx := context.Background()
	messages := []map[string]string{
		{"role": "user", "content": "Hello"},
	}

	cfg := ChatWithToolsConfig{
		Model:   "gpt-4",
		GuildID: 123,
		Tools:   []map[string]interface{}{}, // Empty tools
		Exec:    &fakeToolExecutor{},
		Max:     3,
	}

	response, err := client.ChatWithTools(ctx, messages, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if response != "This is plain text response" {
		t.Errorf("expected 'This is plain text response', got %s", response)
	}
}

func TestChatWithTools_OneToolCall_ThenText(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		body, _ := io.ReadAll(r.Body)
		var req map[string]interface{}
		json.Unmarshal(body, &req)

		if callCount == 1 {
			// First call: return a tool call
			response := map[string]interface{}{
				"id":      "chatcmpl-123",
				"object":  "chat.completion",
				"created": 1234567890,
				"model":   "gpt-4",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"message": map[string]interface{}{
							"role":    "assistant",
							"content": "I'll search for that",
							"tool_calls": []map[string]interface{}{
								{
									"id":   "call_123",
									"type": "function",
									"function": map[string]interface{}{
										"name":      "websearch",
										"arguments": `{"query":"test query"}`,
									},
								},
							},
						},
						"finish_reason": "tool_calls",
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		} else if callCount == 2 {
			// Second call: verify tool message is in request
			messages, ok := req["messages"].([]interface{})
			if !ok || len(messages) < 3 {
				t.Errorf("expected at least 3 messages in second request, got %d", len(messages))
			}

			// Check for tool message
			foundToolMsg := false
			for _, msg := range messages {
				msgMap, ok := msg.(map[string]interface{})
				if !ok {
					continue
				}
				if role, ok := msgMap["role"].(string); ok && role == "tool" {
					foundToolMsg = true
					if toolCallID, ok := msgMap["tool_call_id"].(string); !ok || toolCallID != "call_123" {
						t.Errorf("expected tool_call_id 'call_123', got %v", toolCallID)
					}
					if content, ok := msgMap["content"].(string); !ok || content != "search result" {
						t.Errorf("expected content 'search result', got %v", content)
					}
				}
			}
			if !foundToolMsg {
				t.Errorf("expected tool message in second request")
			}

			// Return plain text
			response := map[string]interface{}{
				"id":      "chatcmpl-124",
				"object":  "chat.completion",
				"created": 1234567891,
				"model":   "gpt-4",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"message": map[string]interface{}{
							"role":    "assistant",
							"content": "Here is the answer",
						},
						"finish_reason": "stop",
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}
	}))
	defer server.Close()

	client := NewClient(&Config{
		APIKey:      "test-key",
		BaseURL:     server.URL,
		Model:       "gpt-4",
		Temperature: 0.7,
		MaxTokens:   100,
	})

	ctx := context.Background()
	messages := []map[string]string{
		{"role": "user", "content": "Search for something"},
	}

	executor := &fakeToolExecutor{
		results: map[string]string{
			"websearch": "search result",
		},
	}

	cfg := ChatWithToolsConfig{
		Model:   "gpt-4",
		GuildID: 123,
		Tools: []map[string]interface{}{
			{
				"type": "function",
				"function": map[string]interface{}{
					"name":        "websearch",
					"description": "Search the web",
					"parameters": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"query": map[string]interface{}{
								"type":        "string",
								"description": "Search query",
							},
						},
						"required": []string{"query"},
					},
				},
			},
		},
		Exec: executor,
		Max:  3,
	}

	response, err := client.ChatWithTools(ctx, messages, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if response != "Here is the answer" {
		t.Errorf("expected 'Here is the answer', got %s", response)
	}

	if len(executor.calls) != 1 {
		t.Errorf("expected 1 tool call, got %d", len(executor.calls))
	}
	if executor.calls[0].name != "websearch" {
		t.Errorf("expected tool name 'websearch', got %s", executor.calls[0].name)
	}
}

func TestChatWithTools_MaxRounds_Exceeded_ReturnsLastText(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++

		// Always return a tool call
		response := map[string]interface{}{
			"id":      "chatcmpl-123",
			"object":  "chat.completion",
			"created": 1234567890,
			"model":   "gpt-4",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": fmt.Sprintf("Response %d", callCount),
						"tool_calls": []map[string]interface{}{
							{
								"id":   fmt.Sprintf("call_%d", callCount),
								"type": "function",
								"function": map[string]interface{}{
									"name":      "websearch",
									"arguments": `{"query":"test"}`,
								},
							},
						},
					},
					"finish_reason": "tool_calls",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewClient(&Config{
		APIKey:      "test-key",
		BaseURL:     server.URL,
		Model:       "gpt-4",
		Temperature: 0.7,
		MaxTokens:   100,
	})

	ctx := context.Background()
	messages := []map[string]string{
		{"role": "user", "content": "Search"},
	}

	executor := &fakeToolExecutor{
		results: map[string]string{
			"websearch": "result",
		},
	}

	cfg := ChatWithToolsConfig{
		Model:   "gpt-4",
		GuildID: 123,
		Tools: []map[string]interface{}{
			{
				"type": "function",
				"function": map[string]interface{}{
					"name": "websearch",
				},
			},
		},
		Exec: executor,
		Max:  2, // Max 2 rounds
	}

	response, err := client.ChatWithTools(ctx, messages, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should return the last assistant content
	if response != "Response 2" {
		t.Errorf("expected 'Response 2', got %s", response)
	}

	// Should have made exactly 2 requests
	if callCount != 2 {
		t.Errorf("expected 2 requests, got %d", callCount)
	}
}

func TestChatWithTools_ToolExecuteError_PropagatedAsToolMessage(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		body, _ := io.ReadAll(r.Body)
		var req map[string]interface{}
		json.Unmarshal(body, &req)

		if callCount == 1 {
			// First call: return a tool call
			response := map[string]interface{}{
				"id":      "chatcmpl-123",
				"object":  "chat.completion",
				"created": 1234567890,
				"model":   "gpt-4",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"message": map[string]interface{}{
							"role":    "assistant",
							"content": "I'll search",
							"tool_calls": []map[string]interface{}{
								{
									"id":   "call_123",
									"type": "function",
									"function": map[string]interface{}{
										"name":      "websearch",
										"arguments": `{"query":"test"}`,
									},
								},
							},
						},
						"finish_reason": "tool_calls",
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		} else if callCount == 2 {
			// Second call: verify error message is in request
			messages, ok := req["messages"].([]interface{})
			if !ok || len(messages) < 3 {
				t.Errorf("expected at least 3 messages in second request")
			}

			// Check for tool message with error
			foundErrorMsg := false
			for _, msg := range messages {
				msgMap, ok := msg.(map[string]interface{})
				if !ok {
					continue
				}
				if role, ok := msgMap["role"].(string); ok && role == "tool" {
					foundErrorMsg = true
					if content, ok := msgMap["content"].(string); !ok || content != "error: tool failed" {
						t.Errorf("expected error message, got %v", content)
					}
				}
			}
			if !foundErrorMsg {
				t.Errorf("expected error message in tool result")
			}

			// Return plain text
			response := map[string]interface{}{
				"id":      "chatcmpl-124",
				"object":  "chat.completion",
				"created": 1234567891,
				"model":   "gpt-4",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"message": map[string]interface{}{
							"role":    "assistant",
							"content": "Tool failed but I recovered",
						},
						"finish_reason": "stop",
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}
	}))
	defer server.Close()

	client := NewClient(&Config{
		APIKey:      "test-key",
		BaseURL:     server.URL,
		Model:       "gpt-4",
		Temperature: 0.7,
		MaxTokens:   100,
	})

	ctx := context.Background()
	messages := []map[string]string{
		{"role": "user", "content": "Search"},
	}

	executor := &fakeToolExecutor{
		errors: map[string]error{
			"websearch": fmt.Errorf("tool failed"),
		},
	}

	cfg := ChatWithToolsConfig{
		Model:   "gpt-4",
		GuildID: 123,
		Tools: []map[string]interface{}{
			{
				"type": "function",
				"function": map[string]interface{}{
					"name": "websearch",
				},
			},
		},
		Exec: executor,
		Max:  3,
	}

	response, err := client.ChatWithTools(ctx, messages, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if response != "Tool failed but I recovered" {
		t.Errorf("expected 'Tool failed but I recovered', got %s", response)
	}

	if len(executor.calls) != 1 {
		t.Errorf("expected 1 tool call, got %d", len(executor.calls))
	}
}
