package slash

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/eko/iris-bot/internal/discord"
)

type ThreadLister interface {
	ListThreadsInChannel(ctx context.Context, parentChannelID int64) ([]discord.ThreadInfo, error)
}

type ThreadDeleter interface {
	DeleteThread(ctx context.Context, threadID int64) error
}

type BotIDProvider interface {
	BotID() int64
}

type ClearThreadsHandler struct {
	lister  ThreadLister
	deleter ThreadDeleter
	bot     BotIDProvider
}

func NewClearThreadsHandler(lister ThreadLister, deleter ThreadDeleter, bot BotIDProvider) *ClearThreadsHandler {
	return &ClearThreadsHandler{lister: lister, deleter: deleter, bot: bot}
}

func newClearThreadsCommand(h *ClearThreadsHandler) NativeCommand {
	return NativeCommand{
		Name:        "iris-clear-threads",
		Description: "Hapus semua thread di channel ini yang dibuat iris (admin only).",
		AdminOnly:   true,
		Execute: func(ctx context.Context, inv *NativeInvocation) (string, error) {
			if !inv.IsAdmin {
				return "Mohon maaf, hanya admin server yang dapat menjalankan perintah ini.", nil
			}
			if inv.GuildID == 0 {
				return "Perintah ini hanya bisa dijalankan di server, bukan DM.", nil
			}
			if inv.ChannelID == 0 {
				return "Channel tidak terdeteksi.", nil
			}
			return h.run(ctx, inv.ChannelID)
		},
	}
}

func (h *ClearThreadsHandler) run(ctx context.Context, parentChannelID int64) (string, error) {
	if h.lister == nil || h.deleter == nil || h.bot == nil {
		return "Layanan thread tidak tersedia.", nil
	}

	botID := h.bot.BotID()
	if botID == 0 {
		return "Bot ID belum siap, coba lagi sebentar.", nil
	}

	threads, err := h.lister.ListThreadsInChannel(ctx, parentChannelID)
	if err != nil {
		slog.WarnContext(ctx, "clear_threads_list_failed", "channel", parentChannelID, "err", err)
		return "", fmt.Errorf("gagal mengambil daftar thread: %w", err)
	}

	var owned []discord.ThreadInfo
	for _, t := range threads {
		if t.OwnerID == botID {
			owned = append(owned, t)
		}
	}
	if len(owned) == 0 {
		return "Tidak ada thread iris di channel ini.", nil
	}

	var (
		deleted []string
		gone    int
		failed  []string
	)
	for _, t := range owned {
		if err := h.deleter.DeleteThread(ctx, t.ID); err != nil {
			var restErr *discordgo.RESTError
			if errors.As(err, &restErr) && restErr.Message != nil &&
				restErr.Message.Code == discordgo.ErrCodeUnknownChannel {
				gone++
				continue
			}
			slog.WarnContext(ctx, "clear_threads_delete_failed",
				"channel", parentChannelID, "thread", t.ID, "name", t.Name, "err", err)
			failed = append(failed, t.Name)
			continue
		}
		deleted = append(deleted, t.Name)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Selesai. %d thread dihapus, %d sudah tidak ada, %d gagal.",
		len(deleted), gone, len(failed))
	if len(deleted) > 0 {
		fmt.Fprintf(&sb, " Dihapus: %s.", strings.Join(deleted, ", "))
	}
	if len(failed) > 0 {
		fmt.Fprintf(&sb, " Gagal: %s. Cek log bot untuk detail.", strings.Join(failed, ", "))
	}
	return sb.String(), nil
}
