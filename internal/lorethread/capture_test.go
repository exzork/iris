package lorethread

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// MockSessionStore implements SessionStore for testing.
type MockSessionStore struct {
	sessions map[string]*Session
	err      error
}

func NewMockSessionStore() *MockSessionStore {
	return &MockSessionStore{
		sessions: make(map[string]*Session),
	}
}

func (m *MockSessionStore) Create(ctx context.Context, session *Session) error {
	if m.err != nil {
		return m.err
	}
	session.ID = int64(len(m.sessions) + 1)
	key := sessionKey(session.GuildID, session.ChannelID)
	m.sessions[key] = session
	return nil
}

func (m *MockSessionStore) GetByID(ctx context.Context, id int64) (*Session, error) {
	if m.err != nil {
		return nil, m.err
	}
	for _, s := range m.sessions {
		if s.ID == id {
			return s, nil
		}
	}
	return nil, nil
}

func (m *MockSessionStore) GetActive(ctx context.Context, guildID, channelID int64) (*Session, error) {
	if m.err != nil {
		return nil, m.err
	}
	key := sessionKey(guildID, channelID)
	return m.sessions[key], nil
}

func (m *MockSessionStore) Update(ctx context.Context, session *Session) error {
	if m.err != nil {
		return m.err
	}
	key := sessionKey(session.GuildID, session.ChannelID)
	m.sessions[key] = session
	return nil
}

func (m *MockSessionStore) ListByGuild(ctx context.Context, guildID int64) ([]*Session, error) {
	if m.err != nil {
		return nil, m.err
	}
	var result []*Session
	for _, s := range m.sessions {
		if s.GuildID == guildID {
			result = append(result, s)
		}
	}
	return result, nil
}

func sessionKey(guildID, channelID int64) string {
	return fmt.Sprintf("%d:%d", guildID, channelID)
}

// MockGuildSettingsStore implements GuildSettingsStore for testing.
type MockGuildSettingsStore struct {
	enabled map[int64]bool
	err     error
}

func NewMockGuildSettingsStore() *MockGuildSettingsStore {
	return &MockGuildSettingsStore{
		enabled: make(map[int64]bool),
	}
}

func (m *MockGuildSettingsStore) GetLoreThreadEnabled(ctx context.Context, guildID int64) (bool, error) {
	if m.err != nil {
		return false, m.err
	}
	return m.enabled[guildID], nil
}

func (m *MockGuildSettingsStore) SetLoreThreadEnabled(ctx context.Context, guildID int64, enabled bool) error {
	if m.err != nil {
		return m.err
	}
	m.enabled[guildID] = enabled
	return nil
}

// MockLoreClassifier implements LoreClassifier for testing.
type MockLoreClassifier struct {
	result *ClassifyResult
	err    error
}

func NewMockLoreClassifier(isLore bool) *MockLoreClassifier {
	return &MockLoreClassifier{
		result: &ClassifyResult{IsLore: isLore},
	}
}

func (m *MockLoreClassifier) Classify(ctx context.Context, guildID int64, message *Message) (*ClassifyResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.result, nil
}

func TestCapturer_FeatureDisabled(t *testing.T) {
	capturer := NewCapturer(CapturerDeps{
		SessionStore:       NewMockSessionStore(),
		GuildSettingsStore: NewMockGuildSettingsStore(),
		LoreClassifier:     NewMockLoreClassifier(true),
		Clock:              &RealClock{},
		IdleDuration:       5 * time.Minute,
		Enabled:            false,
	})

	msg := &Message{
		ID:        1,
		GuildID:   123,
		ChannelID: 456,
		AuthorID:  789,
		Content:   "test",
		CreatedAt: time.Now(),
	}

	err := capturer.OnMessage(context.Background(), msg)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestCapturer_DMSkipped(t *testing.T) {
	capturer := NewCapturer(CapturerDeps{
		SessionStore:       NewMockSessionStore(),
		GuildSettingsStore: NewMockGuildSettingsStore(),
		LoreClassifier:     NewMockLoreClassifier(true),
		Clock:              &RealClock{},
		IdleDuration:       5 * time.Minute,
		Enabled:            true,
	})

	msg := &Message{
		ID:        1,
		GuildID:   0,
		ChannelID: 456,
		AuthorID:  789,
		Content:   "test",
		CreatedAt: time.Now(),
	}

	err := capturer.OnMessage(context.Background(), msg)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestCapturer_BotMessageSkipped(t *testing.T) {
	capturer := NewCapturer(CapturerDeps{
		SessionStore:       NewMockSessionStore(),
		GuildSettingsStore: NewMockGuildSettingsStore(),
		LoreClassifier:     NewMockLoreClassifier(true),
		Clock:              &RealClock{},
		IdleDuration:       5 * time.Minute,
		Enabled:            true,
	})

	msg := &Message{
		ID:          1,
		GuildID:     123,
		ChannelID:   456,
		AuthorID:    789,
		AuthorIsBot: true,
		Content:     "test",
		CreatedAt:   time.Now(),
	}

	err := capturer.OnMessage(context.Background(), msg)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestCapturer_NonLoreMessageSkipped(t *testing.T) {
	capturer := NewCapturer(CapturerDeps{
		SessionStore:       NewMockSessionStore(),
		GuildSettingsStore: NewMockGuildSettingsStore(),
		LoreClassifier:     NewMockLoreClassifier(false),
		Clock:              &RealClock{},
		IdleDuration:       5 * time.Minute,
		Enabled:            true,
	})

	settings := NewMockGuildSettingsStore()
	settings.enabled[123] = true

	capturer.guildSettingsStore = settings

	msg := &Message{
		ID:        1,
		GuildID:   123,
		ChannelID: 456,
		AuthorID:  789,
		Content:   "test",
		CreatedAt: time.Now(),
	}

	err := capturer.OnMessage(context.Background(), msg)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestCapturer_FeatureDisabledForGuild(t *testing.T) {
	settings := NewMockGuildSettingsStore()
	settings.enabled[123] = false

	capturer := NewCapturer(CapturerDeps{
		SessionStore:       NewMockSessionStore(),
		GuildSettingsStore: settings,
		LoreClassifier:     NewMockLoreClassifier(true),
		Clock:              &RealClock{},
		IdleDuration:       5 * time.Minute,
		Enabled:            true,
	})

	msg := &Message{
		ID:        1,
		GuildID:   123,
		ChannelID: 456,
		AuthorID:  789,
		Content:   "test",
		CreatedAt: time.Now(),
	}

	err := capturer.OnMessage(context.Background(), msg)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestCapturer_CreateNewSession(t *testing.T) {
	sessionStore := NewMockSessionStore()
	settings := NewMockGuildSettingsStore()
	settings.enabled[123] = true

	clock := NewFakeClock(time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC))

	capturer := NewCapturer(CapturerDeps{
		SessionStore:       sessionStore,
		GuildSettingsStore: settings,
		LoreClassifier:     NewMockLoreClassifier(true),
		Clock:              clock,
		IdleDuration:       5 * time.Minute,
		Enabled:            true,
	})

	msg := &Message{
		ID:        1,
		GuildID:   123,
		ChannelID: 456,
		AuthorID:  789,
		Content:   "test message",
		CreatedAt: time.Now(),
	}

	err := capturer.OnMessage(context.Background(), msg)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	session, err := sessionStore.GetActive(context.Background(), 123, 456)
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}

	if session == nil {
		t.Fatal("expected session to be created")
	}

	if len(session.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(session.Messages))
	}

	if session.FirstMessage.ID != 1 {
		t.Fatalf("expected first message ID 1, got %d", session.FirstMessage.ID)
	}
}

func TestCapturer_RefreshExistingSession(t *testing.T) {
	sessionStore := NewMockSessionStore()
	settings := NewMockGuildSettingsStore()
	settings.enabled[123] = true

	clock := NewFakeClock(time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC))

	capturer := NewCapturer(CapturerDeps{
		SessionStore:       sessionStore,
		GuildSettingsStore: settings,
		LoreClassifier:     NewMockLoreClassifier(true),
		Clock:              clock,
		IdleDuration:       5 * time.Minute,
		Enabled:            true,
	})

	msg1 := &Message{
		ID:        1,
		GuildID:   123,
		ChannelID: 456,
		AuthorID:  789,
		Content:   "first message",
		CreatedAt: time.Now(),
	}

	err := capturer.OnMessage(context.Background(), msg1)
	if err != nil {
		t.Fatalf("expected nil error on first message, got %v", err)
	}

	session1, _ := sessionStore.GetActive(context.Background(), 123, 456)
	sessionID1 := session1.ID

	msg2 := &Message{
		ID:        2,
		GuildID:   123,
		ChannelID: 456,
		AuthorID:  789,
		Content:   "second message",
		CreatedAt: time.Now(),
	}

	err = capturer.OnMessage(context.Background(), msg2)
	if err != nil {
		t.Fatalf("expected nil error on second message, got %v", err)
	}

	session2, _ := sessionStore.GetActive(context.Background(), 123, 456)

	if session2.ID != sessionID1 {
		t.Fatalf("expected same session ID, got different IDs: %d vs %d", sessionID1, session2.ID)
	}

	if len(session2.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(session2.Messages))
	}

	if session2.Messages[1].ID != 2 {
		t.Fatalf("expected second message ID 2, got %d", session2.Messages[1].ID)
	}
}

func TestCapturer_ClassifierErrorReturnsNil(t *testing.T) {
	classifier := &MockLoreClassifier{
		err: errors.New("classifier error"),
	}

	capturer := NewCapturer(CapturerDeps{
		SessionStore:       NewMockSessionStore(),
		GuildSettingsStore: NewMockGuildSettingsStore(),
		LoreClassifier:     classifier,
		Clock:              &RealClock{},
		IdleDuration:       5 * time.Minute,
		Enabled:            true,
	})

	msg := &Message{
		ID:        1,
		GuildID:   123,
		ChannelID: 456,
		AuthorID:  789,
		Content:   "test",
		CreatedAt: time.Now(),
	}

	err := capturer.OnMessage(context.Background(), msg)
	if err != nil {
		t.Fatalf("expected nil error on classifier error, got %v", err)
	}
}

func TestCapturer_SettingsStoreErrorReturnsNil(t *testing.T) {
	settings := &MockGuildSettingsStore{
		err: errors.New("settings error"),
	}

	capturer := NewCapturer(CapturerDeps{
		SessionStore:       NewMockSessionStore(),
		GuildSettingsStore: settings,
		LoreClassifier:     NewMockLoreClassifier(true),
		Clock:              &RealClock{},
		IdleDuration:       5 * time.Minute,
		Enabled:            true,
	})

	msg := &Message{
		ID:        1,
		GuildID:   123,
		ChannelID: 456,
		AuthorID:  789,
		Content:   "test",
		CreatedAt: time.Now(),
	}

	err := capturer.OnMessage(context.Background(), msg)
	if err != nil {
		t.Fatalf("expected nil error on settings error, got %v", err)
	}
}

func TestCapturer_SessionStoreCreateErrorReturnsNil(t *testing.T) {
	sessionStore := &MockSessionStore{
		sessions: make(map[string]*Session),
		err:      errors.New("create error"),
	}

	settings := NewMockGuildSettingsStore()
	settings.enabled[123] = true

	capturer := NewCapturer(CapturerDeps{
		SessionStore:       sessionStore,
		GuildSettingsStore: settings,
		LoreClassifier:     NewMockLoreClassifier(true),
		Clock:              &RealClock{},
		IdleDuration:       5 * time.Minute,
		Enabled:            true,
	})

	msg := &Message{
		ID:        1,
		GuildID:   123,
		ChannelID: 456,
		AuthorID:  789,
		Content:   "test",
		CreatedAt: time.Now(),
	}

	err := capturer.OnMessage(context.Background(), msg)
	if err != nil {
		t.Fatalf("expected nil error on session store error, got %v", err)
	}
}
