package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"

	"github.com/eko/iris-bot/internal/config"
	"github.com/eko/iris-bot/internal/embedder"
	"github.com/eko/iris-bot/internal/repository"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "show counts but do not write embeddings")
	batchSize := flag.Int("batch", 50, "max rows to embed per pass")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("pgxpool", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	db := repository.NewDB(pool)

	if cfg.EmbedModelPath == "" || cfg.EmbedTokenizerPath == "" {
		slog.Error("ONNX embedder paths missing; set IRIS_EMBED_MODEL_PATH and IRIS_EMBED_TOKENIZER_PATH")
		os.Exit(1)
	}

	emb, err := embedder.NewONNX(embedder.ONNXConfig{
		ModelPath:     cfg.EmbedModelPath,
		TokenizerPath: cfg.EmbedTokenizerPath,
	})
	if err != nil {
		slog.Error("onnx embedder", "err", err)
		os.Exit(1)
	}
	defer emb.Close()

	rows, err := db.Query(ctx,
		`SELECT id, content FROM memory_records WHERE embedding IS NULL ORDER BY id LIMIT $1`,
		*batchSize,
	)
	if err != nil {
		slog.Error("query", "err", err)
		os.Exit(1)
	}

	type pending struct {
		id      int64
		content string
	}
	var todo []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.id, &p.content); err != nil {
			rows.Close()
			slog.Error("scan", "err", err)
			os.Exit(1)
		}
		todo = append(todo, p)
	}
	rows.Close()

	fmt.Printf("rows pending embedding: %d (limit %d)\n", len(todo), *batchSize)
	if len(todo) == 0 {
		return
	}
	if *dryRun {
		for _, p := range todo {
			snippet := p.content
			if len(snippet) > 80 {
				snippet = snippet[:80]
			}
			fmt.Printf("  - id=%d content=%q\n", p.id, snippet)
		}
		return
	}

	var (
		ok    int
		fails int
	)
	for _, p := range todo {
		vec, embErr := emb.Embed(ctx, p.content)
		if embErr != nil {
			slog.Warn("embed_failed", "id", p.id, "err", embErr)
			fails++
			continue
		}
		if _, err := db.Exec(ctx,
			`UPDATE memory_records SET embedding = $1, updated_at = NOW() WHERE id = $2`,
			pgvector.NewVector(vec), p.id,
		); err != nil {
			slog.Warn("update_failed", "id", p.id, "err", err)
			fails++
			continue
		}
		ok++
	}

	fmt.Printf("done. updated=%d failed=%d\n", ok, fails)
	if fails > 0 {
		os.Exit(1)
	}
}
