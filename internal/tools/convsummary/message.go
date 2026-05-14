package convsummary

import "time"

// Message represents a Discord message in conversation history.
type Message struct {
	UserID    int64
	Username  string
	Content   string
	CreatedAt time.Time
}
