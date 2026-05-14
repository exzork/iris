package slash

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/eko/iris-bot/internal/admin"
)

// NewNativeCommands returns the hand-written slash surface equivalent to the
// old `!iris ...` commands. They wrap the same admin.*Handler business
// logic, just translating slash options into the `args []string` shape the
// handlers already know how to parse.
func NewNativeCommands(
	exception *admin.ExceptionHandler,
	allowed *admin.AllowedChannelHandler,
	settings *admin.SettingsHandler,
	ratelimit *admin.RatelimitHandler,
	loreSettings *LoreSettingsHandler,
) []NativeCommand {
	cmds := []NativeCommand{
		newExceptionCommand(exception),
		newAllowedCommand(allowed),
		newHelpCommand(),
	}
	if settings != nil {
		cmds = append(cmds, newSettingsCommand(settings))
	}
	if ratelimit != nil {
		cmds = append(cmds, newRatelimitCommand(ratelimit))
	}
	if loreSettings != nil {
		cmds = append(cmds, newLoreSettingsCommand(loreSettings))
	}
	return cmds
}

func newExceptionCommand(h *admin.ExceptionHandler) NativeCommand {
	return NativeCommand{
		Name:        "iris-exception",
		Description: "Kelola pengecualian channel (admin only).",
		AdminOnly:   true,
		Options: []*discordgo.ApplicationCommandOption{
			{
				Name:        "add",
				Description: "Tambahkan channel ke daftar pengecualian.",
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Options: []*discordgo.ApplicationCommandOption{
					{Name: "channel_id", Description: "ID channel", Type: discordgo.ApplicationCommandOptionString, Required: true},
				},
			},
			{
				Name:        "remove",
				Description: "Hapus channel dari daftar pengecualian.",
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Options: []*discordgo.ApplicationCommandOption{
					{Name: "channel_id", Description: "ID channel", Type: discordgo.ApplicationCommandOptionString, Required: true},
				},
			},
			{
				Name:        "list",
				Description: "Lihat daftar pengecualian.",
				Type:        discordgo.ApplicationCommandOptionSubCommand,
			},
		},
		Execute: func(ctx context.Context, inv *NativeInvocation) (string, error) {
			args := subArgs(inv.Options)
			cmdCtx := &admin.CommandContext{GuildID: inv.GuildID, UserID: inv.UserID, ChannelID: inv.ChannelID}
			return h.Handle(ctx, cmdCtx, args)
		},
	}
}

func newAllowedCommand(h *admin.AllowedChannelHandler) NativeCommand {
	return NativeCommand{
		Name:        "iris-allowed",
		Description: "Kelola channel yang diizinkan (admin only).",
		AdminOnly:   true,
		Options: []*discordgo.ApplicationCommandOption{
			{
				Name:        "add",
				Description: "Tambahkan channel ke daftar izin.",
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Options: []*discordgo.ApplicationCommandOption{
					{Name: "channel_id", Description: "ID channel", Type: discordgo.ApplicationCommandOptionString, Required: true},
				},
			},
			{
				Name:        "remove",
				Description: "Hapus channel dari daftar izin.",
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Options: []*discordgo.ApplicationCommandOption{
					{Name: "channel_id", Description: "ID channel", Type: discordgo.ApplicationCommandOptionString, Required: true},
				},
			},
			{
				Name:        "list",
				Description: "Lihat daftar channel yang diizinkan.",
				Type:        discordgo.ApplicationCommandOptionSubCommand,
			},
		},
		Execute: func(ctx context.Context, inv *NativeInvocation) (string, error) {
			args := subArgs(inv.Options)
			cmdCtx := &admin.CommandContext{GuildID: inv.GuildID, UserID: inv.UserID, ChannelID: inv.ChannelID}
			return h.Handle(ctx, cmdCtx, args)
		},
	}
}

func newSettingsCommand(h *admin.SettingsHandler) NativeCommand {
	return NativeCommand{
		Name:        "iris-config",
		Description: "Konfigurasi iris (admin only).",
		AdminOnly:   true,
		Options: []*discordgo.ApplicationCommandOption{
			{
				Name:        "set",
				Description: "Set nilai konfigurasi.",
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Options: []*discordgo.ApplicationCommandOption{
					{Name: "key", Description: "Kunci konfigurasi", Type: discordgo.ApplicationCommandOptionString, Required: true},
					{Name: "value", Description: "Nilai baru", Type: discordgo.ApplicationCommandOptionString, Required: true},
				},
			},
			{
				Name:        "get",
				Description: "Ambil nilai konfigurasi.",
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Options: []*discordgo.ApplicationCommandOption{
					{Name: "key", Description: "Kunci konfigurasi", Type: discordgo.ApplicationCommandOptionString, Required: true},
				},
			},
			{
				Name:        "list",
				Description: "Lihat daftar konfigurasi.",
				Type:        discordgo.ApplicationCommandOptionSubCommand,
			},
		},
		Execute: func(ctx context.Context, inv *NativeInvocation) (string, error) {
			args := subArgs(inv.Options)
			cmdCtx := &admin.CommandContext{GuildID: inv.GuildID, UserID: inv.UserID, ChannelID: inv.ChannelID}
			return h.Handle(ctx, cmdCtx, args)
		},
	}
}

func newRatelimitCommand(h *admin.RatelimitHandler) NativeCommand {
	return NativeCommand{
		Name:        "iris-ratelimit",
		Description: "Kelola rate limit (admin only).",
		AdminOnly:   true,
		Options: []*discordgo.ApplicationCommandOption{
			{
				Name:        "set",
				Description: "Set rate limit.",
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Options: []*discordgo.ApplicationCommandOption{
					{Name: "channel_id", Description: "ID channel", Type: discordgo.ApplicationCommandOptionString, Required: true},
					{Name: "rate", Description: "Rate per detik", Type: discordgo.ApplicationCommandOptionInteger, Required: true},
				},
			},
			{
				Name:        "get",
				Description: "Ambil rate limit channel.",
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Options: []*discordgo.ApplicationCommandOption{
					{Name: "channel_id", Description: "ID channel", Type: discordgo.ApplicationCommandOptionString, Required: true},
				},
			},
		},
		Execute: func(ctx context.Context, inv *NativeInvocation) (string, error) {
			args := subArgs(inv.Options)
			cmdCtx := &admin.CommandContext{GuildID: inv.GuildID, UserID: inv.UserID, ChannelID: inv.ChannelID}
			return h.Handle(ctx, cmdCtx, args)
		},
	}
}

func newHelpCommand() NativeCommand {
	return NativeCommand{
		Name:        "iris-help",
		Description: "Daftar perintah iris.",
		Execute: func(ctx context.Context, inv *NativeInvocation) (string, error) {
			return strings.Join([]string{
				"**Perintah iris:**",
				"• `/search query:<kata kunci>` - cari di web",
				"• `/iris-help` - lihat bantuan ini",
				"• `/iris-exception add|remove|list` - kelola pengecualian channel (admin)",
				"• `/iris-allowed add|remove|list` - kelola channel yang diizinkan (admin)",
				"• `/iris-config set|get|list` - konfigurasi bot (admin)",
				"• `/iris-ratelimit set|get` - rate limit channel (admin)",
				"",
				"Owner bot juga bisa minta iris `install mcp ...` untuk menambah MCP server baru.",
			}, "\n"), nil
		},
	}
}

// subArgs flattens a top-level subcommand option into the `args []string`
// shape the existing admin handlers expect: ["add", "123"] for
// `/iris-exception add channel_id:123`.
func subArgs(opts []*discordgo.ApplicationCommandInteractionDataOption) []string {
	if len(opts) == 0 {
		return nil
	}
	top := opts[0]
	out := []string{top.Name}
	for _, o := range top.Options {
		out = append(out, formatOption(o))
	}
	return out
}

func formatOption(opt *discordgo.ApplicationCommandInteractionDataOption) string {
	switch opt.Type {
	case discordgo.ApplicationCommandOptionInteger:
		return strconv.FormatInt(opt.IntValue(), 10)
	case discordgo.ApplicationCommandOptionBoolean:
		return strconv.FormatBool(opt.BoolValue())
	default:
		return fmt.Sprintf("%v", opt.Value)
	}
}
