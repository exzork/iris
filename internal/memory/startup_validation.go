package memory

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/eko/iris-bot/internal/config"
	"github.com/eko/iris-bot/internal/embedder"
	"github.com/eko/iris-bot/internal/repository"
	"github.com/jackc/pgx/v5"
)

type startupValidationDB interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type MemoryRuntimeConfig struct {
	Enabled       bool
	RecallEnabled bool
	WorkerEnabled bool
	RecallConfig  GuildRecallConfig
	WorkerConfig  EmbeddingWorkerConfig
	Embedder      embedder.Embedder
}

const (
	pgvectorExtensionQuery    = `SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname='vector')`
	contentEmbeddingTypeQuery = `
		SELECT format_type(a.atttypid, a.atttypmod)
		FROM pg_attribute a
		WHERE a.attrelid='channel_messages'::regclass
		  AND a.attname='content_embedding'
		  AND NOT a.attisdropped
	`
)

func BuildRuntimeConfig(cfg config.MemoryServerConfig, emb embedder.Embedder) MemoryRuntimeConfig {
	runtime := MemoryRuntimeConfig{
		Enabled: cfg.Enabled,
		RecallConfig: GuildRecallConfig{
			Enabled:   cfg.Enabled,
			Threshold: cfg.RecallThreshold,
			TopK:      cfg.RecallTopK,
		},
		WorkerConfig: EmbeddingWorkerConfig{
			Workers:           cfg.EmbedWorkers,
			BackfillLimit:     cfg.EmbedBackfillLimit,
			BackfillInterval:  30 * time.Second,
			DequeueTimeout:    1 * time.Second,
			MaxAttemptsPerJob: 3,
		},
	}
	if !cfg.Enabled {
		return runtime
	}
	runtime.RecallEnabled = true
	runtime.WorkerEnabled = true
	runtime.Embedder = emb
	return runtime
}

// ValidateServerMemoryStartup enforces hard startup contracts for server memory.
// When memory is disabled, validation is bypassed intentionally.
func ValidateServerMemoryStartup(
	ctx context.Context,
	log *slog.Logger,
	cfg *config.Config,
	emb embedder.Embedder,
	db startupValidationDB,
) error {
	if log == nil {
		log = slog.Default()
	}
	if cfg == nil {
		return errors.New("memory startup validation: config is nil")
	}
	log.Info(
		"memory startup",
		"enabled", cfg.MemoryServer.Enabled,
		"threshold", cfg.MemoryServer.RecallThreshold,
		"top_k", cfg.MemoryServer.RecallTopK,
		"workers", cfg.MemoryServer.EmbedWorkers,
		"backfill_limit", cfg.MemoryServer.EmbedBackfillLimit,
	)
	if !cfg.MemoryServer.Enabled {
		log.Info("memory startup disabled; skipping memory validation and wiring")
		return nil
	}
	if err := validateMemoryServerConfig(cfg.MemoryServer); err != nil {
		return err
	}
	if err := validateEmbedderPaths(cfg); err != nil {
		return err
	}
	if emb == nil {
		return errors.New("memory startup validation: embedder is nil")
	}
	if emb.Dim() != repository.ExpectedEmbeddingDim {
		return fmt.Errorf(
			"memory startup validation: embedder dim mismatch: expected=%d actual=%d",
			repository.ExpectedEmbeddingDim,
			emb.Dim(),
		)
	}
	if db == nil {
		return errors.New("memory startup validation: db is nil")
	}
	if err := validatePgvector(ctx, db); err != nil {
		return err
	}
	if err := validateContentEmbeddingDim(ctx, db); err != nil {
		return err
	}
	return nil
}

func validateMemoryServerConfig(cfg config.MemoryServerConfig) error {
	if cfg.RecallThreshold < 0 || cfg.RecallThreshold > 1 {
		return fmt.Errorf(
			"memory startup validation: MEMORY_SERVER_RECALL_THRESHOLD must be in [0,1], got %v",
			cfg.RecallThreshold,
		)
	}
	if cfg.RecallTopK <= 0 {
		return fmt.Errorf(
			"memory startup validation: MEMORY_SERVER_RECALL_TOP_K must be > 0, got %d",
			cfg.RecallTopK,
		)
	}
	if cfg.EmbedWorkers < 1 {
		return fmt.Errorf(
			"memory startup validation: MEMORY_SERVER_EMBED_WORKERS must be >= 1, got %d",
			cfg.EmbedWorkers,
		)
	}
	if cfg.EmbedBatchSize <= 0 {
		return fmt.Errorf(
			"memory startup validation: MEMORY_SERVER_EMBED_BATCH_SIZE must be > 0, got %d",
			cfg.EmbedBatchSize,
		)
	}
	if cfg.EmbedBackfillLimit < 0 {
		return fmt.Errorf(
			"memory startup validation: MEMORY_SERVER_EMBED_BACKFILL_LIMIT must be >= 0, got %d",
			cfg.EmbedBackfillLimit,
		)
	}
	return nil
}

func validateEmbedderPaths(cfg *config.Config) error {
	if cfg.EmbedModelPath == "" {
		return errors.New("memory startup validation: IRIS_EMBED_MODEL_PATH is required when memory server is enabled")
	}
	if cfg.EmbedTokenizerPath == "" {
		return errors.New("memory startup validation: IRIS_EMBED_TOKENIZER_PATH is required when memory server is enabled")
	}
	if err := assertPathExists(cfg.EmbedModelPath, "IRIS_EMBED_MODEL_PATH"); err != nil {
		return err
	}
	if err := assertPathExists(cfg.EmbedTokenizerPath, "IRIS_EMBED_TOKENIZER_PATH"); err != nil {
		return err
	}
	return nil
}

func assertPathExists(path, envName string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("memory startup validation: %s path %q is invalid: %w", envName, path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("memory startup validation: %s path %q is a directory, expected file", envName, path)
	}
	return nil
}

func validatePgvector(ctx context.Context, db startupValidationDB) error {
	var exists bool
	err := db.QueryRow(ctx, pgvectorExtensionQuery).Scan(&exists)
	if err != nil {
		return fmt.Errorf("memory startup validation: failed checking pgvector extension: %w", err)
	}
	if !exists {
		return errors.New("memory startup validation: pgvector extension 'vector' is missing")
	}
	return nil
}

func validateContentEmbeddingDim(ctx context.Context, db startupValidationDB) error {
	var columnType string
	err := db.QueryRow(ctx, contentEmbeddingTypeQuery).Scan(&columnType)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errors.New("memory startup validation: channel_messages.content_embedding column is missing")
		}
		return fmt.Errorf("memory startup validation: failed checking channel_messages.content_embedding schema: %w", err)
	}

	actualDim, ok := parseVectorDim(columnType)
	if !ok {
		return fmt.Errorf(
			"memory startup validation: channel_messages.content_embedding has incompatible type %q (expected vector(%d))",
			columnType,
			repository.ExpectedEmbeddingDim,
		)
	}
	if actualDim != repository.ExpectedEmbeddingDim {
		return fmt.Errorf(
			"memory startup validation: channel_messages.content_embedding dimension mismatch: expected=%d actual=%d",
			repository.ExpectedEmbeddingDim,
			actualDim,
		)
	}
	return nil
}

func parseVectorDim(columnType string) (int, bool) {
	columnType = strings.TrimSpace(strings.ToLower(columnType))
	if !strings.HasPrefix(columnType, "vector(") || !strings.HasSuffix(columnType, ")") {
		return 0, false
	}
	dimPart := strings.TrimSuffix(strings.TrimPrefix(columnType, "vector("), ")")
	if dimPart == "" {
		return 0, false
	}
	var dim int
	if _, err := fmt.Sscanf(dimPart, "%d", &dim); err != nil || dim <= 0 {
		return 0, false
	}
	return dim, true
}
