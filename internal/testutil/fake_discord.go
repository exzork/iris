package testutil

import (
	"context"
	"time"

	"github.com/eko/iris-bot/internal/domain"
)

// FakeDiscordClient implements domain.DiscordClient for testing.
type FakeDiscordClient struct {
	SendMessageFn func(ctx context.Context, guildID, channelID int64, content string) error
	SendTypingFn  func(ctx context.Context, guildID, channelID int64) error
	GetMessageFn  func(ctx context.Context, guildID, channelID, messageID int64) (*domain.DiscordMessage, error)
	GetGuildFn    func(ctx context.Context, guildID int64) (*domain.Guild, error)

	// Configurable behaviors
	SimulateLatency time.Duration
	SimulateError   error
	SentMessages    []SentMessage
}

// SentMessage tracks a sent message for verification.
type SentMessage struct {
	GuildID   int64
	ChannelID int64
	Content   string
	Timestamp time.Time
}

// NewFakeDiscordClient creates a new fake Discord client with default behavior.
func NewFakeDiscordClient() *FakeDiscordClient {
	return &FakeDiscordClient{
		SentMessages: []SentMessage{},
	}
}

// SendMessage sends a message (or simulates failure).
func (f *FakeDiscordClient) SendMessage(ctx context.Context, guildID, channelID int64, content string) error {
	if f.SimulateLatency > 0 {
		select {
		case <-time.After(f.SimulateLatency):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	if f.SimulateError != nil {
		return f.SimulateError
	}

	if f.SendMessageFn != nil {
		return f.SendMessageFn(ctx, guildID, channelID, content)
	}

	f.SentMessages = append(f.SentMessages, SentMessage{
		GuildID:   guildID,
		ChannelID: channelID,
		Content:   content,
		Timestamp: time.Now(),
	})
	return nil
}

// GetMessage retrieves a message.
func (f *FakeDiscordClient) GetMessage(ctx context.Context, guildID, channelID, messageID int64) (*domain.DiscordMessage, error) {
	if f.SimulateError != nil {
		return nil, f.SimulateError
	}

	if f.GetMessageFn != nil {
		return f.GetMessageFn(ctx, guildID, channelID, messageID)
	}

	return &domain.DiscordMessage{
		ID:        messageID,
		GuildID:   guildID,
		ChannelID: channelID,
		UserID:    12345,
		Content:   "test message",
		CreatedAt: time.Now(),
	}, nil
}

// GetGuild retrieves a guild.
func (f *FakeDiscordClient) GetGuild(ctx context.Context, guildID int64) (*domain.Guild, error) {
	if f.SimulateError != nil {
		return nil, f.SimulateError
	}

	if f.GetGuildFn != nil {
		return f.GetGuildFn(ctx, guildID)
	}

	return &domain.Guild{
		ID:        guildID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, nil
}

// SendTyping sends a typing indicator.
func (f *FakeDiscordClient) SendTyping(ctx context.Context, guildID, channelID int64) error {
	if f.SendTypingFn != nil {
		return f.SendTypingFn(ctx, guildID, channelID)
	}
	return nil
}

// GetSentMessages returns all sent messages for verification.
func (f *FakeDiscordClient) GetSentMessages() []SentMessage {
	return f.SentMessages
}

// ClearSentMessages clears the sent messages log.
func (f *FakeDiscordClient) ClearSentMessages() {
	f.SentMessages = []SentMessage{}
}
