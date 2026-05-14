package ingest

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
)

func TestRunOnceIngestsBatch(t *testing.T) {
	client := &fakeClient{pages: []*Page{
		{ID: 1, Title: "Page 1", URL: "https://wiki/p1", Wikitext: "alpha"},
		{ID: 2, Title: "Page 2", URL: "https://wiki/p2", Wikitext: "beta"},
		{ID: 3, Title: "Page 3", URL: "https://wiki/p3", Wikitext: "gamma"},
	}}
	chunker := NewChunker(1000, 0)
	cursor := NewInMemoryCursorStore()
	dedupe := NewInMemoryDeduper()
	embedder := &fakeEmbedder{}
	store := newFakeStore()

	ingester := New(Config{
		Client:    client,
		Chunker:   chunker,
		Cursor:    cursor,
		Dedupe:    dedupe,
		Embedder:  embedder,
		Store:     store,
		SourceID:  "wiki-src",
		BatchSize: 2,
	})

	stats1, err := ingester.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce() first error = %v", err)
	}
	if stats1.PagesFetched != 2 || stats1.ChunksInserted != 2 || stats1.Errors != 0 || stats1.LastID != 2 {
		t.Fatalf("unexpected first stats: %+v", stats1)
	}

	stats2, err := ingester.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce() second error = %v", err)
	}
	if stats2.PagesFetched != 1 || stats2.ChunksInserted != 1 || stats2.Errors != 0 || stats2.LastID != 3 {
		t.Fatalf("unexpected second stats: %+v", stats2)
	}

	if got := store.totalInserted(); got != 3 {
		t.Fatalf("expected 3 inserted chunks total, got %d", got)
	}
	if dupCount := store.duplicateInserts(); dupCount != 0 {
		t.Fatalf("expected no duplicate inserts, got %d", dupCount)
	}

	cur, err := cursor.Get(context.Background(), "wiki-src")
	if err != nil {
		t.Fatalf("cursor Get error: %v", err)
	}
	if cur == nil || cur.LastID != 3 {
		t.Fatalf("expected cursor at page 3, got %+v", cur)
	}
}

func TestRunOnceResumesAfterFailure(t *testing.T) {
	client := &fakeClient{
		pages: []*Page{
			{ID: 1, Title: "Page 1", URL: "https://wiki/p1", Wikitext: "alpha"},
			{ID: 2, Title: "Page 2", URL: "https://wiki/p2", Wikitext: "beta"},
			{ID: 3, Title: "Page 3", URL: "https://wiki/p3", Wikitext: "gamma"},
		},
		failGetPageCount: map[int64]int{2: 1},
	}
	chunker := NewChunker(1000, 0)
	cursor := NewInMemoryCursorStore()
	dedupe := NewInMemoryDeduper()
	embedder := &fakeEmbedder{}
	store := newFakeStore()

	ingester := New(Config{
		Client:    client,
		Chunker:   chunker,
		Cursor:    cursor,
		Dedupe:    dedupe,
		Embedder:  embedder,
		Store:     store,
		SourceID:  "wiki-src",
		BatchSize: 2,
	})

	stats1, err := ingester.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce() first error = %v", err)
	}
	if stats1.Errors != 1 || stats1.ChunksInserted != 1 || stats1.LastID != 1 {
		t.Fatalf("unexpected first stats: %+v", stats1)
	}

	cur1, err := cursor.Get(context.Background(), "wiki-src")
	if err != nil {
		t.Fatalf("cursor Get error: %v", err)
	}
	if cur1 == nil || cur1.LastID != 1 {
		t.Fatalf("expected cursor to stay at 1 after failure, got %+v", cur1)
	}

	stats2, err := ingester.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce() second error = %v", err)
	}
	if stats2.Errors != 0 || stats2.ChunksInserted != 2 || stats2.LastID != 3 {
		t.Fatalf("unexpected second stats: %+v", stats2)
	}

	if got := store.totalInserted(); got != 3 {
		t.Fatalf("expected 3 chunks inserted across runs, got %d", got)
	}
	if dupCount := store.duplicateInserts(); dupCount != 0 {
		t.Fatalf("expected no duplicate inserts, got %d", dupCount)
	}

	insertedPages := store.insertedPageIDs()
	sort.Slice(insertedPages, func(i, j int) bool { return insertedPages[i] < insertedPages[j] })
	if fmt.Sprint(insertedPages) != "[1 2 3]" {
		t.Fatalf("expected inserted pages [1 2 3], got %v", insertedPages)
	}
}

func TestRunOnceDedupeSkipsExistingHash(t *testing.T) {
	client := &fakeClient{pages: []*Page{{ID: 1, Title: "Page 1", URL: "https://wiki/p1", Wikitext: "shared-content"}}}
	cursor := NewInMemoryCursorStore()
	dedupe := NewInMemoryDeduper()
	if err := dedupe.MarkHash(context.Background(), ContentHash("shared-content")); err != nil {
		t.Fatalf("seed dedupe error: %v", err)
	}
	store := newFakeStore()

	ingester := New(Config{
		Client:    client,
		Chunker:   NewChunker(1000, 0),
		Cursor:    cursor,
		Dedupe:    dedupe,
		Embedder:  &fakeEmbedder{},
		Store:     store,
		SourceID:  "wiki-src",
		BatchSize: 1,
	})

	stats, err := ingester.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if stats.Skipped != 1 {
		t.Fatalf("expected skipped=1, got %d", stats.Skipped)
	}
	if stats.ChunksInserted != 0 {
		t.Fatalf("expected no inserts, got %d", stats.ChunksInserted)
	}
}

func TestRunOnceChunkErrorStopsPage(t *testing.T) {
	client := &fakeClient{pages: []*Page{
		{ID: 1, Title: "Page 1", URL: "https://wiki/p1", Wikitext: "page one"},
		{ID: 2, Title: "Page 2", URL: "https://wiki/p2", Wikitext: strings.Repeat("segment sentence. ", 25)},
		{ID: 3, Title: "Page 3", URL: "https://wiki/p3", Wikitext: "page three"},
	}}
	cursor := NewInMemoryCursorStore()
	store := newFakeStore()
	embedder := &fakeEmbedder{failAfterCount: 2}

	ingester := New(Config{
		Client:    client,
		Chunker:   NewChunker(160, 0),
		Cursor:    cursor,
		Dedupe:    NewInMemoryDeduper(),
		Embedder:  embedder,
		Store:     store,
		SourceID:  "wiki-src",
		BatchSize: 2,
	})

	stats, err := ingester.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if stats.Errors == 0 {
		t.Fatalf("expected at least one error")
	}
	if stats.LastID != 1 {
		t.Fatalf("expected cursor to remain at page 1, got LastID=%d", stats.LastID)
	}

	cur, err := cursor.Get(context.Background(), "wiki-src")
	if err != nil {
		t.Fatalf("cursor Get error: %v", err)
	}
	if cur == nil || cur.LastID != 1 {
		t.Fatalf("expected saved cursor at page 1, got %+v", cur)
	}

	insertedFor2 := 0
	for _, rec := range store.recordsByPage(2) {
		if rec.PageID == 2 {
			insertedFor2++
		}
	}
	if insertedFor2 != 1 {
		t.Fatalf("expected exactly one successful chunk insert for page 2 before failure, got %d", insertedFor2)
	}
}

type fakeClient struct {
	pages            []*Page
	failGetPageCount map[int64]int
}

func (f *fakeClient) ListPages(_ context.Context, fromID int64, limit int) ([]PageSummary, error) {
	if limit <= 0 {
		return nil, nil
	}
	out := make([]PageSummary, 0, limit)
	for _, p := range f.pages {
		if p.ID <= fromID {
			continue
		}
		out = append(out, PageSummary{ID: p.ID, Title: p.Title})
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func (f *fakeClient) GetPage(_ context.Context, id int64) (*Page, error) {
	if n := f.failGetPageCount[id]; n > 0 {
		f.failGetPageCount[id] = n - 1
		return nil, errors.New("temporary get page failure")
	}
	for _, p := range f.pages {
		if p.ID == id {
			cp := *p
			return &cp, nil
		}
	}
	return nil, fmt.Errorf("page %d not found", id)
}

type fakeEmbedder struct {
	mu                 sync.Mutex
	failOnTextContains string
	failAfterCount     int
	callCount          int
}

func (f *fakeEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.failAfterCount > 0 && f.callCount >= f.failAfterCount {
		return nil, errors.New("embedding failure")
	}

	if f.failOnTextContains != "" && strings.Contains(text, f.failOnTextContains) {
		return nil, errors.New("embedding failure")
	}

	f.callCount++
	return []float32{float32(len(text))}, nil
}

type fakeStore struct {
	mu         sync.Mutex
	records    map[string]LoreChunkRecord
	order      []LoreChunkRecord
	dupInserts int
}

func newFakeStore() *fakeStore {
	return &fakeStore{records: make(map[string]LoreChunkRecord)}
}

func (s *fakeStore) InsertChunk(_ context.Context, chunk LoreChunkRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := chunkKey(chunk.PageID, chunk.ChunkIdx)
	if _, exists := s.records[k]; exists {
		s.dupInserts++
	}
	s.records[k] = chunk
	s.order = append(s.order, chunk)
	return nil
}

func (s *fakeStore) ChunkExists(_ context.Context, pageID int64, chunkIndex int) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.records[chunkKey(pageID, chunkIndex)]
	return ok, nil
}

func (s *fakeStore) totalInserted() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.records)
}

func (s *fakeStore) duplicateInserts() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dupInserts
}

func (s *fakeStore) insertedPageIDs() []int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]int64, 0, len(s.records))
	for _, rec := range s.records {
		ids = append(ids, rec.PageID)
	}
	return ids
}

func (s *fakeStore) recordsByPage(pageID int64) []LoreChunkRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]LoreChunkRecord, 0)
	for _, rec := range s.order {
		if rec.PageID == pageID {
			out = append(out, rec)
		}
	}
	return out
}

func chunkKey(pageID int64, chunkIdx int) string {
	return fmt.Sprintf("%d:%d", pageID, chunkIdx)
}

func stringsRepeat(s string, count int) string {
	if count <= 0 {
		return ""
	}
	out := ""
	for i := 0; i < count; i++ {
		out += s
	}
	return out
}
