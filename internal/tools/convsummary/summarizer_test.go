package convsummary

import (
	"context"
	"testing"
	"time"
)

type fakeLLM struct {
	result string
	called bool
	prompt string
}

func (f *fakeLLM) Summarize(ctx context.Context, prompt string) (string, error) {
	f.called = true
	f.prompt = prompt
	return f.result, nil
}

func TestSummarizeChannelMessages(t *testing.T) {
	history := NewInMemoryHistory()
	now := time.Now()

	history.Add(100, 200, Message{
		UserID:    1,
		Username:  "alice",
		Content:   "hello world",
		CreatedAt: now,
	})
	history.Add(100, 200, Message{
		UserID:    2,
		Username:  "bob",
		Content:   "my secret is sk-test-1234567890abcdefghij",
		CreatedAt: now.Add(1 * time.Second),
	})
	history.Add(100, 200, Message{
		UserID:    3,
		Username:  "charlie",
		Content:   "goodbye",
		CreatedAt: now.Add(2 * time.Second),
	})

	fakeLLM := &fakeLLM{result: "- Pembahasan A\n- Pembahasan B"}
	redactor := NewRedactor()

	summarizer := &Summarizer{
		History: history,
		Redact:  redactor,
		LLM:     fakeLLM,
	}

	summary, err := summarizer.Summarize(context.Background(), 100, 200, 10)
	if err != nil {
		t.Fatalf("Summarize failed: %v", err)
	}

	if summary.Empty {
		t.Errorf("expected non-empty summary")
	}

	if summary.Count != 3 {
		t.Errorf("expected Count=3, got %d", summary.Count)
	}

	if summary.GuildID != 100 {
		t.Errorf("expected GuildID=100, got %d", summary.GuildID)
	}

	if summary.ChannelID != 200 {
		t.Errorf("expected ChannelID=200, got %d", summary.ChannelID)
	}

	if summary.Text != "- Pembahasan A\n- Pembahasan B" {
		t.Errorf("expected LLM result, got: %s", summary.Text)
	}

	if !fakeLLM.called {
		t.Errorf("LLM.Summarize should have been called")
	}

	if fakeLLM.prompt == "" {
		t.Errorf("LLM prompt should not be empty")
	}

	if fakeLLM.prompt == "" {
		t.Errorf("LLM prompt should not be empty")
	}

	if contains(fakeLLM.prompt, "sk-test-1234567890abcdefghij") {
		t.Errorf("LLM prompt should not contain unredacted secret key")
	}

	if !contains(fakeLLM.prompt, "[REDACTED_TOKEN]") {
		t.Errorf("LLM prompt should contain redacted token marker")
	}
}

func TestSummarizeEmptyReturnsIndonesianMessage(t *testing.T) {
	history := NewInMemoryHistory()
	fakeLLM := &fakeLLM{result: "should not be called"}
	redactor := NewRedactor()

	summarizer := &Summarizer{
		History: history,
		Redact:  redactor,
		LLM:     fakeLLM,
	}

	summary, err := summarizer.Summarize(context.Background(), 100, 200, 10)
	if err != nil {
		t.Fatalf("Summarize failed: %v", err)
	}

	if !summary.Empty {
		t.Errorf("expected Empty=true")
	}

	if summary.Text != "Belum ada riwayat pesan yang dapat diringkas untuk channel ini." {
		t.Errorf("expected Indonesian empty message, got: %s", summary.Text)
	}

	if fakeLLM.called {
		t.Errorf("LLM.Summarize should not have been called for empty history")
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && s != "" && substr != "" && (s == substr || len(s) >= len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
