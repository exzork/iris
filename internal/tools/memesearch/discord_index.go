package memesearch

import (
	"context"
	"strings"
)

type DiscordMediaIndex interface {
	Search(ctx context.Context, guildID int64, query string, limit int) ([]MediaItem, error)
}

// InMemoryDiscordIndex is a test helper that stores messages in memory.
type InMemoryDiscordIndex struct {
	messages []struct {
		guildID int64
		caption string
		url     string
		mimeType string
	}
}

func NewInMemoryDiscordIndex() *InMemoryDiscordIndex {
	return &InMemoryDiscordIndex{
		messages: []struct {
			guildID int64
			caption string
			url     string
			mimeType string
		}{},
	}
}

// AddMessage adds a message to the index.
func (i *InMemoryDiscordIndex) AddMessage(guildID int64, caption, url, mimeType string) {
	i.messages = append(i.messages, struct {
		guildID int64
		caption string
		url     string
		mimeType string
	}{guildID, caption, url, mimeType})
}

// Search searches for messages matching the query in the given guild.
func (i *InMemoryDiscordIndex) Search(ctx context.Context, guildID int64, query string, limit int) ([]MediaItem, error) {
	var results []MediaItem
	queryLower := strings.ToLower(query)

	for _, msg := range i.messages {
		if msg.guildID != guildID {
			continue
		}

		if strings.Contains(strings.ToLower(msg.caption), queryLower) {
			results = append(results, MediaItem{
				URL:      msg.url,
				Source:   SourceDiscordHistory,
				MimeType: msg.mimeType,
				Caption:  msg.caption,
				Safety:   SafetyUnknown,
			})

			if len(results) >= limit {
				break
			}
		}
	}

	return results, nil
}
