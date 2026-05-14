package admin

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

type ExceptionHandler struct {
	store ExceptionChannelStore
	audit AuditLogger
}

func NewExceptionHandler(store ExceptionChannelStore, audit AuditLogger) *ExceptionHandler {
	return &ExceptionHandler{
		store: store,
		audit: audit,
	}
}

func (h *ExceptionHandler) Handle(ctx context.Context, cmd *CommandContext, args []string) (string, error) {
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

func (h *ExceptionHandler) handleAdd(ctx context.Context, cmd *CommandContext, args []string) (string, error) {
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

	h.audit.Log(ctx, cmd.GuildID, cmd.UserID, "exception_channel_added", "exception_channel", fmt.Sprintf("%d", channelID), map[string]interface{}{
		"channel_id": channelID,
	})

	return fmt.Sprintf("Channel `%d` telah ditambahkan ke daftar pengecualian.", channelID), nil
}

func (h *ExceptionHandler) handleRemove(ctx context.Context, cmd *CommandContext, args []string) (string, error) {
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

	h.audit.Log(ctx, cmd.GuildID, cmd.UserID, "exception_channel_removed", "exception_channel", fmt.Sprintf("%d", channelID), map[string]interface{}{
		"channel_id": channelID,
	})

	return fmt.Sprintf("Channel `%d` telah dihapus dari daftar pengecualian.", channelID), nil
}

func (h *ExceptionHandler) handleList(ctx context.Context, cmd *CommandContext, args []string) (string, error) {
	channels, err := h.store.List(ctx, cmd.GuildID)
	if err != nil {
		return "", err
	}

	if len(channels) == 0 {
		return "Tidak ada channel pengecualian yang terdaftar.", nil
	}

	var sb strings.Builder
	sb.WriteString("Channel pengecualian:\n")
	for _, ch := range channels {
		sb.WriteString(fmt.Sprintf("- `%d`\n", ch))
	}

	return sb.String(), nil
}
