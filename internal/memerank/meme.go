package memerank

import "time"

type Meme struct {
	ID         int64
	GuildID    int64
	MessageID  int64
	ChannelID  int64
	URL        string
	Caption    string
	UploaderID int64
	Score      int       // up - down
	CreatedAt  time.Time
	Unsafe     bool      // if true, cannot be ranked as approved (never appear in top)
}

type Reaction struct {
	ID        int64
	GuildID   int64
	MemeID    int64
	UserID    int64
	Kind      ReactionKind
	CreatedAt time.Time
}

type ReactionKind string

const (
	KindUp   ReactionKind = "up"
	KindDown ReactionKind = "down"
)
