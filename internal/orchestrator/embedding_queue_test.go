package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/eko/iris-bot/internal/domain"
	"github.com/eko/iris-bot/internal/memory"
	"github.com/eko/iris-bot/internal/testutil"
)

func TestOrchestratorCaptureNonBlockingWithBlockingEmbedder(t *testing.T) {
	q := memory.NewBoundedQueue(10)
	defer q.Close()

	capture := memory.NewNonBlockingCaptureAdapter(q)

	msg := &domain.ChannelMessage{
		GuildID:   123,
		ChannelID: 456,
		MessageID: 789,
		UserID:    111,
		Content:   "test message",
		IsBot:     false,
		CreatedAt: time.Now(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := capture.Capture(ctx, msg)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Capture failed: %v", err)
	}

	if elapsed > 50*time.Millisecond {
		t.Errorf("Capture took too long: %v (should be <50ms)", elapsed)
	}

	stats := q.Stats()
	if stats.Current != 1 {
		t.Errorf("Expected 1 job in queue, got %d", stats.Current)
	}
}

func TestOrchestratorCaptureEnqueuesMultipleMessages(t *testing.T) {
	q := memory.NewBoundedQueue(100)
	defer q.Close()

	capture := memory.NewNonBlockingCaptureAdapter(q)

	ctx := context.Background()

	for i := range 10 {
		msg := &domain.ChannelMessage{
			GuildID:   123,
			ChannelID: 456,
			MessageID: int64(1000 + i),
			UserID:    111,
			Content:   "message " + string(rune(i)),
			IsBot:     false,
			CreatedAt: time.Now(),
		}

		if err := capture.Capture(ctx, msg); err != nil {
			t.Fatalf("Capture %d failed: %v", i, err)
		}
	}

	stats := q.Stats()
	if stats.Current != 10 {
		t.Errorf("Expected 10 jobs in queue, got %d", stats.Current)
	}
	if stats.Enqueued != 10 {
		t.Errorf("Expected 10 enqueued, got %d", stats.Enqueued)
	}
}

func TestOrchestratorCaptureHandlesQueueFullGracefully(t *testing.T) {
	q := memory.NewBoundedQueue(3)
	defer q.Close()

	capture := memory.NewNonBlockingCaptureAdapter(q)

	ctx := context.Background()

	for i := range 10 {
		msg := &domain.ChannelMessage{
			GuildID:   123,
			ChannelID: 456,
			MessageID: int64(1000 + i),
			UserID:    111,
			Content:   "message",
			IsBot:     false,
			CreatedAt: time.Now(),
		}

		err := capture.Capture(ctx, msg)
		if err != nil {
			t.Fatalf("Capture %d failed: %v", i, err)
		}
	}

	stats := q.Stats()
	if stats.Dropped == 0 {
		t.Errorf("Expected some dropped jobs, got %d", stats.Dropped)
	}

	if stats.Enqueued+stats.Dropped != 10 {
		t.Errorf("Expected 10 total attempts, got enqueued=%d + dropped=%d", stats.Enqueued, stats.Dropped)
	}
}

func TestCaptureWithFakeEmbedderBlocking(t *testing.T) {
	q := memory.NewBoundedQueue(10)
	defer q.Close()

	capture := memory.NewNonBlockingCaptureAdapter(q)

	fakeEmbedder := testutil.NewFakeEmbeddingClient()
	fakeEmbedder.SimulateLatency = 5 * time.Second

	msg := &domain.ChannelMessage{
		GuildID:   123,
		ChannelID: 456,
		MessageID: 789,
		UserID:    111,
		Content:   "test message",
		IsBot:     false,
		CreatedAt: time.Now(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := capture.Capture(ctx, msg)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Capture failed: %v", err)
	}

	if elapsed > 50*time.Millisecond {
		t.Errorf("Capture took too long even with blocking embedder: %v", elapsed)
	}

	stats := q.Stats()
	if stats.Current != 1 {
		t.Errorf("Expected job enqueued despite embedder latency, got %d in queue", stats.Current)
	}

	_ = fakeEmbedder
}

func TestQueueDequeueReturnsJobsInOrder(t *testing.T) {
	q := memory.NewBoundedQueue(10)
	defer q.Close()

	ctx := context.Background()

	for i := range 5 {
		job := &memory.EmbeddingJob{
			GuildID:   123,
			MessageID: int64(1000 + i),
			Content:   "msg",
		}
		q.Enqueue(ctx, job)
	}

	for i := range 5 {
		job, err := q.Dequeue(ctx)
		if err != nil {
			t.Fatalf("Dequeue %d failed: %v", i, err)
		}
		if job.MessageID != int64(1000+i) {
			t.Errorf("Expected message ID %d, got %d", 1000+i, job.MessageID)
		}
	}
}
