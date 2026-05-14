package memory

import (
	"context"
	"testing"
	"time"

	"github.com/eko/iris-bot/internal/domain"
)

func TestBoundedQueueEnqueueNonBlocking(t *testing.T) {
	q := NewBoundedQueue(2)
	defer q.Close()

	job := &EmbeddingJob{
		GuildID:   123,
		MessageID: 456,
		Content:   "test",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := q.Enqueue(ctx, job)
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	stats := q.Stats()
	if stats.Current != 1 {
		t.Errorf("Expected 1 job in queue, got %d", stats.Current)
	}
}

func TestNonBlockingCaptureDoesNotWaitForEmbedding(t *testing.T) {
	q := NewBoundedQueue(10)
	defer q.Close()

	adapter := NewNonBlockingCaptureAdapter(q)

	msg := &domain.ChannelMessage{
		GuildID:   123,
		ChannelID: 456,
		MessageID: 789,
		UserID:    111,
		Content:   "test message",
		IsBot:     false,
		CreatedAt: time.Now(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := adapter.Capture(ctx, msg)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Capture failed: %v", err)
	}

	if elapsed > 30*time.Millisecond {
		t.Errorf("Capture took too long: %v (should be <30ms)", elapsed)
	}

	stats := q.Stats()
	if stats.Current != 1 {
		t.Errorf("Expected 1 job enqueued, got %d", stats.Current)
	}
}

func TestQueueFullReturnsError(t *testing.T) {
	q := NewBoundedQueue(1)
	defer q.Close()

	job1 := &EmbeddingJob{GuildID: 1, MessageID: 1, Content: "msg1"}
	job2 := &EmbeddingJob{GuildID: 1, MessageID: 2, Content: "msg2"}
	job3 := &EmbeddingJob{GuildID: 1, MessageID: 3, Content: "msg3"}

	ctx := context.Background()

	if err := q.Enqueue(ctx, job1); err != nil {
		t.Fatalf("First enqueue failed: %v", err)
	}

	if err := q.Enqueue(ctx, job2); err != ErrQueueFull {
		t.Errorf("Expected ErrQueueFull, got %v", err)
	}

	stats := q.Stats()
	if stats.Dropped != 1 {
		t.Errorf("Expected 1 dropped job, got %d", stats.Dropped)
	}

	if err := q.Enqueue(ctx, job3); err != ErrQueueFull {
		t.Errorf("Expected ErrQueueFull for third job, got %v", err)
	}

	if stats := q.Stats(); stats.Dropped != 2 {
		t.Errorf("Expected 2 dropped jobs total, got %d", stats.Dropped)
	}
}

func TestQueueOverflowIsGraceful(t *testing.T) {
	q := NewBoundedQueue(2)
	defer q.Close()

	adapter := NewNonBlockingCaptureAdapter(q)

	msg := &domain.ChannelMessage{
		GuildID:   123,
		ChannelID: 456,
		MessageID: 789,
		UserID:    111,
		Content:   "test",
		IsBot:     false,
		CreatedAt: time.Now(),
	}

	ctx := context.Background()

	for i := 0; i < 5; i++ {
		msg.MessageID = int64(789 + i)
		err := adapter.Capture(ctx, msg)
		if err != nil {
			t.Fatalf("Capture %d failed: %v", i, err)
		}
	}

	stats := q.Stats()
	if stats.Dropped == 0 {
		t.Errorf("Expected some dropped jobs, got %d", stats.Dropped)
	}

	if stats.Enqueued+stats.Dropped != 5 {
		t.Errorf("Expected 5 total attempts, got enqueued=%d + dropped=%d", stats.Enqueued, stats.Dropped)
	}
}

func TestDequeueBlocksUntilJobAvailable(t *testing.T) {
	q := NewBoundedQueue(10)
	defer q.Close()

	job := &EmbeddingJob{GuildID: 1, MessageID: 1, Content: "test"}

	ctx := context.Background()

	go func() {
		time.Sleep(50 * time.Millisecond)
		q.Enqueue(ctx, job)
	}()

	start := time.Now()
	dequeued, err := q.Dequeue(ctx)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Dequeue failed: %v", err)
	}

	if dequeued == nil {
		t.Fatal("Dequeued job is nil")
	}

	if elapsed < 40*time.Millisecond {
		t.Errorf("Dequeue returned too quickly: %v (expected ~50ms)", elapsed)
	}

	if dequeued.MessageID != 1 {
		t.Errorf("Expected message ID 1, got %d", dequeued.MessageID)
	}
}

func TestDequeueRespectsContextCancellation(t *testing.T) {
	q := NewBoundedQueue(10)
	defer q.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := q.Dequeue(ctx)
	if err != context.DeadlineExceeded {
		t.Errorf("Expected context.DeadlineExceeded, got %v", err)
	}
}

func TestQueueStatsAccuracy(t *testing.T) {
	q := NewBoundedQueue(5)
	defer q.Close()

	ctx := context.Background()

	for i := 0; i < 3; i++ {
		job := &EmbeddingJob{GuildID: 1, MessageID: int64(i), Content: "msg"}
		q.Enqueue(ctx, job)
	}

	for i := 0; i < 2; i++ {
		q.Dequeue(ctx)
	}

	stats := q.Stats()
	if stats.Enqueued != 3 {
		t.Errorf("Expected 3 enqueued, got %d", stats.Enqueued)
	}
	if stats.Dequeued != 2 {
		t.Errorf("Expected 2 dequeued, got %d", stats.Dequeued)
	}
	if stats.Current != 1 {
		t.Errorf("Expected 1 current, got %d", stats.Current)
	}
	if stats.Capacity != 5 {
		t.Errorf("Expected capacity 5, got %d", stats.Capacity)
	}
}
