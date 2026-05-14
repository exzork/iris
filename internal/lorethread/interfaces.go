package lorethread

import (
	"context"
	"time"
)

// SessionStore manages lore session persistence.
type SessionStore interface {
	Create(ctx context.Context, session *Session) error
	GetByID(ctx context.Context, id int64) (*Session, error)
	GetActive(ctx context.Context, guildID, channelID int64) (*Session, error)
	Update(ctx context.Context, session *Session) error
	ListByGuild(ctx context.Context, guildID int64) ([]*Session, error)
}

// ThreadAnchorStore manages thread anchor messages (first message in a lore thread).
type ThreadAnchorStore interface {
	Create(ctx context.Context, sessionID, threadID, messageID int64) error
	GetBySessionID(ctx context.Context, sessionID int64) (threadID, messageID int64, err error)
	GetByThreadID(ctx context.Context, threadID int64) (sessionID int64, err error)
}

// GuildSettingsStore manages per-guild lore thread settings.
type GuildSettingsStore interface {
	GetLoreThreadEnabled(ctx context.Context, guildID int64) (bool, error)
	SetLoreThreadEnabled(ctx context.Context, guildID int64, enabled bool) error
}

// LoreClassifier classifies whether a message is lore-relevant.
type LoreClassifier interface {
	Classify(ctx context.Context, guildID int64, message *Message) (*ClassifyResult, error)
}

// LoreSummarizer summarizes a collection of lore messages.
type LoreSummarizer interface {
	Summarize(ctx context.Context, req *SummaryRequest) (*SummaryResult, error)
}

// TitleGenerator generates a title for a lore thread.
type TitleGenerator interface {
	Generate(ctx context.Context, guildID int64, messages []*Message) (string, error)
}

// ThreadCreator creates a Discord thread.
type ThreadCreator interface {
	Create(ctx context.Context, req *ThreadCreateRequest) (*ThreadCreateResult, error)
}

// MessageFetcher fetches messages from Discord.
type MessageFetcher interface {
	FetchRecent(ctx context.Context, guildID, channelID int64, limit int) ([]*Message, error)
	FetchByID(ctx context.Context, guildID, messageID int64) (*Message, error)
}

// Clock provides time operations for deterministic testing.
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

// Limiter enforces rate limits on lore thread operations.
type Limiter interface {
	Allow(ctx context.Context, guildID int64) bool
	Reset(ctx context.Context, guildID int64) error
}
