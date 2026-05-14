package wire

import (
	"context"

	"github.com/eko/iris-bot/internal/repository"
)

// LoreSettingsStoreAdapter adapts the repository.LoreGuildSettingsRepo to the lorethread.GuildSettingsStore interface.
type LoreSettingsStoreAdapter struct {
	repo *repository.LoreGuildSettingsRepo
}

// NewLoreSettingsStoreAdapter creates a new LoreSettingsStoreAdapter.
func NewLoreSettingsStoreAdapter(repo *repository.LoreGuildSettingsRepo) *LoreSettingsStoreAdapter {
	return &LoreSettingsStoreAdapter{repo: repo}
}

func (a *LoreSettingsStoreAdapter) GetLoreThreadEnabled(ctx context.Context, guildID int64) (bool, error) {
	return a.repo.IsEnabled(ctx, guildID)
}

func (a *LoreSettingsStoreAdapter) SetLoreThreadEnabled(ctx context.Context, guildID int64, enabled bool) error {
	return a.repo.SetEnabled(ctx, guildID, enabled)
}
