package convsummary

import (
	"testing"
	"time"
)

func TestRedactMessagesSK(t *testing.T) {
	r := NewRedactor()

	msgs := []Message{
		{
			UserID:    1,
			Username:  "alice",
			Content:   "my key is sk-test-1234567890abcdefghij",
			CreatedAt: time.Now(),
		},
	}

	redacted := r.RedactMessages(msgs)

	if len(redacted) != 1 {
		t.Errorf("expected 1 message, got %d", len(redacted))
	}

	if redacted[0].Content == msgs[0].Content {
		t.Errorf("content should be redacted, but got: %s", redacted[0].Content)
	}

	if redacted[0].UserID != msgs[0].UserID {
		t.Errorf("UserID should not change")
	}

	if redacted[0].Username != msgs[0].Username {
		t.Errorf("Username should not change")
	}
}

func TestRedactMessagesLeavesCleanUntouched(t *testing.T) {
	r := NewRedactor()

	msgs := []Message{
		{
			UserID:    1,
			Username:  "bob",
			Content:   "just a normal message",
			CreatedAt: time.Now(),
		},
	}

	redacted := r.RedactMessages(msgs)

	if redacted[0].Content != msgs[0].Content {
		t.Errorf("clean content should not be redacted, got: %s", redacted[0].Content)
	}
}
