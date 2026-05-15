package domain

import (
	"time"
)

// Guild represents a Discord server configuration.
type Guild struct {
	ID        int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ExceptionChannel represents a channel where the bot should not respond.
type ExceptionChannel struct {
	ID        int64
	GuildID   int64
	ChannelID int64
	CreatedAt time.Time
}

// GuildConfig holds per-guild settings.
type GuildConfig struct {
	ID              int64
	GuildID         int64
	SettingKey      string
	SettingValue    string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// MemoryRecord represents a stored memory entry for a guild.
type MemoryRecord struct {
	ID         int64
	GuildID    int64
	UserID     int64
	Content    string
	Embedding  []float32
	Similarity float64
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// ToolRequest represents a request to execute a tool.
type ToolRequest struct {
	ID          string
	GuildID     int64
	ToolName    string
	Arguments   map[string]interface{}
	CreatedAt   time.Time
}

// Validate ensures ToolRequest has required fields.
func (tr *ToolRequest) Validate() error {
	if tr.ID == "" {
		return ErrToolRequestMissingID
	}
	if tr.ToolName == "" {
		return ErrToolRequestMissingName
	}
	return nil
}

// ToolResult represents the result of a tool execution.
type ToolResult struct {
	ID        string
	ToolName  string
	Output    string
	Error     string
	CreatedAt time.Time
}

// Validate ensures ToolResult has required fields.
func (tr *ToolResult) Validate() error {
	if tr.ID == "" {
		return ErrToolResultMissingID
	}
	return nil
}

// LoreCitation represents a citation to Wuthering Waves lore.
type LoreCitation struct {
	ID        int64
	GuildID   int64
	Source    string
	Content   string
	URL       string
	CreatedAt time.Time
}

// ChannelConversation represents a per-channel conversation lock.
type ChannelConversation struct {
	ID             int64
	GuildID        int64
	ChannelID      int64
	LastBotReplyAt time.Time
	LockUntil      time.Time
	UpdatedAt      time.Time
}

// DiscordMessage represents a Discord message.
type DiscordMessage struct {
	ID               int64
	GuildID          int64
	ChannelID        int64
	UserID           int64
	AuthorName       *string
	Content          string
	Attachments      []interface{}
	AttachmentCount  int
	ReplyToMessageID *int64
	ReplyToChannelID *int64
	IsBot            bool
	CreatedAt        time.Time
}

// DiscordEvent represents a Discord event.
type DiscordEvent struct {
	Type                string
	GuildID             int64
	ChannelID           int64
	ThreadID            int64
	UserID              int64
	AuthorName          *string
	Message             *DiscordMessage
	ReplyToMessageID    *int64
	ReplyToChannelID    *int64
	IsBot               bool
	AttachmentCount     int
	CreatedAt           time.Time
}

// ChannelMessage represents a Discord message stored in rolling channel context.
type ChannelMessage struct {
	ID               int64
	GuildID          int64
	ChannelID        int64
	MessageID        int64
	UserID           int64
	AuthorName       *string
	Content          string
	AttachmentCount  int
	ReplyToMessageID *int64
	ReplyToChannelID *int64
	IsBot            bool
	TriggerSource    string
	CreatedAt        time.Time
	EditedAt         *time.Time
	DeletedAt        *time.Time
	ContentEmbedding []float32
}

// UserBehaviorProfile captures non-sensitive interaction hints learned for a
// user inside a single guild. Profiles never cross guilds; the same user has
// an independent profile in each server where Iris observes them.
type UserBehaviorProfile struct {
	ID                       int64
	GuildID                  int64
	UserID                   int64
	CommunicationStyle       string
	Formality                string
	ResponseLengthPreference string
	FormattingPreference     string
	RecurringTopics          []string
	EvidenceCount            int
	LastObservedAt           time.Time
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

// EpisodeMemory is an append-only stash-style memory row captured just
// before a message is compacted out of the live context window. Guild
// isolation is mandatory at the repository layer.
type EpisodeMemory struct {
	ID             int64
	GuildID        int64
	ChannelID      int64
	ThreadID       *int64
	ChannelName    string
	ThreadName     string
	UserID         int64
	AuthorName     string
	MessageID      int64
	Content        string
	TaggedLine     string
	Embedding      []float32
	EmbeddingModel string
	OccurredAt     time.Time
	ArchivedAt     time.Time
	DeletedAt      *time.Time
}

type LoreSession struct {
	ID                 int64
	GuildID            int64
	ChannelID          int64
	FirstLoreMessageID int64
	LastLoreMessageID  int64
	LastLoreMessageAt  time.Time
	IdleDeadline       time.Time
	Status             string
	Title              *string
	Summary            *string
	ThreadID           *int64
	SummaryMessageID   *int64
	RetryCount         int
	LastError          *string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type LoreThreadAnchor struct {
	ID               int64
	GuildID          int64
	ChannelID        int64
	ThreadID         int64
	SummaryMessageID *int64
	SummaryText      *string
	Title            *string
	SourceSessionID  *int64
	CreatedAt        time.Time
}

type LoreGuildSettings struct {
	GuildID          int64
	Enabled          bool
	ThreadCapPerHour int
	CreatedAt        time.Time
	UpdatedAt        time.Time
}
