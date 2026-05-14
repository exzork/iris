package convsummary

import (
	"github.com/eko/iris-bot/internal/memory"
)

// Redactor wraps memory.Redactor for pre-summary privacy scrubbing.
type Redactor struct {
	inner *memory.Redactor
}

// NewRedactor creates a new Redactor.
func NewRedactor() *Redactor {
	return &Redactor{inner: memory.NewRedactor()}
}

// RedactMessages applies redaction to all message contents.
func (r *Redactor) RedactMessages(msgs []Message) []Message {
	redacted := make([]Message, len(msgs))
	for i, msg := range msgs {
		redacted[i] = Message{
			UserID:    msg.UserID,
			Username:  msg.Username,
			Content:   r.inner.Redact(msg.Content),
			CreatedAt: msg.CreatedAt,
		}
	}
	return redacted
}
