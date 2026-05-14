package admin

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

var whitelistedKeys = map[string]bool{
	"admin_role_ids":           true,
	"default_locale":           true,
	"memory_enabled":           true,
	"lore_citations_required":  true,
	"max_response_chars":       true,
}

type SettingsHandler struct {
	store SettingsStore
	audit AuditLogger
}

func NewSettingsHandler(store SettingsStore, audit AuditLogger) *SettingsHandler {
	return &SettingsHandler{
		store: store,
		audit: audit,
	}
}

func (h *SettingsHandler) Handle(ctx context.Context, cmd *CommandContext, args []string) (string, error) {
	if len(args) == 0 {
		return "Format perintah salah. Gunakan `!iris help` untuk melihat daftar perintah.", nil
	}

	sub := args[0]

	switch sub {
	case "set":
		return h.handleSet(ctx, cmd, args[1:])
	case "get":
		return h.handleGet(ctx, cmd, args[1:])
	case "list":
		return h.handleList(ctx, cmd, args[1:])
	default:
		return "Format perintah salah. Gunakan `!iris help` untuk melihat daftar perintah.", nil
	}
}

func (h *SettingsHandler) handleSet(ctx context.Context, cmd *CommandContext, args []string) (string, error) {
	if len(args) != 2 {
		return "Format perintah salah. Gunakan `!iris help` untuk melihat daftar perintah.", nil
	}

	key := args[0]
	value := args[1]

	if !whitelistedKeys[key] {
		return fmt.Sprintf("Kunci konfigurasi tidak dikenali: `%s`. Lihat `!iris help`.", key), nil
	}

	if err := h.store.Set(ctx, cmd.GuildID, key, value); err != nil {
		return "", err
	}

	h.audit.Log(ctx, cmd.GuildID, cmd.UserID, "config_updated", "guild_settings", key, map[string]interface{}{
		"key":   key,
		"value": value,
	})

	return fmt.Sprintf("Konfigurasi `%s` telah diperbarui.", key), nil
}

func (h *SettingsHandler) handleGet(ctx context.Context, cmd *CommandContext, args []string) (string, error) {
	if len(args) != 1 {
		return "Format perintah salah. Gunakan `!iris help` untuk melihat daftar perintah.", nil
	}

	key := args[0]
	value, found, err := h.store.Get(ctx, cmd.GuildID, key)
	if err != nil {
		return "", err
	}

	if !found {
		return fmt.Sprintf("Konfigurasi tidak ditemukan: `%s`.", key), nil
	}

	return fmt.Sprintf("%s: %s", key, value), nil
}

func (h *SettingsHandler) handleList(ctx context.Context, cmd *CommandContext, args []string) (string, error) {
	settings, err := h.store.List(ctx, cmd.GuildID)
	if err != nil {
		return "", err
	}

	if len(settings) == 0 {
		return "Tidak ada konfigurasi yang terdaftar.", nil
	}

	var sb strings.Builder
	sb.WriteString("Konfigurasi guild:\n")
	for k, v := range settings {
		sb.WriteString(fmt.Sprintf("- `%s`: %s\n", k, v))
	}

	return sb.String(), nil
}

type RatelimitHandler struct {
	store SettingsStore
	audit AuditLogger
}

func NewRatelimitHandler(store SettingsStore, audit AuditLogger) *RatelimitHandler {
	return &RatelimitHandler{
		store: store,
		audit: audit,
	}
}

func (h *RatelimitHandler) Handle(ctx context.Context, cmd *CommandContext, args []string) (string, error) {
	if len(args) == 0 {
		return "Format perintah salah. Gunakan `!iris help` untuk melihat daftar perintah.", nil
	}

	sub := args[0]

	switch sub {
	case "set":
		return h.handleSet(ctx, cmd, args[1:])
	case "get":
		return h.handleGet(ctx, cmd, args[1:])
	default:
		return "Format perintah salah. Gunakan `!iris help` untuk melihat daftar perintah.", nil
	}
}

func (h *RatelimitHandler) handleSet(ctx context.Context, cmd *CommandContext, args []string) (string, error) {
	if len(args) != 2 {
		return "Format perintah salah. Gunakan `!iris help` untuk melihat daftar perintah.", nil
	}

	scope := args[0]
	limitStr := args[1]

	if scope != "user" && scope != "guild" {
		return "Scope tidak valid. Gunakan `user` atau `guild`.", nil
	}

	limit, err := strconv.Atoi(limitStr)
	if err != nil {
		return "Limit harus berupa angka.", nil
	}

	if limit <= 0 {
		return "Limit harus lebih besar dari 0.", nil
	}

	key := fmt.Sprintf("ratelimit_%s_per_min", scope)
	if err := h.store.Set(ctx, cmd.GuildID, key, limitStr); err != nil {
		return "", err
	}

	h.audit.Log(ctx, cmd.GuildID, cmd.UserID, "ratelimit_updated", "ratelimit", scope, map[string]interface{}{
		"scope": scope,
		"limit": limit,
	})

	return fmt.Sprintf("Rate limit untuk `%s` telah diperbarui menjadi `%d` permintaan per menit.", scope, limit), nil
}

func (h *RatelimitHandler) handleGet(ctx context.Context, cmd *CommandContext, args []string) (string, error) {
	settings, err := h.store.List(ctx, cmd.GuildID)
	if err != nil {
		return "", err
	}

	userLimit, _ := settings["ratelimit_user_per_min"]
	guildLimit, _ := settings["ratelimit_guild_per_min"]

	if userLimit == "" {
		userLimit = "default"
	}
	if guildLimit == "" {
		guildLimit = "default"
	}

	return fmt.Sprintf("Rate limit per user: %s\nRate limit per guild: %s", userLimit, guildLimit), nil
}
