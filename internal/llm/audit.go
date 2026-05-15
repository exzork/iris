package llm

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/eko/iris-bot/internal/safety"
)

type contextMetaKey struct{}

// ContextMeta holds optional metadata for LLM audit logging.
// Fields are attached to context via WithMeta and read via MetaFromContext.
type ContextMeta struct {
	GuildID       int64
	ChannelID     int64
	MessageID     int64
	UserID        int64
	Tier          string
	TriggerReason string
}

// WithMeta returns ctx with ContextMeta attached.
func WithMeta(ctx context.Context, meta *ContextMeta) context.Context {
	if meta == nil {
		return ctx
	}
	return context.WithValue(ctx, contextMetaKey{}, meta)
}

// MergeMeta layers patch on top of any existing ContextMeta in ctx so that
// callers (e.g. the LLM client annotating TriggerReason) do not clobber
// upstream-provided ChannelID/UserID/MessageID. Empty fields in patch leave
// the existing values intact.
func MergeMeta(ctx context.Context, patch *ContextMeta) context.Context {
	if patch == nil {
		return ctx
	}
	merged := &ContextMeta{}
	if existing := MetaFromContext(ctx); existing != nil {
		*merged = *existing
	}
	if patch.GuildID != 0 {
		merged.GuildID = patch.GuildID
	}
	if patch.ChannelID != 0 {
		merged.ChannelID = patch.ChannelID
	}
	if patch.MessageID != 0 {
		merged.MessageID = patch.MessageID
	}
	if patch.UserID != 0 {
		merged.UserID = patch.UserID
	}
	if patch.Tier != "" {
		merged.Tier = patch.Tier
	}
	if patch.TriggerReason != "" {
		merged.TriggerReason = patch.TriggerReason
	}
	return context.WithValue(ctx, contextMetaKey{}, merged)
}

// MetaFromContext retrieves ContextMeta from ctx, or nil if not set.
func MetaFromContext(ctx context.Context) *ContextMeta {
	v, _ := ctx.Value(contextMetaKey{}).(*ContextMeta)
	return v
}

// AuditRecord holds all fields for a single LLM request audit log.
type AuditRecord struct {
	RequestID       string
	GuildID         int64
	ChannelID       int64
	MessageID       int64
	UserID          int64
	Model           string
	Tier            string
	TriggerReason   string
	MessageCount    int
	PromptChars     int
	PromptSnippet   string
	ResponseChars   int
	ResponseSnippet string
	Status          string
	ErrorClass      string
	ErrorMessage    string
	DurationMS      int64
	RetryCount      int
	StartedAt       string
	FinishedAt      string
}

// TruncateSnippet returns the first maxRunes Unicode characters of text,
// with "…" suffix if truncated. Applies redaction first.
func TruncateSnippet(text string, maxRunes int, redactor *safety.SecretRedactor) string {
	if redactor == nil {
		redactor = safety.NewSecretRedactor()
	}
	redacted := redactor.Redact(text)

	if utf8.RuneCountInString(redacted) <= maxRunes {
		return redacted
	}

	runes := []rune(redacted)
	return string(runes[:maxRunes]) + "…"
}

// ClassifyLLMError maps an error to an audit error class.
func ClassifyLLMError(err error) string {
	if err == nil {
		return ""
	}

	errStr := strings.ToLower(err.Error())

	if strings.Contains(errStr, "400") || strings.Contains(errStr, "bad request") {
		return "http_status_4xx"
	}
	if strings.Contains(errStr, "401") || strings.Contains(errStr, "403") || strings.Contains(errStr, "404") {
		return "http_status_4xx"
	}
	if strings.Contains(errStr, "500") || strings.Contains(errStr, "502") || strings.Contains(errStr, "503") || strings.Contains(errStr, "530") {
		return "http_status_5xx"
	}
	if strings.Contains(errStr, "timeout") || strings.Contains(errStr, "context deadline") {
		return "timeout"
	}
	if strings.Contains(errStr, "parse") || strings.Contains(errStr, "unmarshal") || strings.Contains(errStr, "json") {
		return "parse_error"
	}
	if strings.Contains(errStr, "empty") || strings.Contains(errStr, "no choices") {
		return "empty_response"
	}

	return "other"
}

// PromptCharsFromMessages sums the character count of all message content.
func PromptCharsFromMessages(messages []map[string]string) int {
	total := 0
	for _, msg := range messages {
		if content, ok := msg["content"]; ok {
			total += len(content)
		}
	}
	return total
}

// ConcatenateUserSystemContent returns concatenated user+system message content for snippet extraction.
func ConcatenateUserSystemContent(messages []map[string]string) string {
	var parts []string
	for _, msg := range messages {
		role, ok := msg["role"]
		if !ok {
			continue
		}
		content, ok := msg["content"]
		if !ok {
			continue
		}
		if role == "user" || role == "system" {
			parts = append(parts, content)
		}
	}
	return strings.Join(parts, "\n")
}

// FormatTimestamp returns RFC3339 formatted timestamp.
func FormatTimestamp(t time.Time) string {
	return t.Format(time.RFC3339)
}

// BuildAuditRecord constructs an AuditRecord from call parameters and results.
func BuildAuditRecord(
	requestID string,
	model string,
	guildID int64,
	messages []map[string]string,
	response string,
	err error,
	duration time.Duration,
	retryCount int,
	startedAt, finishedAt time.Time,
	meta *ContextMeta,
	redactor *safety.SecretRedactor,
) *AuditRecord {
	if redactor == nil {
		redactor = safety.NewSecretRedactor()
	}

	if meta == nil {
		meta = &ContextMeta{}
	}

	status := "success"
	errorClass := ""
	errorMessage := ""
	if err != nil {
		status = "error"
		errorClass = ClassifyLLMError(err)
		errorMessage = err.Error()
		if len(errorMessage) > 200 {
			errorMessage = errorMessage[:200]
		}
	}

	promptChars := PromptCharsFromMessages(messages)
	promptContent := ConcatenateUserSystemContent(messages)
	promptSnippet := TruncateSnippet(promptContent, 512, redactor)

	responseChars := len(response)
	responseSnippet := TruncateSnippet(response, 512, redactor)

	return &AuditRecord{
		RequestID:       requestID,
		GuildID:         meta.GuildID,
		ChannelID:       meta.ChannelID,
		MessageID:       meta.MessageID,
		UserID:          meta.UserID,
		Model:           model,
		Tier:            meta.Tier,
		TriggerReason:   meta.TriggerReason,
		MessageCount:    len(messages),
		PromptChars:     promptChars,
		PromptSnippet:   promptSnippet,
		ResponseChars:   responseChars,
		ResponseSnippet: responseSnippet,
		Status:          status,
		ErrorClass:      errorClass,
		ErrorMessage:    errorMessage,
		DurationMS:      duration.Milliseconds(),
		RetryCount:      retryCount,
		StartedAt:       FormatTimestamp(startedAt),
		FinishedAt:      FormatTimestamp(finishedAt),
	}
}

// RecordToLogArgs converts an AuditRecord to slog-compatible key-value pairs.
func RecordToLogArgs(rec *AuditRecord) []any {
	args := []any{
		"request_id", rec.RequestID,
		"guild_id", rec.GuildID,
		"channel_id", rec.ChannelID,
		"message_id", rec.MessageID,
		"user_id", rec.UserID,
		"model", rec.Model,
		"tier", rec.Tier,
		"trigger_reason", rec.TriggerReason,
		"message_count", rec.MessageCount,
		"prompt_chars", rec.PromptChars,
		"prompt_snippet", rec.PromptSnippet,
		"response_chars", rec.ResponseChars,
		"response_snippet", rec.ResponseSnippet,
		"status", rec.Status,
		"error_class", rec.ErrorClass,
		"error_message", rec.ErrorMessage,
		"duration_ms", rec.DurationMS,
		"retry_count", rec.RetryCount,
		"started_at", rec.StartedAt,
		"finished_at", rec.FinishedAt,
	}
	return args
}
