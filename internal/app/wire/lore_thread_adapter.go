package wire

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/eko/iris-bot/internal/lorethread"
)

// ThreadGateway is the interface for Discord gateway thread operations.
type ThreadGateway interface {
	CreateThreadFromMessage(ctx context.Context, guildID, channelID, parentMessageID int64, name string, archiveAfter time.Duration) (int64, error)
	SendMessageToThread(ctx context.Context, threadID int64, content string) (int64, error)
}

// DiscordThreadCreator implements lorethread.ThreadCreator using the Discord gateway.
type DiscordThreadCreator struct {
	gateway ThreadGateway
	logger  *slog.Logger
}

// NewDiscordThreadCreator creates a new DiscordThreadCreator.
func NewDiscordThreadCreator(gateway ThreadGateway) *DiscordThreadCreator {
	return &DiscordThreadCreator{
		gateway: gateway,
		logger:  slog.Default(),
	}
}

// Create implements lorethread.ThreadCreator.Create.
// It creates a Discord thread from a source message and posts the initial summary.
// Returns ErrDMNotSupported if guildID is 0 (DM marker).
// Returns ErrFirstMessageTooLong if firstMessage exceeds 2000 characters.
// Truncates thread name to 100 characters (Discord's limit) with ellipsis if needed.
func (dtc *DiscordThreadCreator) Create(ctx context.Context, req *lorethread.ThreadCreateRequest) (*lorethread.ThreadCreateResult, error) {
	// Reject DM channels
	if req.GuildID == 0 {
		dtc.logger.Warn("thread_creation_rejected_dm", "channel", req.ChannelID)
		return nil, lorethread.ErrDMNotSupported
	}

	// Reject if first message exceeds Discord's 2000 character limit
	if len(req.FirstMessage) > 2000 {
		dtc.logger.Warn("thread_creation_rejected_long_message",
			"guild", req.GuildID,
			"channel", req.ChannelID,
			"message_length", len(req.FirstMessage))
		return nil, lorethread.ErrFirstMessageTooLong
	}

	// Truncate thread name to 100 characters (Discord's limit)
	threadName := req.Title
	if len(threadName) > 100 {
		threadName = threadName[:97] + "..."
	}

	// Create the thread
	threadID, err := dtc.gateway.CreateThreadFromMessage(
		ctx,
		req.GuildID,
		req.ChannelID,
		req.ParentMessageID,
		threadName,
		24*time.Hour, // Default archive after 24 hours
	)
	if err != nil {
		dtc.logger.Error("failed_to_create_thread",
			"guild", req.GuildID,
			"channel", req.ChannelID,
			"parent_message", req.ParentMessageID,
			"err", err)
		return nil, fmt.Errorf("failed to create thread: %w", err)
	}

	// Send the initial summary message to the thread
	messageID, err := dtc.gateway.SendMessageToThread(ctx, threadID, req.FirstMessage)
	if err != nil {
		dtc.logger.Error("failed_to_send_summary_to_thread",
			"guild", req.GuildID,
			"thread", threadID,
			"err", err)
		return nil, fmt.Errorf("failed to send summary to thread: %w", err)
	}

	return &lorethread.ThreadCreateResult{
		ThreadID:  threadID,
		MessageID: messageID,
	}, nil
}
