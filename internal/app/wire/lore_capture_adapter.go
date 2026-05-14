package wire

import (
	"context"
	"log/slog"

	"github.com/eko/iris-bot/internal/domain"
	"github.com/eko/iris-bot/internal/lorethread"
)

type AllowedChannelQuerier interface {
	HasAny(ctx context.Context, guildID int64) (bool, error)
	IsAllowed(ctx context.Context, guildID, channelID int64) (bool, error)
}

type CapturerInterface interface {
	OnMessage(ctx context.Context, msg *lorethread.Message) error
}

// LoreCapturer adapts the lorethread.Capturer to the orchestrator's event flow.
// It handles allowed-channel filtering and converts domain.DiscordEvent to lorethread.Message.
type LoreCapturer struct {
	capturer              CapturerInterface
	allowedChannelQuerier AllowedChannelQuerier
	botID                 int64
}

// NewLoreCapturer creates a new LoreCapturer adapter.
func NewLoreCapturer(
	capturer CapturerInterface,
	allowedChannelQuerier AllowedChannelQuerier,
	botID int64,
) *LoreCapturer {
	return &LoreCapturer{
		capturer:              capturer,
		allowedChannelQuerier: allowedChannelQuerier,
		botID:                 botID,
	}
}

// OnMessage processes a Discord event for lore capture.
// Applies allowed-channel filtering before delegating to the capturer.
func (lc *LoreCapturer) OnMessage(ctx context.Context, event *domain.DiscordEvent) {
	if event == nil || event.Message == nil {
		return
	}

	// Skip if message is from the bot itself
	if event.UserID == lc.botID {
		return
	}

	// Skip bot messages (v1: exclude all bot messages)
	if event.IsBot {
		return
	}

	// Skip DMs
	if event.GuildID == 0 {
		return
	}

	// Check allowed-channel filtering
	hasAllowed, err := lc.allowedChannelQuerier.HasAny(ctx, event.GuildID)
	if err != nil {
		slog.DebugContext(ctx, "lore_capture_allowed_check_error", "guild", event.GuildID, "error", err)
		return
	}

	if hasAllowed {
		isAllowed, err := lc.allowedChannelQuerier.IsAllowed(ctx, event.GuildID, event.ChannelID)
		if err != nil {
			slog.DebugContext(ctx, "lore_capture_allowed_check_error", "guild", event.GuildID, "channel", event.ChannelID, "error", err)
			return
		}

		if !isAllowed {
			return
		}
	}

	// Convert domain.DiscordEvent to lorethread.Message
	msg := &lorethread.Message{
		ID:          event.Message.ID,
		GuildID:     event.GuildID,
		ChannelID:   event.ChannelID,
		AuthorID:    event.UserID,
		AuthorIsBot: event.IsBot,
		Content:     event.Message.Content,
		CreatedAt:   event.Message.CreatedAt,
	}

	// Call capturer (non-blocking, errors logged at DEBUG)
	if err := lc.capturer.OnMessage(ctx, msg); err != nil {
		slog.DebugContext(ctx, "lore_capture_error", "guild", event.GuildID, "channel", event.ChannelID, "error", err)
	}
}
