package wire

import (
	"context"
	"testing"
	"time"

	"github.com/eko/iris-bot/internal/domain"
	"github.com/eko/iris-bot/internal/lorethread"
)

type MockAllowedChannelQuerier interface {
	HasAny(ctx context.Context, guildID int64) (bool, error)
	IsAllowed(ctx context.Context, guildID, channelID int64) (bool, error)
}

type MockAllowedChannelRepo struct {
	hasAny    bool
	isAllowed map[int64]map[int64]bool
	err       error
}

func NewMockAllowedChannelRepo() *MockAllowedChannelRepo {
	return &MockAllowedChannelRepo{
		isAllowed: make(map[int64]map[int64]bool),
	}
}

func (m *MockAllowedChannelRepo) HasAny(ctx context.Context, guildID int64) (bool, error) {
	if m.err != nil {
		return false, m.err
	}
	return m.hasAny, nil
}

func (m *MockAllowedChannelRepo) IsAllowed(ctx context.Context, guildID, channelID int64) (bool, error) {
	if m.err != nil {
		return false, m.err
	}
	if m.isAllowed[guildID] == nil {
		return false, nil
	}
	return m.isAllowed[guildID][channelID], nil
}

func (m *MockAllowedChannelRepo) SetAllowed(ctx context.Context, guildID, channelID int64) error {
	if m.isAllowed[guildID] == nil {
		m.isAllowed[guildID] = make(map[int64]bool)
	}
	m.isAllowed[guildID][channelID] = true
	return nil
}

type MockCapturer struct {
	called   bool
	lastMsg  *lorethread.Message
	err      error
}

func NewMockCapturer() *MockCapturer {
	return &MockCapturer{}
}

func (m *MockCapturer) OnMessage(ctx context.Context, msg *lorethread.Message) error {
	m.called = true
	m.lastMsg = msg
	return m.err
}

func TestLoreCapturer_BotMessageSkipped(t *testing.T) {
	capturer := NewMockCapturer()
	allowedRepo := NewMockAllowedChannelRepo()
	allowedRepo.hasAny = true
	allowedRepo.SetAllowed(context.Background(), 123, 456)

	adapter := &LoreCapturer{
		capturer:              &lorethread.Capturer{},
		allowedChannelQuerier: allowedRepo,
		botID:                 999,
	}

	event := &domain.DiscordEvent{
		GuildID:   123,
		ChannelID: 456,
		UserID:    789,
		IsBot:     true,
		Message: &domain.DiscordMessage{
			ID:        1,
			Content:   "test",
			CreatedAt: time.Now(),
		},
	}

	adapter.capturer = capturer
	adapter.OnMessage(context.Background(), event)

	if capturer.called {
		t.Fatal("expected capturer not to be called for bot message")
	}
}

func TestLoreCapturer_DMSkipped(t *testing.T) {
	capturer := NewMockCapturer()
	allowedRepo := NewMockAllowedChannelRepo()

	adapter := &LoreCapturer{
		capturer:              &lorethread.Capturer{},
		allowedChannelQuerier: allowedRepo,
		botID:                 999,
	}

	event := &domain.DiscordEvent{
		GuildID:   0,
		ChannelID: 456,
		UserID:    789,
		IsBot:     false,
		Message: &domain.DiscordMessage{
			ID:        1,
			Content:   "test",
			CreatedAt: time.Now(),
		},
	}

	adapter.capturer = capturer
	adapter.OnMessage(context.Background(), event)

	if capturer.called {
		t.Fatal("expected capturer not to be called for DM")
	}
}

func TestLoreCapturer_DisallowedChannelSkipped(t *testing.T) {
	capturer := NewMockCapturer()
	allowedRepo := NewMockAllowedChannelRepo()
	allowedRepo.hasAny = true
	allowedRepo.SetAllowed(context.Background(), 123, 999)

	adapter := &LoreCapturer{
		capturer:              &lorethread.Capturer{},
		allowedChannelQuerier: allowedRepo,
		botID:                 888,
	}

	event := &domain.DiscordEvent{
		GuildID:   123,
		ChannelID: 456,
		UserID:    789,
		IsBot:     false,
		Message: &domain.DiscordMessage{
			ID:        1,
			Content:   "test",
			CreatedAt: time.Now(),
		},
	}

	adapter.capturer = capturer
	adapter.OnMessage(context.Background(), event)

	if capturer.called {
		t.Fatal("expected capturer not to be called for disallowed channel")
	}
}

func TestLoreCapturer_AllowedChannelProcessed(t *testing.T) {
	capturer := NewMockCapturer()
	allowedRepo := NewMockAllowedChannelRepo()
	allowedRepo.hasAny = true
	allowedRepo.SetAllowed(context.Background(), 123, 456)

	adapter := &LoreCapturer{
		capturer:              &lorethread.Capturer{},
		allowedChannelQuerier: allowedRepo,
		botID:                 888,
	}

	now := time.Now()
	event := &domain.DiscordEvent{
		GuildID:   123,
		ChannelID: 456,
		UserID:    789,
		IsBot:     false,
		Message: &domain.DiscordMessage{
			ID:        1,
			Content:   "test message",
			CreatedAt: now,
		},
	}

	adapter.capturer = capturer
	adapter.OnMessage(context.Background(), event)

	if !capturer.called {
		t.Fatal("expected capturer to be called for allowed channel")
	}

	if capturer.lastMsg == nil {
		t.Fatal("expected message to be passed to capturer")
	}

	if capturer.lastMsg.ID != 1 {
		t.Fatalf("expected message ID 1, got %d", capturer.lastMsg.ID)
	}

	if capturer.lastMsg.GuildID != 123 {
		t.Fatalf("expected guild ID 123, got %d", capturer.lastMsg.GuildID)
	}

	if capturer.lastMsg.ChannelID != 456 {
		t.Fatalf("expected channel ID 456, got %d", capturer.lastMsg.ChannelID)
	}

	if capturer.lastMsg.AuthorID != 789 {
		t.Fatalf("expected author ID 789, got %d", capturer.lastMsg.AuthorID)
	}

	if capturer.lastMsg.Content != "test message" {
		t.Fatalf("expected content 'test message', got %q", capturer.lastMsg.Content)
	}
}

func TestLoreCapturer_BotIDSkipped(t *testing.T) {
	capturer := NewMockCapturer()
	allowedRepo := NewMockAllowedChannelRepo()
	allowedRepo.hasAny = true
	allowedRepo.SetAllowed(context.Background(), 123, 456)

	adapter := &LoreCapturer{
		capturer:              &lorethread.Capturer{},
		allowedChannelQuerier: allowedRepo,
		botID:                 789,
	}

	event := &domain.DiscordEvent{
		GuildID:   123,
		ChannelID: 456,
		UserID:    789,
		IsBot:     false,
		Message: &domain.DiscordMessage{
			ID:        1,
			Content:   "test",
			CreatedAt: time.Now(),
		},
	}

	adapter.capturer = capturer
	adapter.OnMessage(context.Background(), event)

	if capturer.called {
		t.Fatal("expected capturer not to be called when UserID matches botID")
	}
}

func TestLoreCapturer_NoAllowedChannelsPassthrough(t *testing.T) {
	capturer := NewMockCapturer()
	allowedRepo := NewMockAllowedChannelRepo()
	allowedRepo.hasAny = false

	adapter := &LoreCapturer{
		capturer:              &lorethread.Capturer{},
		allowedChannelQuerier: allowedRepo,
		botID:                 888,
	}

	event := &domain.DiscordEvent{
		GuildID:   123,
		ChannelID: 456,
		UserID:    789,
		IsBot:     false,
		Message: &domain.DiscordMessage{
			ID:        1,
			Content:   "test",
			CreatedAt: time.Now(),
		},
	}

	adapter.capturer = capturer
	adapter.OnMessage(context.Background(), event)

	if !capturer.called {
		t.Fatal("expected capturer to be called when no allowed channels configured (fallback mode)")
	}
}

