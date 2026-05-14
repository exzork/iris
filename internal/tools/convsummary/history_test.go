package convsummary

import (
	"context"
	"testing"
	"time"
)

func TestInMemoryHistoryGuildScope(t *testing.T) {
	h := NewInMemoryHistory()

	msg1 := Message{UserID: 1, Username: "alice", Content: "hello", CreatedAt: time.Now()}
	msg2 := Message{UserID: 2, Username: "bob", Content: "world", CreatedAt: time.Now().Add(1 * time.Second)}

	h.Add(100, 200, msg1)
	h.Add(100, 201, msg2)

	msgs, err := h.Fetch(context.Background(), 100, 200, 10)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}

	if len(msgs) != 1 {
		t.Errorf("expected 1 message for channel 200, got %d", len(msgs))
	}

	if msgs[0].Content != "hello" {
		t.Errorf("expected 'hello', got '%s'", msgs[0].Content)
	}

	msgs, err = h.Fetch(context.Background(), 100, 201, 10)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}

	if len(msgs) != 1 {
		t.Errorf("expected 1 message for channel 201, got %d", len(msgs))
	}

	if msgs[0].Content != "world" {
		t.Errorf("expected 'world', got '%s'", msgs[0].Content)
	}
}

func TestFetchRespectsLimit(t *testing.T) {
	h := NewInMemoryHistory()

	for i := 0; i < 10; i++ {
		msg := Message{
			UserID:    int64(i),
			Username:  "user",
			Content:   "msg",
			CreatedAt: time.Now().Add(time.Duration(i) * time.Second),
		}
		h.Add(100, 200, msg)
	}

	msgs, err := h.Fetch(context.Background(), 100, 200, 5)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}

	if len(msgs) != 5 {
		t.Errorf("expected 5 messages, got %d", len(msgs))
	}

	if msgs[0].UserID != 5 {
		t.Errorf("expected first message to be from user 5 (last 5), got %d", msgs[0].UserID)
	}
}
