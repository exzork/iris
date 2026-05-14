package wire

import (
	"context"

	"github.com/eko/iris-bot/internal/domain"
	"github.com/eko/iris-bot/internal/repository"
)

type LoreAnchorResolverAdapter struct {
	Repo *repository.LoreThreadAnchorRepo
}

func (a *LoreAnchorResolverAdapter) GetByThread(ctx context.Context, guildID int64, threadID int64) (*domain.LoreThreadAnchor, error) {
	if a.Repo == nil {
		return nil, nil
	}
	return a.Repo.GetByThread(ctx, guildID, threadID)
}
