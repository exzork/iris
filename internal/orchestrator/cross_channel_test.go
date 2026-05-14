package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/eko/iris-bot/internal/domain"
	"github.com/eko/iris-bot/internal/llm"
)

type fakeCandidateStore struct {
	messages []*domain.ChannelMessage
	err      error

	mu          sync.Mutex
	callCount   int
	lastGuildID int64
	lastUserID  int64
	lastSince   int
	lastLimit   int
}

func (f *fakeCandidateStore) ListByUserAcrossChannels(ctx context.Context, guildID int64, userID int64, sinceMinutes int, limit int) ([]*domain.ChannelMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.callCount++
	f.lastGuildID = guildID
	f.lastUserID = userID
	f.lastSince = sinceMinutes
	f.lastLimit = limit
	if f.err != nil {
		return nil, f.err
	}
	return f.messages, nil
}

type fakeAllowQuerier struct {
	hasAny  bool
	allowed map[int64]bool
}

func (f *fakeAllowQuerier) HasAny(ctx context.Context, guildID int64) (bool, error) {
	return f.hasAny, nil
}

func (f *fakeAllowQuerier) IsAllowed(ctx context.Context, guildID int64, channelID int64) (bool, error) {
	if f.allowed == nil {
		return false, nil
	}
	return f.allowed[channelID], nil
}

type fakeCrossChannelLLM struct {
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

func (f *fakeCrossChannelLLM) ChatWithModel(ctx context.Context, model string, guildID int64, messages []map[string]string) (string, error) {
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

func makeCandidate(guildID, channelID, messageID, userID int64, isBot bool, createdAt time.Time) *domain.ChannelMessage {
	author := "user"
	return &domain.ChannelMessage{
		GuildID:    guildID,
		ChannelID:  channelID,
		MessageID:  messageID,
		UserID:     userID,
		AuthorName: &author,
		Content:    "candidate",
		IsBot:      isBot,
		CreatedAt:  createdAt,
	}
}

func TestCrossChannel_MergeTrue_PrependsBoundedCandidates(t *testing.T) {
	now := time.Now()
	var messages []*domain.ChannelMessage
	for i := range 12 {
		messages = append(messages, makeCandidate(1, 200+int64(i%2), int64(i+1), 42, false, now.Add(-time.Duration(i)*time.Minute)))
	}

	store := &fakeCandidateStore{messages: messages}
	model := "kr/claude-haiku-4.5"
	fakeLLM := &fakeCrossChannelLLM{response: `{"merge": true, "reason": "related"}`}

	classifier := NewLLMCrossChannelClassifier(LLMCrossChannelConfig{
		Store:        store,
		LLM:          fakeLLM,
		Model:        model,
		Timeout:      100 * time.Millisecond,
		MaxCandidates: 10,
		WindowMinutes: 30,
	})

	event := &domain.DiscordEvent{
		GuildID:   1,
		ChannelID: 100,
		UserID:    42,
		Message:   &domain.DiscordMessage{ID: 777, Content: "reply"},
	}

	selected, err := classifier.Classify(context.Background(), event)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(selected) != 10 {
		t.Fatalf("expected 10 candidates, got %d", len(selected))
	}

	for i := 1; i < len(selected); i++ {
		if selected[i-1].CreatedAt.After(selected[i].CreatedAt) {
			t.Fatalf("expected oldest->newest ordering")
		}
	}

	if store.lastSince != 30 || store.lastLimit != 20 {
		t.Fatalf("expected query window=30 limit=20, got window=%d limit=%d", store.lastSince, store.lastLimit)
	}

	fakeLLM.mu.Lock()
	defer fakeLLM.mu.Unlock()
	if fakeLLM.called != 1 {
		t.Fatalf("expected one llm call, got %d", fakeLLM.called)
	}
	if fakeLLM.model != model {
		t.Fatalf("expected model %q, got %q", model, fakeLLM.model)
	}
	if fakeLLM.meta == nil || fakeLLM.meta.TriggerReason != "cross_channel_classify" {
		t.Fatalf("expected trigger_reason cross_channel_classify, got %#v", fakeLLM.meta)
	}
}

func TestCrossChannel_ParseError_Fallback(t *testing.T) {
	store := &fakeCandidateStore{messages: []*domain.ChannelMessage{makeCandidate(1, 200, 1, 42, false, time.Now())}}
	classifier := NewLLMCrossChannelClassifier(LLMCrossChannelConfig{
		Store: store,
		LLM:   &fakeCrossChannelLLM{response: "not-json"},
		Model: "kr/claude-haiku-4.5",
	})

	selected, err := classifier.Classify(context.Background(), &domain.DiscordEvent{
		GuildID:   1,
		ChannelID: 100,
		UserID:    42,
		Message:   &domain.DiscordMessage{ID: 9, Content: "hello"},
	})
	if err != nil {
		t.Fatalf("expected nil error fallback, got %v", err)
	}
	if len(selected) != 0 {
		t.Fatalf("expected no merge on parse failure")
	}
}

func TestCrossChannel_Timeout_Fallback(t *testing.T) {
	store := &fakeCandidateStore{messages: []*domain.ChannelMessage{makeCandidate(1, 200, 1, 42, false, time.Now())}}
	classifier := NewLLMCrossChannelClassifier(LLMCrossChannelConfig{
		Store:   store,
		LLM:     &fakeCrossChannelLLM{response: `{"merge": true, "reason": "x"}`, delay: 150 * time.Millisecond},
		Model:   "kr/claude-haiku-4.5",
		Timeout: 20 * time.Millisecond,
	})

	selected, err := classifier.Classify(context.Background(), &domain.DiscordEvent{
		GuildID:   1,
		ChannelID: 100,
		UserID:    42,
		Message:   &domain.DiscordMessage{ID: 9, Content: "hello"},
	})
	if err != nil {
		t.Fatalf("expected nil error fallback, got %v", err)
	}
	if len(selected) != 0 {
		t.Fatalf("expected no merge on timeout")
	}
}

func TestCrossChannel_FiltersCurrentChannel(t *testing.T) {
	store := &fakeCandidateStore{messages: []*domain.ChannelMessage{
		makeCandidate(1, 100, 1, 42, false, time.Now().Add(-2*time.Minute)),
		makeCandidate(1, 200, 2, 42, false, time.Now().Add(-1*time.Minute)),
	}}
	classifier := NewLLMCrossChannelClassifier(LLMCrossChannelConfig{
		Store: store,
		LLM:   &fakeCrossChannelLLM{response: `{"merge": true, "reason": "x"}`},
		Model: "kr/claude-haiku-4.5",
	})

	selected, err := classifier.Classify(context.Background(), &domain.DiscordEvent{
		GuildID:   1,
		ChannelID: 100,
		UserID:    42,
		Message:   &domain.DiscordMessage{ID: 9, Content: "hello"},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(selected) != 1 {
		t.Fatalf("expected exactly one candidate after filtering current channel, got %d", len(selected))
	}
	if selected[0].ChannelID == 100 {
		t.Fatalf("current channel candidate must be filtered")
	}
}

func TestCrossChannel_FiltersBotMessages(t *testing.T) {
	store := &fakeCandidateStore{messages: []*domain.ChannelMessage{
		makeCandidate(1, 200, 1, 42, true, time.Now().Add(-2*time.Minute)),
		makeCandidate(1, 201, 2, 42, false, time.Now().Add(-1*time.Minute)),
	}}
	classifier := NewLLMCrossChannelClassifier(LLMCrossChannelConfig{
		Store: store,
		LLM:   &fakeCrossChannelLLM{response: `{"merge": true, "reason": "x"}`},
		Model: "kr/claude-haiku-4.5",
	})

	selected, err := classifier.Classify(context.Background(), &domain.DiscordEvent{
		GuildID:   1,
		ChannelID: 100,
		UserID:    42,
		Message:   &domain.DiscordMessage{ID: 9, Content: "hello"},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(selected) != 1 {
		t.Fatalf("expected one non-bot candidate, got %d", len(selected))
	}
	if selected[0].IsBot {
		t.Fatalf("bot candidates must be filtered")
	}
}

func TestCrossChannel_AllowList_FiltersUnallowed(t *testing.T) {
	store := &fakeCandidateStore{messages: []*domain.ChannelMessage{
		makeCandidate(1, 200, 1, 42, false, time.Now().Add(-2*time.Minute)),
		makeCandidate(1, 201, 2, 42, false, time.Now().Add(-1*time.Minute)),
	}}
	allow := &fakeAllowQuerier{hasAny: true, allowed: map[int64]bool{201: true}}
	classifier := NewLLMCrossChannelClassifier(LLMCrossChannelConfig{
		Store:   store,
		Allowed: allow,
		LLM:     &fakeCrossChannelLLM{response: `{"merge": true, "reason": "x"}`},
		Model:   "kr/claude-haiku-4.5",
	})

	selected, err := classifier.Classify(context.Background(), &domain.DiscordEvent{
		GuildID:   1,
		ChannelID: 100,
		UserID:    42,
		Message:   &domain.DiscordMessage{ID: 9, Content: "hello"},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(selected) != 1 {
		t.Fatalf("expected one allowed candidate, got %d", len(selected))
	}
	if selected[0].ChannelID != 201 {
		t.Fatalf("expected only allowed channel 201, got %d", selected[0].ChannelID)
	}
}

func TestCrossChannel_NoCandidates_ReturnsNilNoError(t *testing.T) {
	store := &fakeCandidateStore{messages: []*domain.ChannelMessage{}}
	fakeLLM := &fakeCrossChannelLLM{response: `{"merge": true, "reason": "x"}`}
	classifier := NewLLMCrossChannelClassifier(LLMCrossChannelConfig{
		Store: store,
		LLM:   fakeLLM,
		Model: "kr/claude-haiku-4.5",
	})

	selected, err := classifier.Classify(context.Background(), &domain.DiscordEvent{
		GuildID:   1,
		ChannelID: 100,
		UserID:    42,
		Message:   &domain.DiscordMessage{ID: 9, Content: "hello"},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if selected != nil {
		t.Fatalf("expected nil slice when no candidates")
	}

	fakeLLM.mu.Lock()
	defer fakeLLM.mu.Unlock()
	if fakeLLM.called != 0 {
		t.Fatalf("llm should not be called when no candidates")
	}
}

func TestCrossChannel_LLMError_Fallback(t *testing.T) {
	store := &fakeCandidateStore{messages: []*domain.ChannelMessage{makeCandidate(1, 200, 1, 42, false, time.Now())}}
	classifier := NewLLMCrossChannelClassifier(LLMCrossChannelConfig{
		Store: store,
		LLM:   &fakeCrossChannelLLM{err: errors.New("boom")},
		Model: "kr/claude-haiku-4.5",
	})

	selected, err := classifier.Classify(context.Background(), &domain.DiscordEvent{
		GuildID:   1,
		ChannelID: 100,
		UserID:    42,
		Message:   &domain.DiscordMessage{ID: 9, Content: "hello"},
	})
	if err != nil {
		t.Fatalf("expected nil error fallback, got %v", err)
	}
	if len(selected) != 0 {
		t.Fatalf("expected no merge on llm error")
	}
}
