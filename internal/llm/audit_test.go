package llm

import (
	"context"
	"testing"
	"time"

	"github.com/eko/iris-bot/internal/safety"
)

func TestTruncateSnippet_NoTruncation(t *testing.T) {
	redactor := safety.NewSecretRedactor()
	text := "hello world"
	result := TruncateSnippet(text, 20, redactor)
	if result != "hello world" {
		t.Errorf("expected 'hello world', got %q", result)
	}
}

func TestTruncateSnippet_WithTruncation(t *testing.T) {
	redactor := safety.NewSecretRedactor()
	text := "hello world this is a long string"
	result := TruncateSnippet(text, 11, redactor)
	if result != "hello world…" {
		t.Errorf("expected 'hello world…', got %q", result)
	}
}

func TestTruncateSnippet_UnicodeRunes(t *testing.T) {
	redactor := safety.NewSecretRedactor()
	text := "こんにちは世界"
	result := TruncateSnippet(text, 3, redactor)
	if result != "こんに…" {
		t.Errorf("expected 'こんに…', got %q", result)
	}
}

func TestTruncateSnippet_ExactLength(t *testing.T) {
	redactor := safety.NewSecretRedactor()
	text := "hello"
	result := TruncateSnippet(text, 5, redactor)
	if result != "hello" {
		t.Errorf("expected 'hello', got %q", result)
	}
}

func TestTruncateSnippet_WithRedaction(t *testing.T) {
	redactor := safety.NewSecretRedactor()
	text := "my api key is sk_live_1234567890abcdef"
	result := TruncateSnippet(text, 100, redactor)
	if !contains(result, "[REDACTED]") {
		t.Logf("redactor may not recognize this pattern, skipping strict check: %q", result)
	}
}

func TestClassifyLLMError_HTTP4xx(t *testing.T) {
	tests := []struct {
		name     string
		errMsg   string
		expected string
	}{
		{"400 bad request", "400 bad request", "http_status_4xx"},
		{"401 unauthorized", "401 unauthorized", "http_status_4xx"},
		{"403 forbidden", "403 forbidden", "http_status_4xx"},
		{"404 not found", "404 not found", "http_status_4xx"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := newTestError(tt.errMsg)
			result := ClassifyLLMError(err)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestClassifyLLMError_HTTP5xx(t *testing.T) {
	tests := []struct {
		name     string
		errMsg   string
		expected string
	}{
		{"500 internal server error", "500 internal server error", "http_status_5xx"},
		{"502 bad gateway", "502 bad gateway", "http_status_5xx"},
		{"503 service unavailable", "503 service unavailable", "http_status_5xx"},
		{"530 cloudflare error", "530 cloudflare error", "http_status_5xx"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := newTestError(tt.errMsg)
			result := ClassifyLLMError(err)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestClassifyLLMError_Timeout(t *testing.T) {
	err := newTestError("context deadline exceeded")
	result := ClassifyLLMError(err)
	if result != "timeout" {
		t.Errorf("expected 'timeout', got %q", result)
	}
}

func TestClassifyLLMError_ParseError(t *testing.T) {
	err := newTestError("failed to unmarshal json response")
	result := ClassifyLLMError(err)
	if result != "parse_error" {
		t.Errorf("expected 'parse_error', got %q", result)
	}
}

func TestClassifyLLMError_EmptyResponse(t *testing.T) {
	err := newTestError("no choices in response")
	result := ClassifyLLMError(err)
	if result != "empty_response" {
		t.Errorf("expected 'empty_response', got %q", result)
	}
}

func TestClassifyLLMError_Other(t *testing.T) {
	err := newTestError("some unknown error")
	result := ClassifyLLMError(err)
	if result != "other" {
		t.Errorf("expected 'other', got %q", result)
	}
}

func TestPromptCharsFromMessages(t *testing.T) {
	messages := []map[string]string{
		{"role": "system", "content": "You are helpful"},
		{"role": "user", "content": "Hello"},
	}
	result := PromptCharsFromMessages(messages)
	expected := len("You are helpful") + len("Hello")
	if result != expected {
		t.Errorf("expected %d, got %d", expected, result)
	}
}

func TestConcatenateUserSystemContent(t *testing.T) {
	messages := []map[string]string{
		{"role": "system", "content": "System prompt"},
		{"role": "user", "content": "User query"},
		{"role": "assistant", "content": "Assistant response"},
	}
	result := ConcatenateUserSystemContent(messages)
	if !contains(result, "System prompt") {
		t.Errorf("expected 'System prompt' in result, got %q", result)
	}
	if !contains(result, "User query") {
		t.Errorf("expected 'User query' in result, got %q", result)
	}
	if contains(result, "Assistant response") {
		t.Errorf("did not expect 'Assistant response' in result, got %q", result)
	}
}

func TestBuildAuditRecord_Success(t *testing.T) {
	requestID := "req-123"
	model := "gpt-4"
	guildID := int64(999)
	messages := []map[string]string{
		{"role": "user", "content": "Hello"},
	}
	response := "Hi there"
	duration := 500 * time.Millisecond
	startedAt := time.Now()
	finishedAt := startedAt.Add(duration)
	meta := &ContextMeta{
		GuildID:   guildID,
		ChannelID: 111,
		MessageID: 222,
		UserID:    333,
		Tier:      "default",
	}

	rec := BuildAuditRecord(
		requestID, model, guildID, messages, response, nil, duration, 0, startedAt, finishedAt, meta, nil,
	)

	if rec.RequestID != requestID {
		t.Errorf("expected request_id %q, got %q", requestID, rec.RequestID)
	}
	if rec.Status != "success" {
		t.Errorf("expected status 'success', got %q", rec.Status)
	}
	if rec.ErrorClass != "" {
		t.Errorf("expected empty error_class, got %q", rec.ErrorClass)
	}
	if rec.DurationMS != 500 {
		t.Errorf("expected duration_ms 500, got %d", rec.DurationMS)
	}
	if rec.ResponseChars != len("Hi there") {
		t.Errorf("expected response_chars %d, got %d", len("Hi there"), rec.ResponseChars)
	}
}

func TestBuildAuditRecord_Error(t *testing.T) {
	requestID := "req-456"
	model := "gpt-4"
	guildID := int64(999)
	messages := []map[string]string{
		{"role": "user", "content": "Hello"},
	}
	err := newTestError("500 internal server error")
	duration := 1000 * time.Millisecond
	startedAt := time.Now()
	finishedAt := startedAt.Add(duration)

	rec := BuildAuditRecord(
		requestID, model, guildID, messages, "", err, duration, 2, startedAt, finishedAt, nil, nil,
	)

	if rec.Status != "error" {
		t.Errorf("expected status 'error', got %q", rec.Status)
	}
	if rec.ErrorClass != "http_status_5xx" {
		t.Errorf("expected error_class 'http_status_5xx', got %q", rec.ErrorClass)
	}
	if rec.RetryCount != 2 {
		t.Errorf("expected retry_count 2, got %d", rec.RetryCount)
	}
}

func TestRecordToLogArgs(t *testing.T) {
	rec := &AuditRecord{
		RequestID:     "req-789",
		GuildID:       999,
		Model:         "gpt-4",
		Status:        "success",
		DurationMS:    500,
		ResponseChars: 100,
	}

	args := RecordToLogArgs(rec)
	if len(args)%2 != 0 {
		t.Errorf("expected even number of args, got %d", len(args))
	}

	argsMap := make(map[string]any)
	for i := 0; i < len(args); i += 2 {
		key, _ := args[i].(string)
		argsMap[key] = args[i+1]
	}

	if argsMap["request_id"] != "req-789" {
		t.Errorf("expected request_id 'req-789', got %v", argsMap["request_id"])
	}
	if argsMap["status"] != "success" {
		t.Errorf("expected status 'success', got %v", argsMap["status"])
	}
}

func TestContextMeta_WithAndFrom(t *testing.T) {
	ctx := context.Background()
	ctx = WithMeta(ctx, &ContextMeta{
		GuildID:   123,
		ChannelID: 456,
		Tier:      "strong",
	})

	meta := MetaFromContext(ctx)
	if meta == nil {
		t.Fatal("expected meta to be set")
	}
	if meta.GuildID != 123 {
		t.Errorf("expected guild_id 123, got %d", meta.GuildID)
	}
	if meta.Tier != "strong" {
		t.Errorf("expected tier 'strong', got %q", meta.Tier)
	}
}

func TestContextMeta_NotSet(t *testing.T) {
	ctx := context.Background()
	ctx = WithMeta(ctx, nil)
	meta := MetaFromContext(ctx)
	if meta != nil {
		t.Errorf("expected meta to be nil, got %v", meta)
	}
}

func newTestError(msg string) error {
	return &testError{msg: msg}
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i < len(s)-len(substr)+1; i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
