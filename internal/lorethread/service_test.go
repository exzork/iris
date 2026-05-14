package lorethread

import (
	"context"
	"testing"
	"time"
)

// mockSessionStore is a test double for SessionStore.
type mockSessionStore struct {
	sessions map[int64]*Session
}

func (m *mockSessionStore) Create(ctx context.Context, session *Session) error {
	m.sessions[session.ID] = session
	return nil
}

func (m *mockSessionStore) GetByID(ctx context.Context, id int64) (*Session, error) {
	return m.sessions[id], nil
}

func (m *mockSessionStore) GetActive(ctx context.Context, guildID, channelID int64) (*Session, error) {
	return nil, nil
}

func (m *mockSessionStore) Update(ctx context.Context, session *Session) error {
	m.sessions[session.ID] = session
	return nil
}

func (m *mockSessionStore) ListByGuild(ctx context.Context, guildID int64) ([]*Session, error) {
	return nil, nil
}

// mockThreadAnchorStore is a test double for ThreadAnchorStore.
type mockThreadAnchorStore struct{}

func (m *mockThreadAnchorStore) Create(ctx context.Context, sessionID, threadID, messageID int64) error {
	return nil
}

func (m *mockThreadAnchorStore) GetBySessionID(ctx context.Context, sessionID int64) (int64, int64, error) {
	return 0, 0, nil
}

func (m *mockThreadAnchorStore) GetByThreadID(ctx context.Context, threadID int64) (int64, error) {
	return 0, nil
}

// mockGuildSettingsStore is a test double for GuildSettingsStore.
type mockGuildSettingsStore struct{}

func (m *mockGuildSettingsStore) GetLoreThreadEnabled(ctx context.Context, guildID int64) (bool, error) {
	return true, nil
}

func (m *mockGuildSettingsStore) SetLoreThreadEnabled(ctx context.Context, guildID int64, enabled bool) error {
	return nil
}

// mockLoreClassifier is a test double for LoreClassifier.
type mockLoreClassifier struct{}

func (m *mockLoreClassifier) Classify(ctx context.Context, guildID int64, message *Message) (*ClassifyResult, error) {
	return &ClassifyResult{IsLore: true, Reason: "test"}, nil
}

// mockLoreSummarizer is a test double for LoreSummarizer.
type mockLoreSummarizer struct{}

func (m *mockLoreSummarizer) Summarize(ctx context.Context, req *SummaryRequest) (*SummaryResult, error) {
	return &SummaryResult{Title: "Test Title", Summary: "Test Summary"}, nil
}

// mockTitleGenerator is a test double for TitleGenerator.
type mockTitleGenerator struct{}

func (m *mockTitleGenerator) Generate(ctx context.Context, guildID int64, messages []*Message) (string, error) {
	return "Generated Title", nil
}

// mockThreadCreator is a test double for ThreadCreator.
type mockThreadCreator struct{}

func (m *mockThreadCreator) Create(ctx context.Context, req *ThreadCreateRequest) (*ThreadCreateResult, error) {
	return &ThreadCreateResult{ThreadID: 123, MessageID: 456}, nil
}

// mockMessageFetcher is a test double for MessageFetcher.
type mockMessageFetcher struct{}

func (m *mockMessageFetcher) FetchRecent(ctx context.Context, guildID, channelID int64, limit int) ([]*Message, error) {
	return nil, nil
}

func (m *mockMessageFetcher) FetchByID(ctx context.Context, guildID, messageID int64) (*Message, error) {
	return nil, nil
}

// mockLimiter is a test double for Limiter.
type mockLimiter struct{}

func (m *mockLimiter) Allow(ctx context.Context, guildID int64) bool {
	return true
}

func (m *mockLimiter) Reset(ctx context.Context, guildID int64) error {
	return nil
}

func TestServiceConstruction(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		deps Deps
	}{
		{
			name: "valid construction with all deps",
			cfg: Config{
				IdleTimeout:      300,
				MaxSessionAge:    7200,
				CompactionTarget: 70,
			},
			deps: Deps{
				SessionStore:      &mockSessionStore{sessions: make(map[int64]*Session)},
				ThreadAnchorStore: &mockThreadAnchorStore{},
				GuildSettings:     &mockGuildSettingsStore{},
				Classifier:        &mockLoreClassifier{},
				Summarizer:        &mockLoreSummarizer{},
				TitleGenerator:    &mockTitleGenerator{},
				ThreadCreator:     &mockThreadCreator{},
				MessageFetcher:    &mockMessageFetcher{},
				Clock:             &RealClock{},
				Limiter:           &mockLimiter{},
			},
		},
		{
			name: "construction with fake clock",
			cfg: Config{
				IdleTimeout:      300,
				MaxSessionAge:    7200,
				CompactionTarget: 70,
			},
			deps: Deps{
				SessionStore:      &mockSessionStore{sessions: make(map[int64]*Session)},
				ThreadAnchorStore: &mockThreadAnchorStore{},
				GuildSettings:     &mockGuildSettingsStore{},
				Classifier:        &mockLoreClassifier{},
				Summarizer:        &mockLoreSummarizer{},
				TitleGenerator:    &mockTitleGenerator{},
				ThreadCreator:     &mockThreadCreator{},
				MessageFetcher:    &mockMessageFetcher{},
				Clock:             NewFakeClock(time.Now()),
				Limiter:           &mockLimiter{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewService(tt.cfg, tt.deps)
			if svc == nil {
				t.Fatal("expected non-nil service")
			}
		})
	}
}

func TestNoInitSideEffects(t *testing.T) {
	cfg := Config{
		IdleTimeout:      300,
		MaxSessionAge:    7200,
		CompactionTarget: 70,
	}
	deps := Deps{
		SessionStore:      &mockSessionStore{sessions: make(map[int64]*Session)},
		ThreadAnchorStore: &mockThreadAnchorStore{},
		GuildSettings:     &mockGuildSettingsStore{},
		Classifier:        &mockLoreClassifier{},
		Summarizer:        &mockLoreSummarizer{},
		TitleGenerator:    &mockTitleGenerator{},
		ThreadCreator:     &mockThreadCreator{},
		MessageFetcher:    &mockMessageFetcher{},
		Clock:             NewFakeClock(time.Now()),
		Limiter:           &mockLimiter{},
	}

	svc := NewService(cfg, deps)

	ctx := context.Background()
	err := svc.ProcessMessage(ctx, &Message{})
	if err == nil {
		t.Fatal("expected error from unimplemented method")
	}
}

func TestFakeClockDeterminism(t *testing.T) {
	start := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)
	clock := NewFakeClock(start)

	if !clock.Now().Equal(start) {
		t.Fatalf("expected clock.Now() to be %v, got %v", start, clock.Now())
	}

	clock.Advance(5 * time.Minute)
	expected := start.Add(5 * time.Minute)
	if !clock.Now().Equal(expected) {
		t.Fatalf("expected clock.Now() to be %v after advance, got %v", expected, clock.Now())
	}
}

func TestFakeClockAfter(t *testing.T) {
	start := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)
	clock := NewFakeClock(start)

	ch := clock.After(10 * time.Second)

	select {
	case <-ch:
		t.Fatal("timer should not fire before advance")
	default:
	}

	clock.Advance(10 * time.Second)

	select {
	case <-ch:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timer should fire after advance")
	}
}
