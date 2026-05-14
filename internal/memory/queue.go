package memory

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/eko/iris-bot/internal/domain"
)

var (
	ErrQueueFull = errors.New("embedding queue full")
)

// EmbeddingJob represents a message to be embedded.
type EmbeddingJob struct {
	GuildID   int64
	ChannelID int64
	MessageID int64
	UserID    int64
	Content   string
}

// EmbeddingQueue defines the contract for async embedding job queueing.
type EmbeddingQueue interface {
	// Enqueue attempts to add a job to the queue without blocking.
	// Returns ErrQueueFull if the queue is at capacity.
	Enqueue(ctx context.Context, job *EmbeddingJob) error

	// Dequeue retrieves the next job from the queue, blocking until one is available or ctx is cancelled.
	Dequeue(ctx context.Context) (*EmbeddingJob, error)

	// Close stops accepting new jobs and signals workers to shut down.
	Close() error

	// Stats returns current queue statistics for monitoring.
	Stats() QueueStats
}

// QueueStats provides visibility into queue health.
type QueueStats struct {
	Enqueued int64
	Dequeued int64
	Dropped  int64
	Current  int
	Capacity int
}

// BoundedQueue is a bounded FIFO queue for embedding jobs with drop/error logging.
type BoundedQueue struct {
	mu       sync.Mutex
	ch       chan *EmbeddingJob
	capacity int
	closed   bool

	enqueued int64
	dequeued int64
	dropped  int64
}

// NewBoundedQueue creates a new bounded queue with the given capacity.
func NewBoundedQueue(capacity int) *BoundedQueue {
	if capacity <= 0 {
		capacity = 32 // default
	}
	return &BoundedQueue{
		ch:       make(chan *EmbeddingJob, capacity),
		capacity: capacity,
	}
}

// Enqueue attempts to add a job without blocking.
// If the queue is full, it logs and returns ErrQueueFull.
func (q *BoundedQueue) Enqueue(ctx context.Context, job *EmbeddingJob) error {
	if job == nil {
		return errors.New("nil job")
	}

	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return errors.New("queue closed")
	}
	q.mu.Unlock()

	select {
	case q.ch <- job:
		q.mu.Lock()
		q.enqueued++
		q.mu.Unlock()
		return nil
	default:
		q.mu.Lock()
		q.dropped++
		q.mu.Unlock()
		slog.WarnContext(ctx, "embedding_queue_full",
			"guild", job.GuildID,
			"message", job.MessageID,
			"capacity", q.capacity,
			"dropped_total", q.dropped,
		)
		return ErrQueueFull
	}
}

// Dequeue retrieves the next job, blocking until available or ctx is cancelled.
func (q *BoundedQueue) Dequeue(ctx context.Context) (*EmbeddingJob, error) {
	select {
	case job := <-q.ch:
		if job == nil {
			return nil, errors.New("queue closed")
		}
		q.mu.Lock()
		q.dequeued++
		q.mu.Unlock()
		return job, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Close closes the queue and prevents new enqueues.
func (q *BoundedQueue) Close() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return nil
	}
	q.closed = true
	close(q.ch)
	return nil
}

// Stats returns current queue statistics.
func (q *BoundedQueue) Stats() QueueStats {
	q.mu.Lock()
	defer q.mu.Unlock()
	return QueueStats{
		Enqueued: q.enqueued,
		Dequeued: q.dequeued,
		Dropped:  q.dropped,
		Current:  len(q.ch),
		Capacity: q.capacity,
	}
}

// NonBlockingCaptureAdapter wraps a ChannelCapture to enqueue embedding jobs asynchronously.
type NonBlockingCaptureAdapter struct {
	queue EmbeddingQueue
}

// NewNonBlockingCaptureAdapter creates a new adapter that enqueues jobs without blocking.
func NewNonBlockingCaptureAdapter(queue EmbeddingQueue) *NonBlockingCaptureAdapter {
	return &NonBlockingCaptureAdapter{queue: queue}
}

// Capture enqueues the message for async embedding without waiting.
// It implements the orchestrator.ChannelCapture interface.
func (a *NonBlockingCaptureAdapter) Capture(ctx context.Context, msg *domain.ChannelMessage) error {
	if msg == nil {
		return errors.New("nil message")
	}

	job := &EmbeddingJob{
		GuildID:   msg.GuildID,
		ChannelID: msg.ChannelID,
		MessageID: msg.MessageID,
		UserID:    msg.UserID,
		Content:   msg.Content,
	}

	// Enqueue with a short timeout to avoid blocking the hot path.
	// If the queue is full, we log and return; the message is still in the DB.
	enqueueCtx, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()

	if err := a.queue.Enqueue(enqueueCtx, job); err != nil {
		// Log but don't fail the capture; the message is already persisted.
		slog.DebugContext(ctx, "embedding_enqueue_failed",
			"guild", msg.GuildID,
			"message", msg.MessageID,
			"error", err,
		)
		return nil // Swallow the error; capture succeeded at DB level.
	}

	return nil
}
