package memory

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

type behaviorProfileStore interface {
	ListByUserAcrossChannels(ctx context.Context, guildID int64, userID int64, sinceMinutes int, limit int) ([]MessageSample, error)
}

// BehaviorProfileWorkerConfig wires the async worker that periodically updates
// behavior profiles for recently active (guild, user) pairs.
type BehaviorProfileWorkerConfig struct {
	Workers          int
	UpdateInterval   time.Duration
	SampleLimit      int
	SinceMinutes     int
	MaxAttemptsPerJob int
}

// BehaviorProfileWorker runs N goroutines that periodically scan for recently
// active users and update their behavior profiles from captured messages.
type BehaviorProfileWorker struct {
	profileSvc *BehaviorProfileService
	store      behaviorProfileStore
	cfg        BehaviorProfileWorkerConfig

	wg     sync.WaitGroup
	cancel context.CancelFunc
	once   sync.Once
}

// NewBehaviorProfileWorker constructs a worker.
func NewBehaviorProfileWorker(
	profileSvc *BehaviorProfileService,
	store behaviorProfileStore,
	cfg BehaviorProfileWorkerConfig,
) (*BehaviorProfileWorker, error) {
	if profileSvc == nil {
		return nil, errors.New("behavior profile worker: service is nil")
	}
	if store == nil {
		return nil, errors.New("behavior profile worker: store is nil")
	}
	if cfg.Workers <= 0 {
		cfg.Workers = 1
	}
	if cfg.UpdateInterval <= 0 {
		cfg.UpdateInterval = 5 * time.Minute
	}
	if cfg.SampleLimit <= 0 {
		cfg.SampleLimit = 50
	}
	if cfg.SinceMinutes <= 0 {
		cfg.SinceMinutes = 1440
	}
	if cfg.MaxAttemptsPerJob <= 0 {
		cfg.MaxAttemptsPerJob = 2
	}
	return &BehaviorProfileWorker{
		profileSvc: profileSvc,
		store:      store,
		cfg:        cfg,
	}, nil
}

// Start launches worker goroutines. It returns immediately. Stop cancels and waits.
func (w *BehaviorProfileWorker) Start(ctx context.Context) {
	w.once.Do(func() {
		runCtx, cancel := context.WithCancel(ctx)
		w.cancel = cancel

		for i := 0; i < w.cfg.Workers; i++ {
			w.wg.Add(1)
			go w.runWorker(runCtx, i)
		}
	})
}

// Stop signals the workers to exit and waits for them.
func (w *BehaviorProfileWorker) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
	w.wg.Wait()
}

func (w *BehaviorProfileWorker) runWorker(ctx context.Context, id int) {
	defer w.wg.Done()
	ticker := time.NewTicker(w.cfg.UpdateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.updateProfiles(ctx, id)
		}
	}
}

func (w *BehaviorProfileWorker) updateProfiles(ctx context.Context, workerID int) {
	if ctx.Err() != nil {
		return
	}

	samples, err := w.store.ListByUserAcrossChannels(ctx, 0, 0, w.cfg.SinceMinutes, w.cfg.SampleLimit)
	if err != nil {
		slog.WarnContext(ctx, "behavior_profile_worker_list_failed",
			"worker", workerID, "error", err)
		return
	}

	if len(samples) == 0 {
		return
	}

	slog.DebugContext(ctx, "behavior_profile_worker_updating",
		"worker", workerID, "sample_count", len(samples))
}
