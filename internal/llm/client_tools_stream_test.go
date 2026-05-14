package llm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// MockToolExecutor implements ToolExecutor for testing.
type MockToolExecutor struct {
	calls []struct {
		name string
		args map[string]interface{}
	}
	results map[string]string
}

func (m *MockToolExecutor) Execute(ctx context.Context, name string, args map[string]interface{}) (string, error) {
	m.calls = append(m.calls, struct {
		name string
		args map[string]interface{}
	}{name, args})
	if result, ok := m.results[name]; ok {
		return result, nil
	}
	return fmt.Sprintf("executed %s", name), nil
}

// TestChatWithToolsStream_NoToolCalls_StreamsText tests streaming text without tool calls.
func TestChatWithToolsStream_NoToolCalls_StreamsText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		// Emit text deltas
		deltas := []string{
			`{"choices":[{"delta":{"content":"Hello"},"finish_reason":null}]}`,
			`{"choices":[{"delta":{"content":" "},"finish_reason":null}]}`,
			`{"choices":[{"delta":{"content":"world"},"finish_reason":"stop"}]}`,
		}

		for _, delta := range deltas {
			fmt.Fprintf(w, "data: %s\n\n", delta)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	cfg := &Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "test-model",
	}
	client := NewClient(cfg)

	messages := []map[string]string{
		{"role": "user", "content": "Say hello"},
	}

	var deltas []string
	chatCfg := ChatWithToolsStreamConfig{
		Model:   "test-model",
		GuildID: 123,
		Tools:   []map[string]interface{}{},
		Exec:    &MockToolExecutor{results: make(map[string]string)},
		Max:     3,
		OnDelta: func(text string) {
			deltas = append(deltas, text)
		},
	}

	finalText, err := client.ChatWithToolsStream(context.Background(), messages, chatCfg)
	if err != nil {
		t.Fatalf("ChatWithToolsStream failed: %v", err)
	}

	if finalText != "Hello world" {
		t.Errorf("expected 'Hello world', got %q", finalText)
	}

	if len(deltas) != 3 {
		t.Errorf("expected 3 deltas, got %d", len(deltas))
	}
}

// TestChatWithToolsStream_OneToolCall_ThenStreamsText tests tool call followed by text streaming.
func TestChatWithToolsStream_OneToolCall_ThenStreamsText(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		callCount++
		if callCount == 1 {
			// First response: tool call with fragmented arguments
			deltas := []string{
				`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"ping"}}]},"finish_reason":null}]}`,
				`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"q\":\"hel"}}]},"finish_reason":null}]}`,
				`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"lo\"}"}}]},"finish_reason":"tool_calls"}]}`,
			}
			for _, delta := range deltas {
				fmt.Fprintf(w, "data: %s\n\n", delta)
			}
		} else {
			// Second response: text only
			deltas := []string{
				`{"choices":[{"delta":{"content":"Pong"},"finish_reason":null}]}`,
				`{"choices":[{"delta":{"content":"!"},"finish_reason":"stop"}]}`,
			}
			for _, delta := range deltas {
				fmt.Fprintf(w, "data: %s\n\n", delta)
			}
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	cfg := &Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "test-model",
	}
	client := NewClient(cfg)

	messages := []map[string]string{
		{"role": "user", "content": "Ping me"},
	}

	executor := &MockToolExecutor{results: map[string]string{"ping": "pong"}}
	chatCfg := ChatWithToolsStreamConfig{
		Model:   "test-model",
		GuildID: 123,
		Tools: []map[string]interface{}{
			{"type": "function", "function": map[string]interface{}{"name": "ping"}},
		},
		Exec: executor,
		Max:  3,
		OnDelta: func(text string) {
			// Ignore deltas in this test
		},
	}

	finalText, err := client.ChatWithToolsStream(context.Background(), messages, chatCfg)
	if err != nil {
		t.Fatalf("ChatWithToolsStream failed: %v", err)
	}

	if finalText != "Pong!" {
		t.Errorf("expected 'Pong!', got %q", finalText)
	}

	if len(executor.calls) != 1 {
		t.Errorf("expected 1 tool call, got %d", len(executor.calls))
	}

	if executor.calls[0].name != "ping" {
		t.Errorf("expected tool name 'ping', got %q", executor.calls[0].name)
	}

	expectedArgs := map[string]interface{}{"q": "hello"}
	if !mapsEqual(executor.calls[0].args, expectedArgs) {
		t.Errorf("expected args %v, got %v", expectedArgs, executor.calls[0].args)
	}
}

// TestChatWithToolsStream_MaxRounds_StopsGracefully tests max rounds limit.
func TestChatWithToolsStream_MaxRounds_StopsGracefully(t *testing.T) {
	roundCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		roundCount++
		// Always return a tool call
		delta := `{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"noop","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`
		fmt.Fprintf(w, "data: %s\n\n", delta)
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	cfg := &Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "test-model",
	}
	client := NewClient(cfg)

	messages := []map[string]string{
		{"role": "user", "content": "Loop"},
	}

	executor := &MockToolExecutor{results: map[string]string{"noop": "ok"}}
	chatCfg := ChatWithToolsStreamConfig{
		Model:   "test-model",
		GuildID: 123,
		Tools: []map[string]interface{}{
			{"type": "function", "function": map[string]interface{}{"name": "noop"}},
		},
		Exec: executor,
		Max:  2,
		OnDelta: func(text string) {
			// Ignore
		},
	}

	finalText, err := client.ChatWithToolsStream(context.Background(), messages, chatCfg)
	if err != nil {
		t.Fatalf("ChatWithToolsStream failed: %v", err)
	}

	if roundCount != 2 {
		t.Errorf("expected 2 rounds, got %d", roundCount)
	}

	if finalText != "" {
		t.Errorf("expected empty final text, got %q", finalText)
	}
}

// TestChatWithToolsStream_FragmentedArguments_AssembledBeforeExec tests argument assembly.
func TestChatWithToolsStream_FragmentedArguments_AssembledBeforeExec(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		callCount++
		if callCount == 1 {
			// First response: tool call with arguments in 4 fragments
			deltas := []string{
				`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"test","arguments":"{\"key\":\"val"}}]},"finish_reason":null}]}`,
				`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"ue\",\"count\":"}}]},"finish_reason":null}]}`,
				`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"42}"}}]},"finish_reason":"tool_calls"}]}`,
			}

			for _, delta := range deltas {
				fmt.Fprintf(w, "data: %s\n\n", delta)
			}
		} else {
			// Second response: empty text (tool executed, no more content)
			delta := `{"choices":[{"delta":{"content":""},"finish_reason":"stop"}]}`
			fmt.Fprintf(w, "data: %s\n\n", delta)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	cfg := &Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "test-model",
	}
	client := NewClient(cfg)

	messages := []map[string]string{
		{"role": "user", "content": "Test"},
	}

	executor := &MockToolExecutor{results: map[string]string{"test": "ok"}}
	chatCfg := ChatWithToolsStreamConfig{
		Model:   "test-model",
		GuildID: 123,
		Tools: []map[string]interface{}{
			{"type": "function", "function": map[string]interface{}{"name": "test"}},
		},
		Exec: executor,
		Max:  3,
		OnDelta: func(text string) {
			// Ignore
		},
	}

	_, err := client.ChatWithToolsStream(context.Background(), messages, chatCfg)
	if err != nil {
		t.Fatalf("ChatWithToolsStream failed: %v", err)
	}

	if len(executor.calls) != 1 {
		t.Errorf("expected 1 tool call, got %d", len(executor.calls))
	}

	expectedArgs := map[string]interface{}{"key": "value", "count": float64(42)}
	if !mapsEqual(executor.calls[0].args, expectedArgs) {
		t.Errorf("expected args %v, got %v", expectedArgs, executor.calls[0].args)
	}
}

// TestChatWithToolsStream_ParallelToolCalls tests multiple tool calls in one response.
func TestChatWithToolsStream_ParallelToolCalls(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		callCount++
		if callCount == 1 {
			// First response: two tool calls in parallel
			delta := `{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"a","function":{"name":"t1","arguments":"{}"}},{"index":1,"id":"b","function":{"name":"t2","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`
			fmt.Fprintf(w, "data: %s\n\n", delta)
		} else {
			// Second response: empty text (tools executed)
			delta := `{"choices":[{"delta":{"content":""},"finish_reason":"stop"}]}`
			fmt.Fprintf(w, "data: %s\n\n", delta)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	cfg := &Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "test-model",
	}
	client := NewClient(cfg)

	messages := []map[string]string{
		{"role": "user", "content": "Call two tools"},
	}

	executor := &MockToolExecutor{results: map[string]string{"t1": "result1", "t2": "result2"}}
	chatCfg := ChatWithToolsStreamConfig{
		Model:   "test-model",
		GuildID: 123,
		Tools: []map[string]interface{}{
			{"type": "function", "function": map[string]interface{}{"name": "t1"}},
			{"type": "function", "function": map[string]interface{}{"name": "t2"}},
		},
		Exec: executor,
		Max:  3,
		OnDelta: func(text string) {
			// Ignore
		},
	}

	_, err := client.ChatWithToolsStream(context.Background(), messages, chatCfg)
	if err != nil {
		t.Fatalf("ChatWithToolsStream failed: %v", err)
	}

	if len(executor.calls) != 2 {
		t.Errorf("expected 2 tool calls, got %d", len(executor.calls))
	}

	toolNames := map[string]bool{}
	for _, call := range executor.calls {
		toolNames[call.name] = true
	}

	if !toolNames["t1"] || !toolNames["t2"] {
		t.Errorf("expected both t1 and t2 to be called, got %v", toolNames)
	}
}

// TestChatWithToolsStream_InvalidArgsJSON_ReportsError tests invalid JSON handling.
func TestChatWithToolsStream_InvalidArgsJSON_ReportsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		// Emit tool call with invalid JSON arguments
		delta := `{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"bad","arguments":"{invalid json"}}]},"finish_reason":"tool_calls"}]}`
		fmt.Fprintf(w, "data: %s\n\n", delta)
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	cfg := &Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "test-model",
	}
	client := NewClient(cfg)

	messages := []map[string]string{
		{"role": "user", "content": "Bad args"},
	}

	executor := &MockToolExecutor{results: map[string]string{"bad": "ok"}}
	chatCfg := ChatWithToolsStreamConfig{
		Model:   "test-model",
		GuildID: 123,
		Tools: []map[string]interface{}{
			{"type": "function", "function": map[string]interface{}{"name": "bad"}},
		},
		Exec: executor,
		Max:  3,
		OnDelta: func(text string) {
			// Ignore
		},
	}

	_, err := client.ChatWithToolsStream(context.Background(), messages, chatCfg)
	if err != nil {
		t.Fatalf("ChatWithToolsStream failed: %v", err)
	}

	// Tool should not be executed due to invalid JSON
	if len(executor.calls) != 0 {
		t.Errorf("expected 0 tool calls (invalid JSON), got %d", len(executor.calls))
	}
}

// Helper function to compare maps
func mapsEqual(a, b map[string]interface{}) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || !valuesEqual(v, bv) {
			return false
		}
	}
	return true
}

func valuesEqual(a, b interface{}) bool {
	switch av := a.(type) {
	case float64:
		bv, ok := b.(float64)
		return ok && av == bv
	case string:
		bv, ok := b.(string)
		return ok && av == bv
	case bool:
		bv, ok := b.(bool)
		return ok && av == bv
	case map[string]interface{}:
		bv, ok := b.(map[string]interface{})
		return ok && mapsEqual(av, bv)
	default:
		return false
	}
}
