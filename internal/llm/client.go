package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/eko/iris-bot/internal/obs"
	"github.com/eko/iris-bot/internal/safety"
)

type Logger interface {
	Debug(ctx context.Context, msg string, args ...any)
}

var debugEnabled bool
var logger Logger

type Config struct {
	APIKey      string
	BaseURL     string
	Model       string
	Temperature float32
	MaxTokens   int
	Timeout     time.Duration
	MaxRetries  int
	RetryDelay  time.Duration
}

type Client struct {
	config *Config
	http   *http.Client
}

type ToolCall struct {
	ID        string
	ToolName  string
	Arguments map[string]interface{}
}

// ToolExecutor executes a tool by name with the given arguments.
type ToolExecutor interface {
	Execute(ctx context.Context, name string, args map[string]interface{}) (string, error)
}

// StreamCallbacks defines callbacks for streaming responses.
type StreamCallbacks struct {
	OnDelta func(text string)
	OnDone  func()
	OnError func(err error)
}

// ChatWithToolsConfig configures the ChatWithTools method.
type ChatWithToolsConfig struct {
	Model   string                   // LLM model to use
	GuildID int64                    // Guild ID for audit logging
	Tools   []map[string]interface{} // OpenAI-shaped tool definitions
	Exec    ToolExecutor             // Tool executor
	Max     int                      // Max tool-call rounds; default 3 if 0
}

// ChatWithToolsStreamConfig configures the ChatWithToolsStream method.
type ChatWithToolsStreamConfig struct {
	Model   string                   // LLM model to use
	GuildID int64                    // Guild ID for audit logging
	Tools   []map[string]interface{} // OpenAI-shaped tool definitions
	Exec    ToolExecutor             // Tool executor
	Max     int                      // Max tool-call rounds; default 3 if 0
	OnDelta func(text string)        // Callback fired on assistant content fragments
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []interface{} `json:"messages"`
	Temperature float32       `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Tools       []interface{} `json:"tools,omitempty"`
	Stream      bool          `json:"stream"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Role      string `json:"role"`
			Content   string `json:"content"`
			ToolCalls []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

type sseDelta struct {
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int `json:"index"`
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

type pendingToolCall struct {
	id      string
	name    string
	argsBuf strings.Builder
}

func NewClient(cfg *Config) *Client {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	maxRetries := cfg.MaxRetries
	if maxRetries == 0 {
		maxRetries = 3
	}

	retryDelay := cfg.RetryDelay
	if retryDelay == 0 {
		retryDelay = 1 * time.Second
	}

	cfg.MaxRetries = maxRetries
	cfg.RetryDelay = retryDelay

	return &Client{
		config: cfg,
		http: &http.Client{
			Timeout: timeout,
		},
	}
}

func SetDebug(enabled bool, log Logger) {
	debugEnabled = enabled
	logger = log
}

func (c *Client) Chat(ctx context.Context, guildID int64, messages []map[string]string) (string, error) {
	return c.ChatWithModel(ctx, c.config.Model, guildID, messages)
}

func (c *Client) ChatWithModel(ctx context.Context, model string, guildID int64, messages []map[string]string) (string, error) {
	startedAt := time.Now()
	var requestID string
	var retryCount int

	if debugEnabled && logger != nil {
		corrID := obs.CorrelationID(ctx)
		if corrID == "" {
			ctx, corrID = obs.EnsureCorrelationID(ctx)
		}
		requestID = corrID
	}

	msgInterfaces := make([]interface{}, len(messages))
	for i, m := range messages {
		msgInterfaces[i] = m
	}

	req := chatRequest{
		Model:       model,
		Messages:    msgInterfaces,
		Temperature: c.config.Temperature,
		MaxTokens:   c.config.MaxTokens,
		Stream:      false,
	}

	resp, err := c.doRequestWithRetryTracking(ctx, req, &retryCount)
	finishedAt := time.Now()
	duration := finishedAt.Sub(startedAt)

	if debugEnabled && logger != nil {
		redactor := safety.NewSecretRedactor()
		meta := MetaFromContext(ctx)
		rec := BuildAuditRecord(
			requestID, model, guildID, messages,
			func() string {
				if resp != nil && len(resp.Choices) > 0 {
					return resp.Choices[0].Message.Content
				}
				return ""
			}(),
			err, duration, retryCount, startedAt, finishedAt, meta, redactor,
		)
		args := RecordToLogArgs(rec)
		logger.Debug(ctx, "llm_audit", args...)
	}

	if err != nil {
		return "", err
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("invalid response: no choices returned")
	}

	return resp.Choices[0].Message.Content, nil
}

func (c *Client) ChatStream(
	ctx context.Context,
	model string,
	guildID int64,
	messages []map[string]string,
	cb StreamCallbacks,
) (string, error) {
	startedAt := time.Now()
	var requestID string
	var retryCount int

	if debugEnabled && logger != nil {
		corrID := obs.CorrelationID(ctx)
		if corrID == "" {
			ctx, corrID = obs.EnsureCorrelationID(ctx)
		}
		requestID = corrID
	}

	ctx = WithMeta(ctx, &ContextMeta{
		TriggerReason: "chat_stream",
		GuildID:       guildID,
	})

	if model == "" {
		model = c.config.Model
	}

	msgInterfaces := make([]interface{}, len(messages))
	for i, m := range messages {
		msgInterfaces[i] = m
	}

	req := chatRequest{
		Model:       model,
		Messages:    msgInterfaces,
		Temperature: c.config.Temperature,
		MaxTokens:   c.config.MaxTokens,
		Stream:      true,
	}

	finalText, err := c.doStreamWithRetryTracking(ctx, req, cb, &retryCount)
	finishedAt := time.Now()
	duration := finishedAt.Sub(startedAt)

	if debugEnabled && logger != nil {
		redactor := safety.NewSecretRedactor()
		meta := MetaFromContext(ctx)
		rec := BuildAuditRecord(
			requestID, model, guildID, messages,
			finalText,
			err, duration, retryCount, startedAt, finishedAt, meta, redactor,
		)
		args := RecordToLogArgs(rec)
		logger.Debug(ctx, "llm_audit", args...)
	}

	return finalText, err
}

func (c *Client) ChatWithToolsStream(
	ctx context.Context,
	messages []map[string]string,
	cfg ChatWithToolsStreamConfig,
) (string, error) {
	startedAt := time.Now()
	var requestID string
	var retryCount int

	if debugEnabled && logger != nil {
		corrID := obs.CorrelationID(ctx)
		if corrID == "" {
			ctx, corrID = obs.EnsureCorrelationID(ctx)
		}
		requestID = corrID
	}

	ctx = WithMeta(ctx, &ContextMeta{
		TriggerReason: "chat_with_tools_stream",
		GuildID:       cfg.GuildID,
	})

	maxRounds := cfg.Max
	if maxRounds == 0 {
		maxRounds = 3
	}

	richMessages := make([]map[string]interface{}, len(messages))
	for i, m := range messages {
		richMessages[i] = make(map[string]interface{})
		for k, v := range m {
			richMessages[i][k] = v
		}
	}

	var lastAssistantContent string
	var lastErr error

	for round := 0; round < maxRounds; round++ {
		msgInterfaces := make([]interface{}, len(richMessages))
		for i, m := range richMessages {
			msgInterfaces[i] = m
		}

		req := chatRequest{
			Model:       cfg.Model,
			Messages:    msgInterfaces,
			Temperature: c.config.Temperature,
			MaxTokens:   c.config.MaxTokens,
			Stream:      true,
		}

		if len(cfg.Tools) > 0 {
			toolInterfaces := make([]interface{}, len(cfg.Tools))
			for i, t := range cfg.Tools {
				toolInterfaces[i] = t
			}
			req.Tools = toolInterfaces
		}

		var roundText string
		var toolCalls []struct {
			ID   string
			Name string
			Args string
		}
		var finishReason string

		streamErr := c.doStreamWithToolsRetryTracking(ctx, req, &retryCount, func(text string) {
			roundText += text
			if cfg.OnDelta != nil {
				cfg.OnDelta(text)
			}
		}, func(tc struct {
			ID   string
			Name string
			Args string
		}) {
			toolCalls = append(toolCalls, tc)
		}, &finishReason)

		if streamErr != nil {
			lastErr = streamErr
			break
		}

		lastAssistantContent = roundText

		if len(toolCalls) == 0 {
			finishedAt := time.Now()
			duration := finishedAt.Sub(startedAt)

			if debugEnabled && logger != nil {
				redactor := safety.NewSecretRedactor()
				meta := MetaFromContext(ctx)
				rec := BuildAuditRecord(
					requestID, cfg.Model, cfg.GuildID, messages,
					lastAssistantContent,
					nil, duration, retryCount, startedAt, finishedAt, meta, redactor,
				)
				args := RecordToLogArgs(rec)
				logger.Debug(ctx, "llm_audit", args...)
			}

			return lastAssistantContent, nil
		}

		assistantMsg := map[string]interface{}{
			"role":    "assistant",
			"content": roundText,
		}

		toolCallsArray := make([]map[string]interface{}, len(toolCalls))
		for i, tc := range toolCalls {
			toolCallsArray[i] = map[string]interface{}{
				"id": tc.ID,
				"type": "function",
				"function": map[string]interface{}{
					"name": tc.Name,
				},
			}
		}
		assistantMsg["tool_calls"] = toolCallsArray
		richMessages = append(richMessages, assistantMsg)

		for _, tc := range toolCalls {
			var args map[string]interface{}
			if err := json.Unmarshal([]byte(tc.Args), &args); err != nil {
				toolMsg := map[string]interface{}{
					"role":        "tool",
					"tool_call_id": tc.ID,
					"content":     "error: invalid JSON arguments",
				}
				richMessages = append(richMessages, toolMsg)
				continue
			}

			if debugEnabled && logger != nil {
				argsStr := fmt.Sprintf("%v", args)
				if len(argsStr) > 100 {
					argsStr = argsStr[:100] + "..."
				}
				logger.Debug(ctx, "tool_call", "round", round+1, "tool", tc.Name, "args", argsStr)
			}

			toolResult := ""
			if cfg.Exec != nil {
				result, execErr := cfg.Exec.Execute(ctx, tc.Name, args)
				if execErr != nil {
					toolResult = "error: " + execErr.Error()
				} else {
					toolResult = result
				}
			}

			toolMsg := map[string]interface{}{
				"role":        "tool",
				"tool_call_id": tc.ID,
				"content":     toolResult,
			}
			richMessages = append(richMessages, toolMsg)
		}
	}

	finishedAt := time.Now()
	duration := finishedAt.Sub(startedAt)

	if debugEnabled && logger != nil {
		redactor := safety.NewSecretRedactor()
		meta := MetaFromContext(ctx)
		rec := BuildAuditRecord(
			requestID, cfg.Model, cfg.GuildID, messages,
			lastAssistantContent,
			lastErr, duration, retryCount, startedAt, finishedAt, meta, redactor,
		)
		args := RecordToLogArgs(rec)
		logger.Debug(ctx, "llm_audit", args...)
	}

	if lastErr != nil {
		return "", lastErr
	}

	return lastAssistantContent, nil
}

// ChatWithTools runs a tool-calling loop: sends messages with tools, executes tool calls,
// feeds results back, and loops until the model returns text or max rounds exceeded.
// Messages are kept as []map[string]string in the public signature but enriched internally
// with tool-call and tool-result turns for the request body.
func (c *Client) ChatWithTools(ctx context.Context, messages []map[string]string, cfg ChatWithToolsConfig) (string, error) {
	startedAt := time.Now()
	var requestID string
	var retryCount int

	if debugEnabled && logger != nil {
		corrID := obs.CorrelationID(ctx)
		if corrID == "" {
			ctx, corrID = obs.EnsureCorrelationID(ctx)
		}
		requestID = corrID
	}

	// Attach audit metadata
	ctx = WithMeta(ctx, &ContextMeta{
		TriggerReason: "chat_with_tools",
		GuildID:       cfg.GuildID,
	})

	// Default max rounds to 3
	maxRounds := cfg.Max
	if maxRounds == 0 {
		maxRounds = 3
	}

	// Build a richer message history for the request body that includes tool calls and results.
	// Keep the original messages slice unchanged for the public signature.
	richMessages := make([]map[string]interface{}, len(messages))
	for i, m := range messages {
		richMessages[i] = make(map[string]interface{})
		for k, v := range m {
			richMessages[i][k] = v
		}
	}

	var lastAssistantContent string
	var lastErr error

	for round := 0; round < maxRounds; round++ {
		// Build request
		msgInterfaces := make([]interface{}, len(richMessages))
		for i, m := range richMessages {
			msgInterfaces[i] = m
		}

		req := chatRequest{
			Model:       cfg.Model,
			Messages:    msgInterfaces,
			Temperature: c.config.Temperature,
			MaxTokens:   c.config.MaxTokens,
			Stream:      false,
		}

		// Add tools if provided
		if len(cfg.Tools) > 0 {
			toolInterfaces := make([]interface{}, len(cfg.Tools))
			for i, t := range cfg.Tools {
				toolInterfaces[i] = t
			}
			req.Tools = toolInterfaces
		}

		// Execute request
		resp, err := c.doRequestWithRetryTracking(ctx, req, &retryCount)
		if err != nil {
			lastErr = err
			break
		}

		if len(resp.Choices) == 0 {
			lastErr = fmt.Errorf("invalid response: no choices returned")
			break
		}

		choice := resp.Choices[0]
		lastAssistantContent = choice.Message.Content

		// If no tool calls, return the text
		if len(choice.Message.ToolCalls) == 0 {
			finishedAt := time.Now()
			duration := finishedAt.Sub(startedAt)

			if debugEnabled && logger != nil {
				redactor := safety.NewSecretRedactor()
				meta := MetaFromContext(ctx)
				rec := BuildAuditRecord(
					requestID, cfg.Model, cfg.GuildID, messages,
					lastAssistantContent,
					nil, duration, retryCount, startedAt, finishedAt, meta, redactor,
				)
				args := RecordToLogArgs(rec)
				logger.Debug(ctx, "llm_audit", args...)
			}

			return lastAssistantContent, nil
		}

		// Append assistant message with tool calls to rich history
		assistantMsg := map[string]interface{}{
			"role":       "assistant",
			"content":    choice.Message.Content,
			"tool_calls": choice.Message.ToolCalls,
		}
		richMessages = append(richMessages, assistantMsg)

		// Execute each tool call
		for _, tc := range choice.Message.ToolCalls {
			// Parse arguments
			var args map[string]interface{}
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
				args = map[string]interface{}{}
			}

			// Log tool call at DEBUG level
			if debugEnabled && logger != nil {
				argsStr := fmt.Sprintf("%v", args)
				if len(argsStr) > 100 {
					argsStr = argsStr[:100] + "..."
				}
				logger.Debug(ctx, "tool_call", "round", round+1, "tool", tc.Function.Name, "args", argsStr)
			}

			// Execute tool
			toolResult := ""
			if cfg.Exec != nil {
				result, execErr := cfg.Exec.Execute(ctx, tc.Function.Name, args)
				if execErr != nil {
					toolResult = "error: " + execErr.Error()
				} else {
					toolResult = result
				}
			}

			// Append tool result message
			toolMsg := map[string]interface{}{
				"role":        "tool",
				"tool_call_id": tc.ID,
				"content":     toolResult,
			}
			richMessages = append(richMessages, toolMsg)
		}
	}

	// Max rounds exceeded or error occurred
	finishedAt := time.Now()
	duration := finishedAt.Sub(startedAt)

	if debugEnabled && logger != nil {
		redactor := safety.NewSecretRedactor()
		meta := MetaFromContext(ctx)
		rec := BuildAuditRecord(
			requestID, cfg.Model, cfg.GuildID, messages,
			lastAssistantContent,
			lastErr, duration, retryCount, startedAt, finishedAt, meta, redactor,
		)
		args := RecordToLogArgs(rec)
		logger.Debug(ctx, "llm_audit", args...)
	}

	if lastErr != nil {
		return "", lastErr
	}

	return lastAssistantContent, nil
}

func (c *Client) ParseToolCalls(ctx context.Context, guildID int64, messages []map[string]string) ([]ToolCall, error) {
	msgInterfaces := make([]interface{}, len(messages))
	for i, m := range messages {
		msgInterfaces[i] = m
	}

	req := chatRequest{
		Model:       c.config.Model,
		Messages:    msgInterfaces,
		Temperature: c.config.Temperature,
		MaxTokens:   c.config.MaxTokens,
		Stream:      false,
	}

	resp, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("invalid response: no choices returned")
	}

	var toolCalls []ToolCall
	for _, tc := range resp.Choices[0].Message.ToolCalls {
		var args map[string]interface{}
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
			return nil, fmt.Errorf("invalid tool arguments for %s: %w", tc.Function.Name, err)
		}

		toolCalls = append(toolCalls, ToolCall{
			ID:        tc.ID,
			ToolName:  tc.Function.Name,
			Arguments: args,
		})
	}

	return toolCalls, nil
}

func (c *Client) CallTool(ctx context.Context, guildID int64, toolName string, arguments map[string]interface{}) (string, error) {
	messages := []map[string]string{
		{"role": "user", "content": fmt.Sprintf("Execute tool: %s with args: %v", toolName, arguments)},
	}

	return c.Chat(ctx, guildID, messages)
}

func (c *Client) doRequest(ctx context.Context, req chatRequest) (*chatResponse, error) {
	retryCount := 0
	return c.doRequestWithRetryTracking(ctx, req, &retryCount)
}

func (c *Client) doStreamWithRetryTracking(ctx context.Context, req chatRequest, cb StreamCallbacks, retryCount *int) (string, error) {
	var lastErr error
	var emittedFirstDelta bool

	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if attempt > 0 {
			if emittedFirstDelta {
				return "", fmt.Errorf("stream failed after emitting deltas: %w", lastErr)
			}
			*retryCount = attempt
			select {
			case <-time.After(c.config.RetryDelay):
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}

		body, err := json.Marshal(req)
		if err != nil {
			return "", fmt.Errorf("failed to marshal request: %w", err)
		}

		httpReq, err := http.NewRequestWithContext(ctx, "POST", c.config.BaseURL+"/v1/chat/completions", bytes.NewReader(body))
		if err != nil {
			return "", fmt.Errorf("failed to create request: %w", err)
		}

		httpReq.Header.Set("Authorization", "Bearer "+c.config.APIKey)
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Accept", "text/event-stream")

		httpResp, err := c.http.Do(httpReq)
		if err != nil {
			lastErr = err
			continue
		}
		defer httpResp.Body.Close()

		if httpResp.StatusCode == http.StatusTooManyRequests {
			lastErr = fmt.Errorf("rate limit exceeded (429)")
			continue
		}

		if httpResp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("unexpected status code: %d", httpResp.StatusCode)
			continue
		}

		finalText, err := c.parseSSEStream(ctx, httpResp.Body, cb, &emittedFirstDelta)
		if err != nil {
			lastErr = err
			if emittedFirstDelta {
				cb.OnError(err)
				return "", err
			}
			continue
		}

		return finalText, nil
	}

	if lastErr != nil {
		cb.OnError(lastErr)
		return "", lastErr
	}

	err := fmt.Errorf("failed after %d retries", c.config.MaxRetries)
	cb.OnError(err)
	return "", err
}

func (c *Client) parseSSEStream(ctx context.Context, body io.Reader, cb StreamCallbacks, emittedFirstDelta *bool) (string, error) {
	scanner := bufio.NewScanner(body)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 2*1024*1024)

	var accumulator strings.Builder

	lineCh := make(chan []byte, 16)
	errCh := make(chan error, 1)

	go func() {
		defer close(lineCh)
		for scanner.Scan() {
			b := make([]byte, len(scanner.Bytes()))
			copy(b, scanner.Bytes())
			select {
			case lineCh <- b:
			case <-ctx.Done():
				return
			}
		}
		if err := scanner.Err(); err != nil {
			errCh <- err
		}
	}()

	for {
		select {
		case line, ok := <-lineCh:
			if !ok {
				err := fmt.Errorf("stream ended without [DONE]")
				cb.OnError(err)
				return "", err
			}

			lineStr := string(line)

			if lineStr == "" {
				continue
			}

			if !strings.HasPrefix(lineStr, "data: ") {
				continue
			}

			data := strings.TrimPrefix(lineStr, "data: ")

			if data == "[DONE]" {
				cb.OnDone()
				return accumulator.String(), nil
			}

			var chunk map[string]interface{}
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				return "", fmt.Errorf("failed to parse SSE chunk: %w", err)
			}

			choices, ok := chunk["choices"].([]interface{})
			if !ok || len(choices) == 0 {
				continue
			}

			choice, ok := choices[0].(map[string]interface{})
			if !ok {
				continue
			}

			delta, ok := choice["delta"].(map[string]interface{})
			if !ok {
				continue
			}

			content, ok := delta["content"].(string)
			if ok && content != "" {
				*emittedFirstDelta = true
				accumulator.WriteString(content)
				cb.OnDelta(content)
			}

		case err := <-errCh:
			cb.OnError(err)
			return "", fmt.Errorf("scanner error: %w", err)

		case <-ctx.Done():
			cb.OnError(ctx.Err())
			return "", ctx.Err()
		}
	}
}

func (c *Client) doRequestWithRetryTracking(ctx context.Context, req chatRequest, retryCount *int) (*chatResponse, error) {
	var lastErr error

	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if attempt > 0 {
			*retryCount = attempt
			select {
			case <-time.After(c.config.RetryDelay):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		body, err := json.Marshal(req)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request: %w", err)
		}

		httpReq, err := http.NewRequestWithContext(ctx, "POST", c.config.BaseURL+"/v1/chat/completions", bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		httpReq.Header.Set("Authorization", "Bearer "+c.config.APIKey)
		httpReq.Header.Set("Content-Type", "application/json")

		httpResp, err := c.http.Do(httpReq)
		if err != nil {
			lastErr = err
			continue
		}
		defer httpResp.Body.Close()

		respBody, err := io.ReadAll(httpResp.Body)
		if err != nil {
			lastErr = fmt.Errorf("failed to read response: %w", err)
			continue
		}

		if httpResp.StatusCode == http.StatusTooManyRequests {
			lastErr = fmt.Errorf("rate limit exceeded (429)")
			continue
		}

		if httpResp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("unexpected status code: %d", httpResp.StatusCode)
			continue
		}

		var resp chatResponse
		if err := json.Unmarshal(respBody, &resp); err != nil {
			return nil, fmt.Errorf("invalid response: failed to parse JSON: %w", err)
		}

		if resp.Error != nil {
			return nil, fmt.Errorf("API error: %s", resp.Error.Message)
		}

		return &resp, nil
	}

	if lastErr != nil {
		return nil, lastErr
	}

	return nil, fmt.Errorf("failed after %d retries", c.config.MaxRetries)
}

func (c *Client) doStreamWithToolsRetryTracking(
	ctx context.Context,
	req chatRequest,
	retryCount *int,
	onDelta func(string),
	onToolCall func(struct {
		ID   string
		Name string
		Args string
	}),
	finishReason *string,
) error {
	var lastErr error
	var emittedFirstDelta bool

	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if attempt > 0 {
			if emittedFirstDelta {
				return fmt.Errorf("stream failed after emitting deltas: %w", lastErr)
			}
			*retryCount = attempt
			select {
			case <-time.After(c.config.RetryDelay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		body, err := json.Marshal(req)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}

		httpReq, err := http.NewRequestWithContext(ctx, "POST", c.config.BaseURL+"/v1/chat/completions", bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		httpReq.Header.Set("Authorization", "Bearer "+c.config.APIKey)
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Accept", "text/event-stream")

		httpResp, err := c.http.Do(httpReq)
		if err != nil {
			lastErr = err
			continue
		}
		defer httpResp.Body.Close()

		if httpResp.StatusCode == http.StatusTooManyRequests {
			lastErr = fmt.Errorf("rate limit exceeded (429)")
			continue
		}

		if httpResp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("unexpected status code: %d", httpResp.StatusCode)
			continue
		}

		err = c.parseSSEStreamWithTools(ctx, httpResp.Body, onDelta, onToolCall, finishReason, &emittedFirstDelta)
		if err != nil {
			lastErr = err
			if emittedFirstDelta {
				return err
			}
			continue
		}

		return nil
	}

	if lastErr != nil {
		return lastErr
	}

	return fmt.Errorf("failed after %d retries", c.config.MaxRetries)
}

func (c *Client) parseSSEStreamWithTools(
	ctx context.Context,
	body io.Reader,
	onDelta func(string),
	onToolCall func(struct {
		ID   string
		Name string
		Args string
	}),
	finishReason *string,
	emittedFirstDelta *bool,
) error {
	scanner := bufio.NewScanner(body)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 2*1024*1024)

	pendingCalls := make(map[int]*pendingToolCall)

	lineCh := make(chan []byte, 16)
	errCh := make(chan error, 1)

	go func() {
		defer close(lineCh)
		for scanner.Scan() {
			b := make([]byte, len(scanner.Bytes()))
			copy(b, scanner.Bytes())
			select {
			case lineCh <- b:
			case <-ctx.Done():
				return
			}
		}
		if err := scanner.Err(); err != nil {
			errCh <- err
		}
	}()

	for {
		select {
		case line, ok := <-lineCh:
			if !ok {
				return fmt.Errorf("stream ended without [DONE]")
			}

			lineStr := string(line)

			if lineStr == "" {
				continue
			}

			if !strings.HasPrefix(lineStr, "data: ") {
				continue
			}

			data := strings.TrimPrefix(lineStr, "data: ")

			if data == "[DONE]" {
				for _, tc := range pendingCalls {
					onToolCall(struct {
						ID   string
						Name string
						Args string
					}{
						ID:   tc.id,
						Name: tc.name,
						Args: tc.argsBuf.String(),
					})
				}
				return nil
			}

			var delta sseDelta
			if err := json.Unmarshal([]byte(data), &delta); err != nil {
				return fmt.Errorf("failed to parse SSE chunk: %w", err)
			}

			if len(delta.Choices) == 0 {
				continue
			}

			choice := delta.Choices[0]

			if choice.Delta.Content != "" {
				*emittedFirstDelta = true
				onDelta(choice.Delta.Content)
			}

			if choice.FinishReason != "" {
				*finishReason = choice.FinishReason
			}

			for _, tc := range choice.Delta.ToolCalls {
				if _, exists := pendingCalls[tc.Index]; !exists {
					pendingCalls[tc.Index] = &pendingToolCall{}
				}

				pending := pendingCalls[tc.Index]

				if tc.ID != "" {
					pending.id = tc.ID
					*emittedFirstDelta = true
				}

				if tc.Function.Name != "" {
					pending.name = tc.Function.Name
					*emittedFirstDelta = true
				}

				if tc.Function.Arguments != "" {
					pending.argsBuf.WriteString(tc.Function.Arguments)
					*emittedFirstDelta = true
				}
			}

		case err := <-errCh:
			return fmt.Errorf("scanner error: %w", err)

		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
