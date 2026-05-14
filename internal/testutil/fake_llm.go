package testutil

import (
	"context"
	"errors"
	"sync"
	"time"
)

// FakeLLMClient implements domain.LLMClient for testing.
type FakeLLMClient struct {
	ChatFn            func(ctx context.Context, guildID int64, messages []map[string]string) (string, error)
	ChatWithModelFn   func(ctx context.Context, model string, guildID int64, messages []map[string]string) (string, error)
	CallToolFn        func(ctx context.Context, guildID int64, toolName string, arguments map[string]interface{}) (string, error)

	SimulateLatency      time.Duration
	SimulateError        error
	Simulate429          bool
	SimulateMalformed    bool
	ChatResponses        map[string]string
	ToolResponses        map[string]string
	CallLog              []ToolCall
	LastModelUsed        string
	chatCallCount        int
	mu                   sync.Mutex
}

// ToolCall tracks a tool call for verification.
type ToolCall struct {
	ToolName  string
	Arguments map[string]interface{}
	Timestamp time.Time
}

// NewFakeLLMClient creates a new fake LLM client.
func NewFakeLLMClient() *FakeLLMClient {
	return &FakeLLMClient{
		ChatResponses: make(map[string]string),
		ToolResponses: make(map[string]string),
		CallLog:       []ToolCall{},
	}
}

// Chat sends a chat request.
func (f *FakeLLMClient) Chat(ctx context.Context, guildID int64, messages []map[string]string) (string, error) {
	f.mu.Lock()
	f.chatCallCount++
	f.mu.Unlock()

	if f.SimulateLatency > 0 {
		select {
		case <-time.After(f.SimulateLatency):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}

	if f.Simulate429 {
		return "", errors.New("429 Too Many Requests")
	}

	if f.SimulateError != nil {
		return "", f.SimulateError
	}

	if f.ChatFn != nil {
		return f.ChatFn(ctx, guildID, messages)
	}

	if f.SimulateMalformed {
		return `{"incomplete": "json"`, nil
	}

	if len(messages) > 0 {
		if msg, ok := messages[len(messages)-1]["content"]; ok {
			if resp, exists := f.ChatResponses[msg]; exists {
				return resp, nil
			}
		}
	}

	return "default response", nil
}

// ChatWithModel sends a chat request with an explicit model.
func (f *FakeLLMClient) ChatWithModel(ctx context.Context, model string, guildID int64, messages []map[string]string) (string, error) {
	f.mu.Lock()
	f.LastModelUsed = model
	f.mu.Unlock()

	if f.ChatWithModelFn != nil {
		return f.ChatWithModelFn(ctx, model, guildID, messages)
	}

	// Delegate to Chat if no custom handler
	return f.Chat(ctx, guildID, messages)
}

// CallTool executes a tool call.
func (f *FakeLLMClient) CallTool(ctx context.Context, guildID int64, toolName string, arguments map[string]interface{}) (string, error) {
	if f.SimulateLatency > 0 {
		select {
		case <-time.After(f.SimulateLatency):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}

	if f.Simulate429 {
		return "", errors.New("429 Too Many Requests")
	}

	if f.SimulateError != nil {
		return "", f.SimulateError
	}

	f.mu.Lock()
	f.CallLog = append(f.CallLog, ToolCall{
		ToolName:  toolName,
		Arguments: arguments,
		Timestamp: time.Now(),
	})
	f.mu.Unlock()

	if f.CallToolFn != nil {
		return f.CallToolFn(ctx, guildID, toolName, arguments)
	}

	if f.SimulateMalformed {
		return `{"malformed": "tool_response"`, nil
	}

	if resp, exists := f.ToolResponses[toolName]; exists {
		return resp, nil
	}

	return `{"result": "tool executed"}`, nil
}

// GetCallLog returns all tool calls for verification.
func (f *FakeLLMClient) GetCallLog() []ToolCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]ToolCall{}, f.CallLog...)
}

// ClearCallLog clears the call log.
func (f *FakeLLMClient) ClearCallLog() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.CallLog = []ToolCall{}
}

// CallCount returns the number of Chat calls made.
func (f *FakeLLMClient) CallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.chatCallCount
}
