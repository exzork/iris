package bootstrap

import (
	"context"
	"strings"

	"github.com/eko/iris-bot/internal/domain"
	"github.com/eko/iris-bot/internal/settings"
)

type GuildStore interface {
	Upsert(ctx context.Context, g *domain.Guild) error
	Get(ctx context.Context, id int64) (*domain.Guild, error)
}

type SettingsStore interface {
	Get(ctx context.Context, guildID int64, key string) (string, bool, error)
	Set(ctx context.Context, guildID int64, key, value string) error
	List(ctx context.Context, guildID int64) (map[string]string, error)
}

type Bootstrapper struct {
	Guilds   GuildStore
	Settings SettingsStore
}

type Result struct {
	GuildCreated  bool
	SettingsAdded []settings.Key
	AdminsSeeded  int
	Idempotent    bool
}

func AllDefaultKeys() []settings.Key {
	return []settings.Key{
		settings.KeyDefaultLocale,
		settings.KeyMemoryEnabled,
		settings.KeyLoreCitationsRequired,
		settings.KeyMaxResponseChars,
		settings.KeyImageCooldownSec,
		settings.KeyMemesEnabled,
		settings.KeyRemindersEnabled,
		settings.KeyRateLimitUserPerMin,
		settings.KeyRateLimitGuildPerMin,
	}
}

func (b *Bootstrapper) Seed(ctx context.Context, guildID int64, adminRoleIDs string) (*Result, error) {
	result := &Result{
		SettingsAdded: []settings.Key{},
		AdminsSeeded:  0,
	}

	guild, err := b.Guilds.Get(ctx, guildID)
	if err != nil {
		return nil, err
	}

	if guild == nil {
		err := b.Guilds.Upsert(ctx, &domain.Guild{ID: guildID})
		if err != nil {
			return nil, err
		}
		result.GuildCreated = true
	}

	for _, key := range AllDefaultKeys() {
		_, found, err := b.Settings.Get(ctx, guildID, string(key))
		if err != nil {
			return nil, err
		}

		if !found {
			defaultVal, ok := settings.DefaultValue(key)
			if ok {
				err := b.Settings.Set(ctx, guildID, string(key), defaultVal)
				if err != nil {
					return nil, err
				}
				result.SettingsAdded = append(result.SettingsAdded, key)
			}
		}
	}

	if adminRoleIDs != "" {
		_, found, err := b.Settings.Get(ctx, guildID, string(settings.KeyAdminRoleIDs))
		if err != nil {
			return nil, err
		}

		if !found {
			err := b.Settings.Set(ctx, guildID, string(settings.KeyAdminRoleIDs), adminRoleIDs)
			if err != nil {
				return nil, err
			}
			result.AdminsSeeded = strings.Count(adminRoleIDs, ",") + 1
		}
	}

	result.Idempotent = !result.GuildCreated && len(result.SettingsAdded) == 0 && result.AdminsSeeded == 0

	return result, nil
}
