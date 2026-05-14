package wire

import (
	"context"

	"github.com/eko/iris-bot/internal/domain"
	"github.com/eko/iris-bot/internal/lorethread"
	"github.com/eko/iris-bot/internal/repository"
)

// LoreMessageFetcherAdapter adapts ChannelMessageRepo to lorethread.MessageFetcher.
type LoreMessageFetcherAdapter struct {
	repo *repository.ChannelMessageRepo
}

// NewLoreMessageFetcherAdapter creates a new LoreMessageFetcherAdapter.
func NewLoreMessageFetcherAdapter(repo *repository.ChannelMessageRepo) *LoreMessageFetcherAdapter {
	return &LoreMessageFetcherAdapter{repo: repo}
}

func (a *LoreMessageFetcherAdapter) FetchRecent(ctx context.Context, guildID, channelID int64, limit int) ([]*lorethread.Message, error) {
	rows, err := a.repo.ListRecent(ctx, guildID, channelID, limit)
	if err != nil {
		return nil, err
	}

	out := make([]*lorethread.Message, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		out = append(out, mapDomainMessageToLorethread(row))
	}
	return out, nil
}

func (a *LoreMessageFetcherAdapter) FetchByID(ctx context.Context, guildID, messageID int64) (*lorethread.Message, error) {
	msg, err := a.repo.GetByID(ctx, guildID, messageID)
	if err != nil {
		return nil, err
	}
	if msg == nil {
		return nil, nil
	}
	return mapDomainMessageToLorethread(msg), nil
}

func mapDomainMessageToLorethread(msg *domain.ChannelMessage) *lorethread.Message {
	return &lorethread.Message{
		ID:          msg.MessageID,
		GuildID:     msg.GuildID,
		ChannelID:   msg.ChannelID,
		AuthorID:    msg.UserID,
		AuthorIsBot: msg.IsBot,
		Content:     msg.Content,
		CreatedAt:   msg.CreatedAt,
	}
}
