package slash

import (
	"context"
	"fmt"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/eko/iris-bot/internal/domain"
)

type LoreSettingsRepo interface {
	IsEnabled(ctx context.Context, guildID int64) (bool, error)
	SetEnabled(ctx context.Context, guildID int64, enabled bool) error
	GetSettings(ctx context.Context, guildID int64) (*domain.LoreGuildSettings, error)
	SetThreadCapPerHour(ctx context.Context, guildID int64, cap int) error
}

type LoreSettings struct {
	GuildID           int64
	Enabled           bool
	ThreadCapPerHour  int
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type LoreSettingsHandler struct {
	repo LoreSettingsRepo
}

func NewLoreSettingsHandler(repo LoreSettingsRepo) *LoreSettingsHandler {
	return &LoreSettingsHandler{repo: repo}
}

func newLoreSettingsCommand(h *LoreSettingsHandler) NativeCommand {
	return NativeCommand{
		Name:        "iris-lore",
		Description: "Kelola pengaturan lore threads (admin only).",
		AdminOnly:   true,
		Options: []*discordgo.ApplicationCommandOption{
			{
				Name:        "enable",
				Description: "Aktifkan lore threads untuk guild ini.",
				Type:        discordgo.ApplicationCommandOptionSubCommand,
			},
			{
				Name:        "disable",
				Description: "Nonaktifkan lore threads untuk guild ini.",
				Type:        discordgo.ApplicationCommandOptionSubCommand,
			},
			{
				Name:        "status",
				Description: "Lihat status lore threads dan thread cap.",
				Type:        discordgo.ApplicationCommandOptionSubCommand,
			},
			{
				Name:        "cap",
				Description: "Set thread cap per jam (1-100).",
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Options: []*discordgo.ApplicationCommandOption{
					{
						Name:        "value",
						Description: "Jumlah thread per jam (1-100)",
						Type:        discordgo.ApplicationCommandOptionInteger,
						Required:    true,
						MinValue:    ptrFloat64(1),
						MaxValue:    100,
					},
				},
			},
		},
		Execute: func(ctx context.Context, inv *NativeInvocation) (string, error) {
			if !inv.IsAdmin {
				return "Mohon maaf, hanya admin server yang dapat mengubah pengaturan lore threads.", nil
			}

			if len(inv.Options) == 0 {
				return "Subcommand diperlukan: enable, disable, status, atau cap.", nil
			}

			subCmd := inv.Options[0].Name
			switch subCmd {
			case "enable":
				return h.handleEnable(ctx, inv.GuildID)
			case "disable":
				return h.handleDisable(ctx, inv.GuildID)
			case "status":
				return h.handleStatus(ctx, inv.GuildID)
			case "cap":
				return h.handleCap(ctx, inv.GuildID, inv.Options[0])
			default:
				return "Subcommand tidak dikenali.", nil
			}
		},
	}
}

func (h *LoreSettingsHandler) handleEnable(ctx context.Context, guildID int64) (string, error) {
	err := h.repo.SetEnabled(ctx, guildID, true)
	if err != nil {
		return "", fmt.Errorf("failed to enable lore threads: %w", err)
	}
	return "✅ Lore threads diaktifkan untuk guild ini.", nil
}

func (h *LoreSettingsHandler) handleDisable(ctx context.Context, guildID int64) (string, error) {
	err := h.repo.SetEnabled(ctx, guildID, false)
	if err != nil {
		return "", fmt.Errorf("failed to disable lore threads: %w", err)
	}
	return "✅ Lore threads dinonaktifkan untuk guild ini.", nil
}

func (h *LoreSettingsHandler) handleStatus(ctx context.Context, guildID int64) (string, error) {
	domainSettings, err := h.repo.GetSettings(ctx, guildID)
	if err != nil {
		return "", fmt.Errorf("failed to get lore settings: %w", err)
	}
	settings := &LoreSettings{
		GuildID:          domainSettings.GuildID,
		Enabled:          domainSettings.Enabled,
		ThreadCapPerHour: domainSettings.ThreadCapPerHour,
		CreatedAt:        domainSettings.CreatedAt,
		UpdatedAt:        domainSettings.UpdatedAt,
	}
	return formatLoreStatus(settings), nil
}

func (h *LoreSettingsHandler) handleCap(ctx context.Context, guildID int64, opt *discordgo.ApplicationCommandInteractionDataOption) (string, error) {
	if len(opt.Options) == 0 {
		return "Parameter 'value' diperlukan.", nil
	}

	val := opt.Options[0].IntValue()
	if val < 1 || val > 100 {
		return "Thread cap harus antara 1 dan 100.", nil
	}

	err := h.repo.SetThreadCapPerHour(ctx, guildID, int(val))
	if err != nil {
		return "", fmt.Errorf("failed to set thread cap: %w", err)
	}
	return fmt.Sprintf("✅ Thread cap diatur ke %d per jam.", val), nil
}

func ptrFloat64(v float64) *float64 {
	return &v
}
