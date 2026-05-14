package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestChatCompletion_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected Authorization header, got %s", r.Header.Get("Authorization"))
		}

		body, _ := io.ReadAll(r.Body)
		var req map[string]interface{}
		json.Unmarshal(body, &req)

		if req["model"] != "gpt-4" {
			t.Errorf("expected model gpt-4, got %v", req["model"])
		}

		if req["stream"] != false {
			t.Errorf("expected stream false, got %v", req["stream"])
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
						"content": "Hello, I am I.R.I.S",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{
				"prompt_tokens":     10,
				"completion_tokens": 5,
				"total_tokens":      15,
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

	response, err := client.Chat(ctx, 123, messages)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if response != "Hello, I am I.R.I.S" {
		t.Errorf("expected 'Hello, I am I.R.I.S', got %s", response)
	}
}

func TestChatCompletion_MalformedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"invalid": "response"}`))
	}))
	defer server.Close()

	client := NewClient(&Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "gpt-4",
	})

	ctx := context.Background()
	messages := []map[string]string{
		{"role": "user", "content": "Hello"},
	}

	_, err := client.Chat(ctx, 123, messages)
	if err == nil {
		t.Fatal("expected error for malformed response")
	}
	if !strings.Contains(err.Error(), "invalid response") {
		t.Errorf("expected 'invalid response' error, got %v", err)
	}
}

func TestChatCompletion_Timeout(t *testing.T) {
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
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "gpt-4",
		Timeout: 500 * time.Millisecond,
	})

	ctx := context.Background()
	messages := []map[string]string{
		{"role": "user", "content": "Hello"},
	}

	_, err := client.Chat(ctx, 123, messages)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "context deadline") && !strings.Contains(err.Error(), "timeout") {
		t.Errorf("expected timeout error, got %v", err)
	}
}

func TestChatCompletion_429Retry(t *testing.T) {
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
		APIKey:      "test-key",
		BaseURL:     server.URL,
		Model:       "gpt-4",
		MaxRetries:  2,
		RetryDelay:  100 * time.Millisecond,
	})

	ctx := context.Background()
	messages := []map[string]string{
		{"role": "user", "content": "Hello"},
	}

	response, err := client.Chat(ctx, 123, messages)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if response != "Success after retry" {
		t.Errorf("expected 'Success after retry', got %s", response)
	}

	if callCount != 2 {
		t.Errorf("expected 2 calls (1 retry), got %d", callCount)
	}
}

func TestToolCall_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "Tool result",
						"tool_calls": []map[string]interface{}{
							{
								"id":   "call_123",
								"type": "function",
								"function": map[string]interface{}{
									"name":      "search_meme",
									"arguments": `{"query":"funny cat"}`,
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
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "gpt-4",
	})

	ctx := context.Background()
	messages := []map[string]string{
		{"role": "user", "content": "Find a meme"},
	}

	toolCalls, err := client.ParseToolCalls(ctx, 123, messages)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(toolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(toolCalls))
	}

	if toolCalls[0].ToolName != "search_meme" {
		t.Errorf("expected tool name 'search_meme', got %s", toolCalls[0].ToolName)
	}

	if toolCalls[0].Arguments["query"] != "funny cat" {
		t.Errorf("expected query 'funny cat', got %v", toolCalls[0].Arguments["query"])
	}
}

func TestToolCall_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "Tool result",
						"tool_calls": []map[string]interface{}{
							{
								"id":   "call_123",
								"type": "function",
								"function": map[string]interface{}{
									"name":      "search_meme",
									"arguments": `{"query":"funny cat"`,
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
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "gpt-4",
	})

	ctx := context.Background()
	messages := []map[string]string{
		{"role": "user", "content": "Find a meme"},
	}

	_, err := client.ParseToolCalls(ctx, 123, messages)
	if err == nil {
		t.Fatal("expected error for malformed JSON arguments")
	}
	if !strings.Contains(err.Error(), "invalid tool arguments") {
		t.Errorf("expected 'invalid tool arguments' error, got %v", err)
	}
}

func TestToolCall_NoSecretLogging(t *testing.T) {
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

	client := NewClient(&Config{
		APIKey:  "test-key-should-not-appear",
		BaseURL: server.URL,
		Model:   "gpt-4",
	})

	ctx := context.Background()
	messages := []map[string]string{
		{"role": "user", "content": "Secret message"},
	}

	_, _ = client.Chat(ctx, 123, messages)
	_, err := client.Chat(ctx, 123, messages)
	if err != nil && strings.Contains(err.Error(), "test-key-should-not-appear") {
		t.Error("API key leaked in error message")
	}
}

func TestCallTool_Integration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "Tool executed successfully",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewClient(&Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "gpt-4",
	})

	ctx := context.Background()
	result, err := client.CallTool(ctx, 123, "search_meme", map[string]interface{}{
		"query": "funny cat",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "Tool executed successfully" {
		t.Errorf("expected 'Tool executed successfully', got %s", result)
	}
}

func TestChatCompletion_429ExhaustedRetries(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{
				"message": "rate limit exceeded",
				"type":    "server_error",
			},
		})
	}))
	defer server.Close()

	client := NewClient(&Config{
		APIKey:      "test-key",
		BaseURL:     server.URL,
		Model:       "gpt-4",
		MaxRetries:  1,
		RetryDelay:  50 * time.Millisecond,
	})

	ctx := context.Background()
	messages := []map[string]string{
		{"role": "user", "content": "Hello"},
	}

	_, err := client.Chat(ctx, 123, messages)
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}

	if !strings.Contains(err.Error(), "rate limit") && !strings.Contains(err.Error(), "429") {
		t.Errorf("expected rate limit error, got %v", err)
	}

	if callCount < 2 {
		t.Errorf("expected at least 2 calls (initial + 1 retry), got %d", callCount)
	}
}

func TestAuditLogging_DebugEnabled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "Hello, I am I.R.I.S",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	var logOutput strings.Builder
	mockLogger := &mockLogger{output: &logOutput}

	SetDebug(true, mockLogger)
	defer SetDebug(false, nil)

	client := NewClient(&Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "gpt-4",
	})

	ctx := context.Background()
	meta := &ContextMeta{
		GuildID:   999,
		ChannelID: 111,
		MessageID: 222,
		UserID:    333,
		Tier:      "default",
	}
	ctx = WithMeta(ctx, meta)

	messages := []map[string]string{
		{"role": "user", "content": "Hello"},
	}

	response, err := client.Chat(ctx, 999, messages)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if response != "Hello, I am I.R.I.S" {
		t.Errorf("expected response, got %s", response)
	}

	output := logOutput.String()
	if !strings.Contains(output, "llm_audit") {
		t.Errorf("expected audit log message, got %q", output)
	}
	if !strings.Contains(output, "request_id=") {
		t.Errorf("expected request_id in audit log, got %q", output)
	}
	if !strings.Contains(output, "999") {
		t.Errorf("expected guild_id in audit log, got %q", output)
	}
	if !strings.Contains(output, "success") {
		t.Errorf("expected status=success in audit log, got %q", output)
	}
}

func TestAuditLogging_DebugDisabled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "Hello, I am I.R.I.S",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	var logOutput strings.Builder
	mockLogger := &mockLogger{output: &logOutput}

	SetDebug(false, mockLogger)

	client := NewClient(&Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "gpt-4",
	})

	ctx := context.Background()
	messages := []map[string]string{
		{"role": "user", "content": "Hello"},
	}

	response, err := client.Chat(ctx, 999, messages)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if response != "Hello, I am I.R.I.S" {
		t.Errorf("expected response, got %s", response)
	}

	output := logOutput.String()
	if strings.Contains(output, "llm_audit") {
		t.Errorf("expected no audit log when debug disabled, got %q", output)
	}
}

func TestAuditLogging_ErrorCase(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{
				"message": "internal server error",
				"type":    "server_error",
			},
		})
	}))
	defer server.Close()

	var logOutput strings.Builder
	mockLogger := &mockLogger{output: &logOutput}

	SetDebug(true, mockLogger)
	defer SetDebug(false, nil)

	client := NewClient(&Config{
		APIKey:      "test-key",
		BaseURL:     server.URL,
		Model:       "gpt-4",
		MaxRetries:  0,
		RetryDelay:  10 * time.Millisecond,
	})

	ctx := context.Background()
	messages := []map[string]string{
		{"role": "user", "content": "Hello"},
	}

	_, err := client.Chat(ctx, 999, messages)
	if err == nil {
		t.Fatal("expected error")
	}

	output := logOutput.String()
	if !strings.Contains(output, "llm_audit") {
		t.Errorf("expected audit log on error, got %q", output)
	}
	if !strings.Contains(output, "error") {
		t.Errorf("expected status=error in audit log, got %q", output)
	}
	if !strings.Contains(output, "http_status_5xx") {
		t.Errorf("expected error_class=http_status_5xx in audit log, got %q", output)
	}
}

type mockLogger struct {
	output *strings.Builder
}

func (m *mockLogger) Debug(ctx context.Context, msg string, args ...any) {
	m.output.WriteString(msg)
	m.output.WriteString(" ")
	for i := 0; i < len(args); i += 2 {
		if i > 0 {
			m.output.WriteString(" ")
		}
		if key, ok := args[i].(string); ok {
			m.output.WriteString(key)
			m.output.WriteString("=")
		}
		if i+1 < len(args) {
			m.output.WriteString(fmt.Sprintf("%v", args[i+1]))
		}
	}
	m.output.WriteString("\n")
}

func (m *mockLogger) Info(ctx context.Context, msg string, args ...any)   {}
func (m *mockLogger) Warn(ctx context.Context, msg string, args ...any)   {}
func (m *mockLogger) Error(ctx context.Context, msg string, args ...any)  {}

func ensureCorrelationID(ctx context.Context) (context.Context, string) {
	return context.WithValue(ctx, corrIDKey{}, "test-corr-id"), "test-corr-id"
}

type corrIDKey struct{}
