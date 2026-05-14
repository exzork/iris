package convsummary

import (
	"context"
)

// HistoryStore fetches conversation messages from storage.
type HistoryStore interface {
	Fetch(ctx context.Context, guildID, channelID int64, limit int) ([]Message, error)
}

// InMemoryHistory stores messages in memory, scoped by guild and channel.
type InMemoryHistory struct {
	messages map[int64]map[int64][]Message
}

// NewInMemoryHistory creates a new in-memory history store.
func NewInMemoryHistory() *InMemoryHistory {
	return &InMemoryHistory{
		messages: make(map[int64]map[int64][]Message),
	}
}

// Add stores a message in the history.
func (h *InMemoryHistory) Add(guildID, channelID int64, msg Message) {
	if h.messages[guildID] == nil {
		h.messages[guildID] = make(map[int64][]Message)
	}
	h.messages[guildID][channelID] = append(h.messages[guildID][channelID], msg)
}

// Fetch retrieves messages for a guild/channel, respecting the limit.
func (h *InMemoryHistory) Fetch(ctx context.Context, guildID, channelID int64, limit int) ([]Message, error) {
	if h.messages[guildID] == nil {
		return []Message{}, nil
	}

	msgs := h.messages[guildID][channelID]
	if len(msgs) == 0 {
		return []Message{}, nil
	}

	if limit <= 0 || limit > len(msgs) {
		limit = len(msgs)
	}

	// Return the last `limit` messages
	return msgs[len(msgs)-limit:], nil
}
