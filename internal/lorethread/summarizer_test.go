package lorethread

import (
	"context"
	"testing"
	"time"
)

type fakeRedactor struct {
	redactedText string
}

func (f *fakeRedactor) Redact(text string) string {
	return f.redactedText
}

type capturingLLMCaller struct {
	response string
	onCall   func(systemPrompt, userPrompt string)
}

func (c *capturingLLMCaller) Call(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	if c.onCall != nil {
		c.onCall(systemPrompt, userPrompt)
	}
	return c.response, nil
}

func TestSummarizer_NormalSummary(t *testing.T) {
	fake := &fakeLLMCaller{
		response: "Ini adalah ringkasan lore yang bagus.",
	}
	redactor := &fakeRedactor{
		redactedText: "Ini adalah ringkasan lore yang bagus.",
	}
	summarizer := NewLLMSummarizer(fake, redactor, 5*time.Second)

	req := &SummaryRequest{
		GuildID: 123,
		Messages: []*Message{
			{
				ID:        1,
				AuthorID:  456,
				Content:   "First message about lore",
				CreatedAt: time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC),
			},
			{
				ID:        2,
				AuthorID:  789,
				Content:   "Second message about lore",
				CreatedAt: time.Date(2026, 5, 13, 10, 5, 0, 0, time.UTC),
			},
		},
	}

	result, err := summarizer.Summarize(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Summary != "Ini adalah ringkasan lore yang bagus." {
		t.Errorf("expected summary 'Ini adalah ringkasan lore yang bagus.', got %q", result.Summary)
	}
}

func TestSummarizer_RedactionOfToken(t *testing.T) {
	fake := &fakeLLMCaller{
		response: "Token sk-1234567890123456789012 was used",
	}
	redactor := NewDefaultRedactor()
	summarizer := NewLLMSummarizer(fake, redactor, 5*time.Second)

	req := &SummaryRequest{
		GuildID: 123,
		Messages: []*Message{
			{
				ID:        1,
				AuthorID:  456,
				Content:   "Some lore",
				CreatedAt: time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC),
			},
		},
	}

	result, err := summarizer.Summarize(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !contains(result.Summary, "[REDACTED]") {
		t.Errorf("expected token to be redacted, got %q", result.Summary)
	}
}

func TestSummarizer_RedactionOfEmail(t *testing.T) {
	fake := &fakeLLMCaller{
		response: "Contact user@example.com for more info",
	}
	redactor := NewDefaultRedactor()
	summarizer := NewLLMSummarizer(fake, redactor, 5*time.Second)

	req := &SummaryRequest{
		GuildID: 123,
		Messages: []*Message{
			{
				ID:        1,
				AuthorID:  456,
				Content:   "Some lore",
				CreatedAt: time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC),
			},
		},
	}

	result, err := summarizer.Summarize(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !contains(result.Summary, "[REDACTED]") {
		t.Errorf("expected email to be redacted, got %q", result.Summary)
	}
}

func TestSummarizer_BahasaIndonesiaInstructionPresent(t *testing.T) {
	var capturedSystemPrompt string

	capturingCaller := &capturingLLMCaller{
		response: "Ringkasan lore",
		onCall: func(systemPrompt, userPrompt string) {
			capturedSystemPrompt = systemPrompt
		},
	}

	redactor := NewDefaultRedactor()
	summarizer := NewLLMSummarizer(capturingCaller, redactor, 5*time.Second)

	req := &SummaryRequest{
		GuildID: 123,
		Messages: []*Message{
			{
				ID:        1,
				AuthorID:  456,
				Content:   "Some lore",
				CreatedAt: time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC),
			},
		},
	}

	_, err := summarizer.Summarize(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !contains(capturedSystemPrompt, "Bahasa Indonesia") {
		t.Errorf("expected 'Bahasa Indonesia' in system prompt, got %q", capturedSystemPrompt)
	}
}

func TestSummarizer_MessageDelimiterPresent(t *testing.T) {
	var capturedUserPrompt string

	capturingCaller := &capturingLLMCaller{
		response: "Ringkasan lore",
		onCall: func(systemPrompt, userPrompt string) {
			capturedUserPrompt = userPrompt
		},
	}

	redactor := NewDefaultRedactor()
	summarizer := NewLLMSummarizer(capturingCaller, redactor, 5*time.Second)

	req := &SummaryRequest{
		GuildID: 123,
		Messages: []*Message{
			{
				ID:        1,
				AuthorID:  456,
				Content:   "Some lore",
				CreatedAt: time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC),
			},
		},
	}

	_, err := summarizer.Summarize(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !contains(capturedUserPrompt, "<msg") {
		t.Errorf("expected '<msg' delimiter in user prompt, got %q", capturedUserPrompt)
	}
}

func TestSummarizer_EmptyMessageList(t *testing.T) {
	fake := &fakeLLMCaller{
		response: "Should not be called",
	}
	redactor := NewDefaultRedactor()
	summarizer := NewLLMSummarizer(fake, redactor, 5*time.Second)

	req := &SummaryRequest{
		GuildID:  123,
		Messages: []*Message{},
	}

	_, err := summarizer.Summarize(context.Background(), req)
	if err == nil {
		t.Fatalf("expected error for empty message list, got nil")
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
