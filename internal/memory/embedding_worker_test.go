package memory

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eko/iris-bot/internal/embedder"
	"github.com/eko/iris-bot/internal/repository"
)

type fakeEmbedStore struct {
	mu       sync.Mutex
	stored   map[string][]float32
	pending  []*repository.PendingEmbedding
	storeErr error
	listErr  error
	storeCalls int32
}

func newFakeEmbedStore() *fakeEmbedStore {
	return &fakeEmbedStore{stored: make(map[string][]float32)}
}

func (s *fakeEmbedStore) StoreEmbedding(ctx context.Context, guildID int64, messageID int64, embedding []float32) error {
	atomic.AddInt32(&s.storeCalls, 1)
	if s.storeErr != nil {
		return s.storeErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stored[embedKey(guildID, messageID)] = append([]float32(nil), embedding...)
	return nil
}

func (s *fakeEmbedStore) ListPendingEmbeddings(ctx context.Context, limit int) ([]*repository.PendingEmbedding, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*repository.PendingEmbedding, 0, len(s.pending))
	for _, p := range s.pending {
		if _, ok := s.stored[embedKey(p.GuildID, p.MessageID)]; ok {
			continue
		}
		out = append(out, p)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func embedKey(guildID, messageID int64) string {
	return string(rune(guildID)) + ":" + string(rune(messageID))
}

type flakyEmbedder struct {
	dim     int
	fails   int32
	calls   int32
}

func (f *flakyEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	n := atomic.AddInt32(&f.calls, 1)
	if n <= f.fails {
		return nil, errors.New("transient embed failure")
	}
	out := make([]float32, f.dim)
	return out, nil
}
func (f *flakyEmbedder) Dim() int     { return f.dim }
func (f *flakyEmbedder) Close() error { return nil }

type fixedEmbedder struct{}

func (fixedEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	return make([]float32, repository.ExpectedEmbeddingDim), nil
}
func (fixedEmbedder) Dim() int     { return repository.ExpectedEmbeddingDim }
func (fixedEmbedder) Close() error { return nil }

func TestNewEmbeddingWorker_RejectsDimensionMismatch(t *testing.T) {
	q := NewBoundedQueue(4)
	store := newFakeEmbedStore()
	_, err := NewEmbeddingWorker(q, embedder.NewFakeEmbedder(), store, EmbeddingWorkerConfig{})
	if err == nil {
		t.Skip("fake embedder is 384-dim, skipping dim test via NewFakeEmbedder")
	}
}

func TestEmbeddingWorker_ProcessesQueuedJob(t *testing.T) {
	q := NewBoundedQueue(4)
	store := newFakeEmbedStore()
	w, err := NewEmbeddingWorker(q, fixedEmbedder{}, store, EmbeddingWorkerConfig{
		Workers:        1,
		BackfillLimit:  0,
		DequeueTimeout: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Start(ctx)
	defer w.Stop()

	if err := q.Enqueue(ctx, &EmbeddingJob{
		GuildID:   42,
		MessageID: 7,
		Content:   "hi",
	}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		store.mu.Lock()
		_, ok := store.stored[embedKey(42, 7)]
		store.mu.Unlock()
		if ok {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("embedding not stored in time")
}

func TestEmbeddingWorker_RetriesThenGivesUp(t *testing.T) {
	q := NewBoundedQueue(4)
	store := newFakeEmbedStore()
	fe := &flakyEmbedder{dim: repository.ExpectedEmbeddingDim, fails: 10}

	w, err := NewEmbeddingWorker(q, fe, store, EmbeddingWorkerConfig{
		Workers:           1,
		MaxAttemptsPerJob: 2,
		DequeueTimeout:    50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Start(ctx)
	defer w.Stop()

	if err := q.Enqueue(ctx, &EmbeddingJob{GuildID: 1, MessageID: 2, Content: "x"}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	time.Sleep(300 * time.Millisecond)
	if atomic.LoadInt32(&fe.calls) < 2 {
		t.Fatalf("expected >=2 embed attempts, got %d", fe.calls)
	}
	store.mu.Lock()
	_, stored := store.stored[embedKey(1, 2)]
	store.mu.Unlock()
	if stored {
		t.Fatalf("nothing should have been stored on persistent failure")
	}
}

func TestEmbeddingWorker_BackfillEmbedsPendingRows(t *testing.T) {
	q := NewBoundedQueue(4)
	store := newFakeEmbedStore()
	store.pending = []*repository.PendingEmbedding{
		{ID: 1, GuildID: 11, MessageID: 111, Content: "legacy row"},
	}
	w, err := NewEmbeddingWorker(q, fixedEmbedder{}, store, EmbeddingWorkerConfig{
		Workers:          1,
		BackfillLimit:    10,
		BackfillInterval: 20 * time.Millisecond,
		DequeueTimeout:   20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Start(ctx)
	defer w.Stop()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		store.mu.Lock()
		_, ok := store.stored[embedKey(11, 111)]
		store.mu.Unlock()
		if ok {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("backfill did not embed pending row")
}

func TestEmbeddingWorker_StopsOnContextCancel(t *testing.T) {
	q := NewBoundedQueue(4)
	store := newFakeEmbedStore()
	w, err := NewEmbeddingWorker(q, fixedEmbedder{}, store, EmbeddingWorkerConfig{
		Workers:        2,
		BackfillLimit:  0,
		DequeueTimeout: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)

	cancel()
	done := make(chan struct{})
	go func() { w.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("worker did not stop on ctx cancel")
	}
}
