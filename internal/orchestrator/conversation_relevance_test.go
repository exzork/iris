package orchestrator

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/eko/iris-bot/internal/domain"
	"github.com/eko/iris-bot/internal/llm"
)

type fakeInWindowLLM struct {
	response string
	err      error
	delay    time.Duration

	mu       sync.Mutex
	called   int
	model    string
	guildID  int64
	meta     *llm.ContextMeta
	messages []map[string]string
}

func (f *fakeInWindowLLM) ChatWithModel(ctx context.Context, model string, guildID int64, messages []map[string]string) (string, error) {
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}

	f.mu.Lock()
	f.called++
	f.model = model
	f.guildID = guildID
	f.meta = llm.MetaFromContext(ctx)
	f.messages = append([]map[string]string(nil), messages...)
	f.mu.Unlock()

	if f.err != nil {
		return "", f.err
	}
	return f.response, nil
}

func (f *fakeInWindowLLM) CallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.called
}

func (f *fakeInWindowLLM) GetMeta() *llm.ContextMeta {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.meta
}

func TestInWindowRelevance_TrueInContext(t *testing.T) {
	fakeLLM := &fakeInWindowLLM{
		response: `{"in_context": true, "reason": "message continues the conversation"}`,
	}

	cfg := LLMInWindowRelevanceConfig{
		LLM:     fakeLLM,
		Model:   "fast-model",
		Timeout: 4 * time.Second,
	}
	classifier := NewLLMInWindowRelevance(cfg)

	event := &domain.DiscordEvent{
		GuildID: 1,
		ChannelID: 100,
		UserID: 42,
		Message: &domain.DiscordMessage{
			ID:      999,
			Content: "what about that?",
		},
	}

	contextMessages := []*domain.ChannelMessage{
		{
			GuildID:    1,
			ChannelID:  100,
			MessageID:  998,
			UserID:     1,
			AuthorName: ptrString("iris"),
			Content:    "here is some info",
			IsBot:      true,
			CreatedAt:  time.Now().Add(-1 * time.Minute),
		},
	}

	relevant, _, _, err := classifier.IsRelevant(context.Background(), event, contextMessages)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !relevant {
		t.Errorf("expected relevant=true, got false")
	}
}

func TestInWindowRelevance_FalseExplicit(t *testing.T) {
	fakeLLM := &fakeInWindowLLM{
		response: `{"in_context": false, "reason": "off-topic message"}`,
	}

	cfg := LLMInWindowRelevanceConfig{
		LLM:     fakeLLM,
		Model:   "fast-model",
		Timeout: 4 * time.Second,
	}
	classifier := NewLLMInWindowRelevance(cfg)

	event := &domain.DiscordEvent{
		GuildID: 1,
		ChannelID: 100,
		UserID: 42,
		Message: &domain.DiscordMessage{
			ID:      999,
			Content: "random off-topic",
		},
	}

	contextMessages := []*domain.ChannelMessage{
		{
			GuildID:    1,
			ChannelID:  100,
			MessageID:  998,
			UserID:     1,
			AuthorName: ptrString("iris"),
			Content:    "here is some info",
			IsBot:      true,
			CreatedAt:  time.Now().Add(-1 * time.Minute),
		},
	}

	relevant, _, _, err := classifier.IsRelevant(context.Background(), event, contextMessages)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if relevant {
		t.Errorf("expected relevant=false, got true")
	}
}

func TestInWindowRelevance_ParseError_ReturnsFalseErr(t *testing.T) {
	fakeLLM := &fakeInWindowLLM{
		response: `not valid json at all`,
	}

	cfg := LLMInWindowRelevanceConfig{
		LLM:     fakeLLM,
		Model:   "fast-model",
		Timeout: 4 * time.Second,
	}
	classifier := NewLLMInWindowRelevance(cfg)

	event := &domain.DiscordEvent{
		GuildID: 1,
		ChannelID: 100,
		UserID: 42,
		Message: &domain.DiscordMessage{
			ID:      999,
			Content: "test",
		},
	}

	contextMessages := []*domain.ChannelMessage{
		{
			GuildID:    1,
			ChannelID:  100,
			MessageID:  998,
			UserID:     1,
			AuthorName: ptrString("iris"),
			Content:    "info",
			IsBot:      true,
			CreatedAt:  time.Now(),
		},
	}

	relevant, _, _, err := classifier.IsRelevant(context.Background(), event, contextMessages)
	if err == nil {
		t.Fatalf("expected parse error, got nil")
	}
	if relevant {
		t.Errorf("expected relevant=false on parse error, got true")
	}
}

func TestInWindowRelevance_Timeout_ReturnsFalseErr(t *testing.T) {
	fakeLLM := &fakeInWindowLLM{
		delay: 100 * time.Millisecond,
	}

	cfg := LLMInWindowRelevanceConfig{
		LLM:     fakeLLM,
		Model:   "fast-model",
		Timeout: 10 * time.Millisecond,
	}
	classifier := NewLLMInWindowRelevance(cfg)

	event := &domain.DiscordEvent{
		GuildID: 1,
		ChannelID: 100,
		UserID: 42,
		Message: &domain.DiscordMessage{
			ID:      999,
			Content: "test",
		},
	}

	contextMessages := []*domain.ChannelMessage{
		{
			GuildID:    1,
			ChannelID:  100,
			MessageID:  998,
			UserID:     1,
			AuthorName: ptrString("iris"),
			Content:    "info",
			IsBot:      true,
			CreatedAt:  time.Now(),
		},
	}

	relevant, _, _, err := classifier.IsRelevant(context.Background(), event, contextMessages)
	if err == nil {
		t.Fatalf("expected timeout error, got nil")
	}
	if relevant {
		t.Errorf("expected relevant=false on timeout, got true")
	}
}

func TestInWindowRelevance_EmptyContext_SkipsLLM(t *testing.T) {
	fakeLLM := &fakeInWindowLLM{
		response: `{"in_context": true, "reason": "should not be called"}`,
	}

	cfg := LLMInWindowRelevanceConfig{
		LLM:     fakeLLM,
		Model:   "fast-model",
		Timeout: 4 * time.Second,
	}
	classifier := NewLLMInWindowRelevance(cfg)

	event := &domain.DiscordEvent{
		GuildID: 1,
		ChannelID: 100,
		UserID: 42,
		Message: &domain.DiscordMessage{
			ID:      999,
			Content: "test",
		},
	}

	relevant, _, _, err := classifier.IsRelevant(context.Background(), event, []*domain.ChannelMessage{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if relevant {
		t.Errorf("expected relevant=false for empty context, got true")
	}
	if fakeLLM.CallCount() != 0 {
		t.Errorf("expected LLM not to be called, but was called %d times", fakeLLM.CallCount())
	}
}

func TestInWindowRelevance_AttachesTriggerReason(t *testing.T) {
	fakeLLM := &fakeInWindowLLM{
		response: `{"in_context": true, "reason": "test"}`,
	}

	cfg := LLMInWindowRelevanceConfig{
		LLM:     fakeLLM,
		Model:   "fast-model",
		Timeout: 4 * time.Second,
	}
	classifier := NewLLMInWindowRelevance(cfg)

	event := &domain.DiscordEvent{
		GuildID: 1,
		ChannelID: 100,
		UserID: 42,
		Message: &domain.DiscordMessage{
			ID:      999,
			Content: "test",
		},
	}

	contextMessages := []*domain.ChannelMessage{
		{
			GuildID:    1,
			ChannelID:  100,
			MessageID:  998,
			UserID:     1,
			AuthorName: ptrString("iris"),
			Content:    "info",
			IsBot:      true,
			CreatedAt:  time.Now(),
		},
	}

	_, _, _, err := classifier.IsRelevant(context.Background(), event, contextMessages)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	meta := fakeLLM.GetMeta()
	if meta == nil {
		t.Fatalf("expected ContextMeta to be attached, got nil")
	}
	if meta.TriggerReason != "conversation_lock_relevance" {
		t.Errorf("expected TriggerReason='conversation_lock_relevance', got '%s'", meta.TriggerReason)
	}
	if meta.GuildID != 1 {
		t.Errorf("expected GuildID=1, got %d", meta.GuildID)
	}
	if meta.ChannelID != 100 {
		t.Errorf("expected ChannelID=100, got %d", meta.ChannelID)
	}
	if meta.MessageID != 999 {
		t.Errorf("expected MessageID=999, got %d", meta.MessageID)
	}
}

func ptrString(s string) *string {
	return &s
}
