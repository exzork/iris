package memory

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/eko/iris-bot/internal/embedder"
	"github.com/eko/iris-bot/internal/repository"
)

type embeddingStore interface {
	StoreEmbedding(ctx context.Context, guildID int64, messageID int64, embedding []float32) error
	ListPendingEmbeddings(ctx context.Context, limit int) ([]*repository.PendingEmbedding, error)
}

// EmbeddingWorkerConfig wires the async worker that drains EmbeddingQueue jobs
// and backfills any rows whose content_embedding is NULL. Workers <= 0 and
// BackfillLimit < 0 are coerced to safe defaults so misconfigured operators
// still get a functioning guild-isolated pipeline.
type EmbeddingWorkerConfig struct {
	Workers           int
	BackfillLimit     int
	BackfillInterval  time.Duration
	DequeueTimeout    time.Duration
	MaxAttemptsPerJob int
}

// EmbeddingWorker runs N goroutines that dequeue jobs and persist embeddings,
// plus a periodic backfill that embeds any rows that were captured without
// going through the queue (e.g. legacy rows or restarts).
type EmbeddingWorker struct {
	queue    EmbeddingQueue
	embedder embedder.Embedder
	store    embeddingStore
	cfg      EmbeddingWorkerConfig

	wg     sync.WaitGroup
	cancel context.CancelFunc
	once   sync.Once
}

// NewEmbeddingWorker constructs a worker. queue/embedder/store must be non-nil.
func NewEmbeddingWorker(
	queue EmbeddingQueue,
	embed embedder.Embedder,
	store embeddingStore,
	cfg EmbeddingWorkerConfig,
) (*EmbeddingWorker, error) {
	if queue == nil {
		return nil, errors.New("embedding worker: queue is nil")
	}
	if embed == nil {
		return nil, errors.New("embedding worker: embedder is nil")
	}
	if store == nil {
		return nil, errors.New("embedding worker: store is nil")
	}
	if embed.Dim() != repository.ExpectedEmbeddingDim {
		return nil, errors.New("embedding worker: embedder dimension mismatch")
	}
	if cfg.Workers <= 0 {
		cfg.Workers = 1
	}
	if cfg.BackfillLimit < 0 {
		cfg.BackfillLimit = 0
	}
	if cfg.BackfillInterval <= 0 {
		cfg.BackfillInterval = 30 * time.Second
	}
	if cfg.DequeueTimeout <= 0 {
		cfg.DequeueTimeout = 1 * time.Second
	}
	if cfg.MaxAttemptsPerJob <= 0 {
		cfg.MaxAttemptsPerJob = 3
	}
	return &EmbeddingWorker{
		queue:    queue,
		embedder: embed,
		store:    store,
		cfg:      cfg,
	}, nil
}

// Start launches worker goroutines and the backfill loop. It returns
// immediately. Stop cancels and waits.
func (w *EmbeddingWorker) Start(ctx context.Context) {
	w.once.Do(func() {
		runCtx, cancel := context.WithCancel(ctx)
		w.cancel = cancel

		for i := 0; i < w.cfg.Workers; i++ {
			w.wg.Add(1)
			go w.runWorker(runCtx, i)
		}
		if w.cfg.BackfillLimit > 0 {
			w.wg.Add(1)
			go w.runBackfill(runCtx)
		}
	})
}

// Stop signals the workers to exit and waits for them.
func (w *EmbeddingWorker) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
	w.wg.Wait()
}

func (w *EmbeddingWorker) runWorker(ctx context.Context, id int) {
	defer w.wg.Done()
	for {
		if ctx.Err() != nil {
			return
		}
		jobCtx, cancel := context.WithTimeout(ctx, w.cfg.DequeueTimeout)
		job, err := w.queue.Dequeue(jobCtx)
		cancel()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			continue
		}
		if job == nil {
			return
		}
		w.processJob(ctx, job, id)
	}
}

func (w *EmbeddingWorker) processJob(ctx context.Context, job *EmbeddingJob, workerID int) {
	if job.GuildID == 0 || job.MessageID == 0 {
		slog.WarnContext(ctx, "embedding_worker_skip_invalid_job",
			"guild", job.GuildID, "message", job.MessageID)
		return
	}
	if job.Content == "" {
		return
	}
	for attempt := 1; attempt <= w.cfg.MaxAttemptsPerJob; attempt++ {
		if ctx.Err() != nil {
			return
		}
		vec, err := w.embedder.Embed(ctx, job.Content)
		if err != nil {
			slog.WarnContext(ctx, "embedding_worker_embed_failed",
				"worker", workerID, "guild", job.GuildID, "message", job.MessageID,
				"attempt", attempt, "error", err)
			continue
		}
		if err := w.store.StoreEmbedding(ctx, job.GuildID, job.MessageID, vec); err != nil {
			slog.WarnContext(ctx, "embedding_worker_store_failed",
				"worker", workerID, "guild", job.GuildID, "message", job.MessageID,
				"attempt", attempt, "error", err)
			continue
		}
		return
	}
	slog.ErrorContext(ctx, "embedding_worker_gave_up",
		"guild", job.GuildID, "message", job.MessageID,
		"attempts", w.cfg.MaxAttemptsPerJob)
}

func (w *EmbeddingWorker) runBackfill(ctx context.Context) {
	defer w.wg.Done()
	ticker := time.NewTicker(w.cfg.BackfillInterval)
	defer ticker.Stop()

	w.backfillOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.backfillOnce(ctx)
		}
	}
}

func (w *EmbeddingWorker) backfillOnce(ctx context.Context) {
	if w.cfg.BackfillLimit <= 0 {
		return
	}
	pending, err := w.store.ListPendingEmbeddings(ctx, w.cfg.BackfillLimit)
	if err != nil {
		slog.WarnContext(ctx, "embedding_worker_backfill_list_failed", "error", err)
		return
	}
	for _, p := range pending {
		if ctx.Err() != nil {
			return
		}
		w.processJob(ctx, &EmbeddingJob{
			GuildID:   p.GuildID,
			MessageID: p.MessageID,
			Content:   p.Content,
		}, -1)
	}
}
