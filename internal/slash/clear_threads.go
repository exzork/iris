package slash

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/bwmarrin/discordgo"
)

// ThreadAnchorPurger lists and deletes Iris-tracked lore-thread anchors for a guild.
type ThreadAnchorPurger interface {
	ListThreadIDsByGuild(ctx context.Context, guildID int64) ([]int64, error)
	DeleteAllByGuild(ctx context.Context, guildID int64) (int64, error)
}

// LoreSessionPurger clears the lore_sessions rows that anchored those threads.
type LoreSessionPurger interface {
	DeleteAllByGuild(ctx context.Context, guildID int64) (int64, error)
}

// ThreadDeleter wraps the Discord-side channel deletion call so we can stub it in tests.
type ThreadDeleter interface {
	DeleteThread(ctx context.Context, threadID int64) error
}

type ClearThreadsHandler struct {
	anchors  ThreadAnchorPurger
	sessions LoreSessionPurger
	deleter  ThreadDeleter
}

func NewClearThreadsHandler(anchors ThreadAnchorPurger, sessions LoreSessionPurger, deleter ThreadDeleter) *ClearThreadsHandler {
	return &ClearThreadsHandler{anchors: anchors, sessions: sessions, deleter: deleter}
}

func newClearThreadsCommand(h *ClearThreadsHandler) NativeCommand {
	return NativeCommand{
		Name:        "iris-clear-threads",
		Description: "Hapus semua thread yang dibuat iris untuk server ini (admin only).",
		AdminOnly:   true,
		Execute: func(ctx context.Context, inv *NativeInvocation) (string, error) {
			if !inv.IsAdmin {
				return "Mohon maaf, hanya admin server yang dapat menjalankan perintah ini.", nil
			}
			if inv.GuildID == 0 {
				return "Perintah ini hanya bisa dijalankan di server, bukan DM.", nil
			}
			return h.run(ctx, inv.GuildID)
		},
	}
}

func (h *ClearThreadsHandler) run(ctx context.Context, guildID int64) (string, error) {
	if h.anchors == nil {
		return "Penyimpanan anchor lore tidak tersedia.", nil
	}

	ids, err := h.anchors.ListThreadIDsByGuild(ctx, guildID)
	if err != nil {
		slog.WarnContext(ctx, "clear_threads_list_failed", "guild", guildID, "err", err)
		return "", fmt.Errorf("gagal mengambil daftar thread: %w", err)
	}
	if len(ids) == 0 {
		return "Tidak ada thread iris yang tercatat untuk server ini.", nil
	}

	var (
		deleted int
		gone    int
		failed  int
	)
	for _, tid := range ids {
		if h.deleter == nil {
			failed++
			continue
		}
		if err := h.deleter.DeleteThread(ctx, tid); err != nil {
			var restErr *discordgo.RESTError
			if errors.As(err, &restErr) && restErr.Message != nil &&
				restErr.Message.Code == discordgo.ErrCodeUnknownChannel {
				gone++
				continue
			}
			slog.WarnContext(ctx, "clear_threads_delete_failed", "guild", guildID, "thread", tid, "err", err)
			failed++
			continue
		}
		deleted++
	}

	rowsAnchors, err := h.anchors.DeleteAllByGuild(ctx, guildID)
	if err != nil {
		slog.WarnContext(ctx, "clear_threads_anchor_purge_failed", "guild", guildID, "err", err)
		return "", fmt.Errorf("gagal membersihkan anchor di database: %w", err)
	}
	var rowsSessions int64
	if h.sessions != nil {
		rowsSessions, err = h.sessions.DeleteAllByGuild(ctx, guildID)
		if err != nil {
			slog.WarnContext(ctx, "clear_threads_session_purge_failed", "guild", guildID, "err", err)
			return "", fmt.Errorf("gagal membersihkan sesi lore di database: %w", err)
		}
	}

	msg := fmt.Sprintf("Selesai. %d thread dihapus, %d sudah tidak ada, %d gagal. DB: %d anchor, %d sesi dibersihkan.",
		deleted, gone, failed, rowsAnchors, rowsSessions)
	if failed > 0 {
		msg += " Cek log bot untuk detail kegagalan."
	}
	return msg, nil
}
