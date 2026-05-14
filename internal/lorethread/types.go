package lorethread

import "time"

// Message represents a Discord message in a lore session.
type Message struct {
	ID        int64
	GuildID   int64
	ChannelID int64
	AuthorID  int64
	AuthorIsBot bool
	Content   string
	CreatedAt time.Time
}

// Session represents a lore discussion session.
type Session struct {
	ID           int64
	GuildID      int64
	ChannelID    int64
	FirstLoreMessageID int64
	LastLoreMessageID  int64
	LastLoreMessageAt  time.Time
	IdleDeadline       time.Time
	Status             string
	RetryCount         int
	LastError          *string
	ThreadID           *int64
	SummaryMessageID   *int64
	Title              *string
	Summary            *string
	FirstMessage *Message
	Messages     []*Message
	CreatedAt    time.Time
	UpdatedAt    time.Time
	IsActive     bool
}

// ThreadAnchor stores persisted thread anchor metadata for lore sessions.
type ThreadAnchor struct {
	SessionID int64
	GuildID   int64
	ChannelID int64
	ThreadID  int64
	MessageID int64
	Title     string
	Summary   string
}

// ClassifyResult is the result of classifying a message as lore or non-lore.
type ClassifyResult struct {
	IsLore bool
	Reason string
}

// SummaryRequest is a request to summarize lore messages.
type SummaryRequest struct {
	GuildID  int64
	Messages []*Message
}

// SummaryResult is the result of summarizing lore messages.
type SummaryResult struct {
	Title   string
	Summary string
}

// ThreadCreateRequest is a request to create a Discord thread.
type ThreadCreateRequest struct {
	GuildID        int64
	ChannelID      int64
	ParentMessageID int64
	Title          string
	FirstMessage   string
}

// ThreadCreateResult is the result of creating a Discord thread.
type ThreadCreateResult struct {
	ThreadID  int64
	MessageID int64
}
