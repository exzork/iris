package admin

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

type AllowedChannelStore interface {
	Add(ctx context.Context, guildID, channelID int64) error
	Remove(ctx context.Context, guildID, channelID int64) error
	List(ctx context.Context, guildID int64) ([]int64, error)
}

type AllowedChannelHandler struct {
	store AllowedChannelStore
	audit AuditLogger
}

func NewAllowedChannelHandler(store AllowedChannelStore, audit AuditLogger) *AllowedChannelHandler {
	return &AllowedChannelHandler{
		store: store,
		audit: audit,
	}
}

func (h *AllowedChannelHandler) Handle(ctx context.Context, cmd *CommandContext, args []string) (string, error) {
	if len(args) == 0 {
		return "Format perintah salah. Gunakan `!iris help` untuk melihat daftar perintah.", nil
	}

	sub := args[0]

	switch sub {
	case "add":
		return h.handleAdd(ctx, cmd, args[1:])
	case "remove":
		return h.handleRemove(ctx, cmd, args[1:])
	case "list":
		return h.handleList(ctx, cmd, args[1:])
	default:
		return "Format perintah salah. Gunakan `!iris help` untuk melihat daftar perintah.", nil
	}
}

func (h *AllowedChannelHandler) handleAdd(ctx context.Context, cmd *CommandContext, args []string) (string, error) {
	if len(args) != 1 {
		return "Format perintah salah. Gunakan `!iris help` untuk melihat daftar perintah.", nil
	}

	channelID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return "Format perintah salah. Gunakan `!iris help` untuk melihat daftar perintah.", nil
	}

	if err := h.store.Add(ctx, cmd.GuildID, channelID); err != nil {
		return "", err
	}

	h.audit.Log(ctx, cmd.GuildID, cmd.UserID, "allowed_channel_added", "allowed_channel", fmt.Sprintf("%d", channelID), map[string]interface{}{
		"channel_id": channelID,
	})

	return fmt.Sprintf("Channel `%d` telah ditambahkan ke daftar saluran yang diizinkan.", channelID), nil
}

func (h *AllowedChannelHandler) handleRemove(ctx context.Context, cmd *CommandContext, args []string) (string, error) {
	if len(args) != 1 {
		return "Format perintah salah. Gunakan `!iris help` untuk melihat daftar perintah.", nil
	}

	channelID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return "Format perintah salah. Gunakan `!iris help` untuk melihat daftar perintah.", nil
	}

	if err := h.store.Remove(ctx, cmd.GuildID, channelID); err != nil {
		return "", err
	}

	h.audit.Log(ctx, cmd.GuildID, cmd.UserID, "allowed_channel_removed", "allowed_channel", fmt.Sprintf("%d", channelID), map[string]interface{}{
		"channel_id": channelID,
	})

	return fmt.Sprintf("Channel `%d` telah dihapus dari daftar saluran yang diizinkan.", channelID), nil
}

func (h *AllowedChannelHandler) handleList(ctx context.Context, cmd *CommandContext, args []string) (string, error) {
	channels, err := h.store.List(ctx, cmd.GuildID)
	if err != nil {
		return "", err
	}

	if len(channels) == 0 {
		return "Tidak ada saluran yang diizinkan untuk guild ini.", nil
	}

	var sb strings.Builder
	sb.WriteString("Saluran yang diizinkan:\n")
	for _, ch := range channels {
		sb.WriteString(fmt.Sprintf("- `%d`\n", ch))
	}

	return sb.String(), nil
}
