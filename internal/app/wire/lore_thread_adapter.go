package wire

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/eko/iris-bot/internal/lorethread"
	"github.com/eko/iris-bot/internal/orchestrator"
)

type ThreadGateway interface {
	CreateThreadFromMessage(ctx context.Context, guildID, channelID, parentMessageID int64, name string, archiveAfter time.Duration) (int64, error)
	CreateThread(ctx context.Context, guildID, channelID int64, name string, archiveAfter time.Duration) (int64, error)
	SendMessageToThread(ctx context.Context, threadID int64, content string) (int64, error)
}

type DiscordThreadCreator struct {
	gateway ThreadGateway
	logger  *slog.Logger
}

func NewDiscordThreadCreator(gateway ThreadGateway) *DiscordThreadCreator {
	return &DiscordThreadCreator{
		gateway: gateway,
		logger:  slog.Default(),
	}
}

const threadArchiveDuration = 24 * time.Hour

// Create posts a lore summary to a Discord thread.
// Tries to thread off the parent message; on 160004 (thread already exists for that
// message), falls back to a standalone public thread in the channel. Splits long
// summaries into 2000-char chunks; the first chunk's message id is returned as the
// anchor.
func (dtc *DiscordThreadCreator) Create(ctx context.Context, req *lorethread.ThreadCreateRequest) (*lorethread.ThreadCreateResult, error) {
	if req.GuildID == 0 {
		dtc.logger.Warn("thread_creation_rejected_dm", "channel", req.ChannelID)
		return nil, lorethread.ErrDMNotSupported
	}

	threadName := req.Title
	if len(threadName) > 100 {
		threadName = threadName[:97] + "..."
	}

	chunks := orchestrator.SplitMessage(req.FirstMessage, orchestrator.DiscordMessageLimit)
	if len(chunks) == 0 {
		return nil, errors.New("first message is empty")
	}

	threadID, err := dtc.gateway.CreateThreadFromMessage(
		ctx,
		req.GuildID,
		req.ChannelID,
		req.ParentMessageID,
		threadName,
		threadArchiveDuration,
	)
	if err != nil {
		if errors.Is(err, lorethread.ErrThreadAlreadyExists) {
			dtc.logger.Info("falling_back_to_standalone_thread",
				"guild", req.GuildID,
				"channel", req.ChannelID,
				"parent_message", req.ParentMessageID)
			threadID, err = dtc.gateway.CreateThread(
				ctx,
				req.GuildID,
				req.ChannelID,
				threadName,
				threadArchiveDuration,
			)
			if err != nil {
				dtc.logger.Error("failed_to_create_standalone_thread",
					"guild", req.GuildID,
					"channel", req.ChannelID,
					"err", err)
				return nil, fmt.Errorf("failed to create standalone thread: %w", err)
			}
		} else {
			dtc.logger.Error("failed_to_create_thread",
				"guild", req.GuildID,
				"channel", req.ChannelID,
				"parent_message", req.ParentMessageID,
				"err", err)
			return nil, fmt.Errorf("failed to create thread: %w", err)
		}
	}

	firstMessageID, err := dtc.gateway.SendMessageToThread(ctx, threadID, chunks[0])
	if err != nil {
		dtc.logger.Error("failed_to_send_summary_to_thread",
			"guild", req.GuildID,
			"thread", threadID,
			"err", err)
		return nil, fmt.Errorf("failed to send summary to thread: %w", err)
	}

	for i, chunk := range chunks[1:] {
		if _, err := dtc.gateway.SendMessageToThread(ctx, threadID, chunk); err != nil {
			dtc.logger.Warn("failed_to_send_summary_chunk",
				"guild", req.GuildID,
				"thread", threadID,
				"chunk_index", i+1,
				"total_chunks", len(chunks),
				"err", err)
			break
		}
	}

	return &lorethread.ThreadCreateResult{
		ThreadID:  threadID,
		MessageID: firstMessageID,
	}, nil
}
