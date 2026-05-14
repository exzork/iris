package memory

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eko/iris-bot/internal/config"
	"github.com/eko/iris-bot/internal/embedder"
	"github.com/eko/iris-bot/internal/repository"
	"github.com/jackc/pgx/v5"
)

type fakeStartupRow struct {
	value any
	err   error
}

func (r fakeStartupRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != 1 {
		return errors.New("fakeStartupRow expects exactly one destination")
	}
	switch d := dest[0].(type) {
	case *bool:
		*d = r.value.(bool)
	case *string:
		*d = r.value.(string)
	default:
		return errors.New("unsupported destination type")
	}
	return nil
}

type fakeStartupDB struct {
	pgvectorRow   fakeStartupRow
	columnTypeRow fakeStartupRow
	queries       []string
}

func (f *fakeStartupDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	f.queries = append(f.queries, sql)
	if strings.Contains(sql, "pg_extension") {
		return f.pgvectorRow
	}
	if strings.Contains(sql, "pg_attribute") {
		return f.columnTypeRow
	}
	return fakeStartupRow{err: errors.New("unexpected query")}
}

type fakeStartupEmbedder struct {
	dim int
}

func (f fakeStartupEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	return make([]float32, f.dim), nil
}

func (f fakeStartupEmbedder) Dim() int { return f.dim }

func (f fakeStartupEmbedder) Close() error { return nil }

func mustWriteTempFile(t *testing.T, dir string, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return p
}

func validStartupConfig(t *testing.T) *config.Config {
	t.Helper()
	tmpDir := t.TempDir()
	modelPath := mustWriteTempFile(t, tmpDir, "model.onnx")
	tokenizerPath := mustWriteTempFile(t, tmpDir, "tokenizer.json")
	return &config.Config{
		EmbedModelPath:     modelPath,
		EmbedTokenizerPath: tokenizerPath,
		MemoryServer: config.MemoryServerConfig{
			Enabled:            true,
			RecallThreshold:    0.72,
			RecallTopK:         5,
			EmbedWorkers:       2,
			EmbedBatchSize:     32,
			EmbedBackfillLimit: 500,
		},
	}
}

func validStartupDB() *fakeStartupDB {
	return &fakeStartupDB{
		pgvectorRow:   fakeStartupRow{value: true},
		columnTypeRow: fakeStartupRow{value: "vector(384)"},
	}
}

func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))
}

func TestServerMemoryStartup_ValidConfigPasses(t *testing.T) {
	cfg := validStartupConfig(t)
	db := validStartupDB()
	err := ValidateServerMemoryStartup(context.Background(), silentLogger(), cfg, embedder.NewFakeEmbedder(), db)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if len(db.queries) != 2 {
		t.Fatalf("expected 2 startup validation queries, got %d", len(db.queries))
	}
}

func TestServerMemoryStartup_DimensionMismatch(t *testing.T) {
	cfg := validStartupConfig(t)
	db := validStartupDB()
	err := ValidateServerMemoryStartup(context.Background(), silentLogger(), cfg, fakeStartupEmbedder{dim: 512}, db)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "embedder dim mismatch") {
		t.Fatalf("expected embedder dim mismatch error, got %v", err)
	}
	if !strings.Contains(err.Error(), "expected") || !strings.Contains(err.Error(), "actual") {
		t.Fatalf("expected expected/actual in dim mismatch error, got %v", err)
	}
	if !strings.Contains(err.Error(), "384") || !strings.Contains(err.Error(), "512") {
		t.Fatalf("expected 384 and 512 in error, got %v", err)
	}
}

func TestServerMemoryStartup_MissingPgvector(t *testing.T) {
	cfg := validStartupConfig(t)
	db := validStartupDB()
	db.pgvectorRow = fakeStartupRow{value: false}
	err := ValidateServerMemoryStartup(context.Background(), silentLogger(), cfg, embedder.NewFakeEmbedder(), db)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "pgvector") {
		t.Fatalf("expected pgvector extension error, got %v", err)
	}
}

func TestServerMemoryStartup_ColumnDimensionMismatch(t *testing.T) {
	cfg := validStartupConfig(t)
	db := validStartupDB()
	db.columnTypeRow = fakeStartupRow{value: "vector(256)"}
	err := ValidateServerMemoryStartup(context.Background(), silentLogger(), cfg, embedder.NewFakeEmbedder(), db)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "channel_messages.content_embedding") {
		t.Fatalf("expected content_embedding mention, got %v", err)
	}
	if !strings.Contains(err.Error(), "expected") || !strings.Contains(err.Error(), "actual") {
		t.Fatalf("expected expected/actual in schema mismatch error, got %v", err)
	}
	if !strings.Contains(err.Error(), "384") || !strings.Contains(err.Error(), "256") {
		t.Fatalf("expected 384 and 256 in error, got %v", err)
	}
	if len(db.queries) < 2 {
		t.Fatalf("expected pgvector+schema queries, got %d", len(db.queries))
	}
	if !strings.Contains(db.queries[1], "channel_messages") || !strings.Contains(db.queries[1], "content_embedding") {
		t.Fatalf("unexpected schema query: %s", db.queries[1])
	}
}

func TestServerMemoryStartup_DisabledBypass(t *testing.T) {
	var logBuf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logBuf, nil))
	cfg := &config.Config{MemoryServer: config.MemoryServerConfig{Enabled: false, RecallThreshold: 0.72, RecallTopK: 5, EmbedWorkers: 1, EmbedBackfillLimit: 500}}
	db := validStartupDB()
	err := ValidateServerMemoryStartup(context.Background(), log, cfg, nil, db)
	if err != nil {
		t.Fatalf("expected success when disabled, got %v", err)
	}
	if len(db.queries) != 0 {
		t.Fatalf("expected no DB queries when disabled, got %d", len(db.queries))
	}
	if !strings.Contains(logBuf.String(), "memory startup") || !strings.Contains(logBuf.String(), "disabled") {
		t.Fatalf("expected disabled startup log, got %s", logBuf.String())
	}
}

func TestServerMemoryStartup_InvalidThresholdRejected(t *testing.T) {
	cfg := validStartupConfig(t)
	cfg.MemoryServer.RecallThreshold = 1.4
	db := validStartupDB()
	err := ValidateServerMemoryStartup(context.Background(), silentLogger(), cfg, embedder.NewFakeEmbedder(), db)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "MEMORY_SERVER_RECALL_THRESHOLD") {
		t.Fatalf("expected threshold error, got %v", err)
	}
}

func TestMemoryRuntimeConfig_DisabledSkipsWorkerAndRecallWiring(t *testing.T) {
	runtime := BuildRuntimeConfig(config.MemoryServerConfig{Enabled: false, RecallThreshold: 0.72, RecallTopK: 5, EmbedWorkers: 1, EmbedBackfillLimit: 500}, nil)
	if runtime.RecallEnabled {
		t.Fatal("expected recall wiring disabled")
	}
	if runtime.WorkerEnabled {
		t.Fatal("expected worker wiring disabled")
	}
}

func TestMemoryRuntimeConfig_EnabledWiresWorkerAndRecall(t *testing.T) {
	runtime := BuildRuntimeConfig(config.MemoryServerConfig{Enabled: true, RecallThreshold: 0.72, RecallTopK: 5, EmbedWorkers: 2, EmbedBackfillLimit: 20}, embedder.NewFakeEmbedder())
	if !runtime.RecallEnabled {
		t.Fatal("expected recall wiring enabled")
	}
	if !runtime.WorkerEnabled {
		t.Fatal("expected worker wiring enabled")
	}
	if runtime.WorkerConfig.Workers != 2 {
		t.Fatalf("expected worker count 2, got %d", runtime.WorkerConfig.Workers)
	}
	if runtime.WorkerConfig.BackfillLimit != 20 {
		t.Fatalf("expected backfill limit 20, got %d", runtime.WorkerConfig.BackfillLimit)
	}
	if runtime.RecallConfig.TopK != 5 {
		t.Fatalf("expected recall topK 5, got %d", runtime.RecallConfig.TopK)
	}
	if runtime.Embedder == nil || runtime.Embedder.Dim() != repository.ExpectedEmbeddingDim {
		t.Fatalf("expected runtime embedder with dim=%d", repository.ExpectedEmbeddingDim)
	}
}
