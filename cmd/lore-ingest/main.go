package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/eko/iris-bot/internal/app/wire"
	"github.com/eko/iris-bot/internal/config"
	"github.com/eko/iris-bot/internal/embedder"
	"github.com/eko/iris-bot/internal/logger"
	"github.com/eko/iris-bot/internal/lore/ingest"
	"github.com/eko/iris-bot/internal/lore/source"
	"github.com/eko/iris-bot/internal/repository"
)

func main() {
	sourceID := flag.String("source", "fandom_wutheringwaves", "registered source ID")
	apiBaseURL := flag.String("api-base-url", "https://wutheringwaves.fandom.com/api.php", "MediaWiki action API endpoint")
	pageBaseURL := flag.String("page-base-url", "https://wutheringwaves.fandom.com/wiki/", "canonical wiki page prefix")
	batchSize := flag.Int("batch-size", 10, "pages fetched per ListPages call")
	maxBatches := flag.Int("max-batches", 0, "stop after N batches (0 = until source exhausted)")
	pageBudget := flag.Int("page-budget", 0, "stop after N pages fetched (0 = unlimited)")
	chunkChars := flag.Int("chunk-chars", 1000, "max characters per chunk")
	chunkOverlap := flag.Int("chunk-overlap", 100, "characters of overlap between chunks")
	minIntervalMS := flag.Int("min-interval-ms", 1000, "minimum spacing between MediaWiki HTTP calls (ms)")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	log := logger.NewWithDebug(cfg.Debug)
	if cfg.Debug {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})))
	}

	registry := source.DefaultRegistry()
	src, ok := registry.Get(*sourceID)
	if !ok {
		log.Error("unknown source id", "source", *sourceID)
		os.Exit(1)
	}
	if err := registry.ValidateAccess(src.Host, source.MethodMediaWikiAPI); err != nil {
		log.Error("source does not allow mediawiki_api access", "source", *sourceID, "err", err)
		os.Exit(1)
	}

	rootCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		log.Info("ingest received shutdown signal; finishing current batch")
		cancel()
	}()

	pool, err := pgxpool.New(rootCtx, cfg.DatabaseURL)
	if err != nil {
		log.Error("failed to connect to Postgres", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	db := repository.NewDB(pool)
	wikiRepo := repository.NewWikiRepo(db)

	embedClient, err := embedder.NewONNX(embedder.ONNXConfig{
		ModelPath:     cfg.EmbedModelPath,
		TokenizerPath: cfg.EmbedTokenizerPath,
	})
	if err != nil {
		log.Error("failed to load ONNX embedder", "model_path", cfg.EmbedModelPath, "err", err)
		os.Exit(1)
	}
	defer embedClient.Close()

	client := ingest.NewHTTPMediaWikiClient(*apiBaseURL, *pageBaseURL, src.Policy.UserAgent)
	client.MinInterval = time.Duration(*minIntervalMS) * time.Millisecond

	ingester := ingest.New(ingest.Config{
		Client:    client,
		Chunker:   ingest.NewChunker(*chunkChars, *chunkOverlap),
		Cursor:    &wire.WikiCursorAdapter{Repo: wikiRepo, SourceID: *sourceID},
		Dedupe:    &wire.WikiDedupeAdapter{Repo: wikiRepo, SourceID: *sourceID},
		Embedder:  embedClient,
		Store:     &wire.WikiIngestStoreAdapter{Repo: wikiRepo, SourceID: *sourceID},
		SourceID:  *sourceID,
		BatchSize: *batchSize,
		OnError: func(stage string, pageID int64, title string, err error) {
			log.Warn("ingest stage error",
				"stage", stage,
				"page_id", pageID,
				"title", title,
				"err", err.Error(),
			)
		},
	})

	log.Info("lore ingestion starting",
		"source", *sourceID,
		"api_base_url", *apiBaseURL,
		"batch_size", *batchSize,
		"max_batches", *maxBatches,
		"page_budget", *pageBudget,
		"min_interval_ms", *minIntervalMS,
	)

	totals := struct {
		Pages   int
		Chunks  int
		Skipped int
		Errors  int
	}{}

	for batch := 0; ; batch++ {
		if *maxBatches > 0 && batch >= *maxBatches {
			log.Info("reached max-batches; stopping", "batch", batch)
			break
		}
		if *pageBudget > 0 && totals.Pages >= *pageBudget {
			log.Info("reached page-budget; stopping", "pages", totals.Pages)
			break
		}

		if err := rootCtx.Err(); err != nil {
			log.Info("context cancelled; stopping", "err", err)
			break
		}

		stats, err := ingester.RunOnce(rootCtx)
		totals.Pages += stats.PagesFetched
		totals.Chunks += stats.ChunksInserted
		totals.Skipped += stats.Skipped
		totals.Errors += stats.Errors

		if err != nil {
			if errors.Is(err, context.Canceled) {
				log.Info("ingest cancelled mid-batch")
				break
			}
			log.Error("ingest batch failed", "batch", batch, "err", err)
			break
		}

		log.Info("ingest batch done",
			"batch", batch,
			"pages", stats.PagesFetched,
			"chunks", stats.ChunksInserted,
			"skipped", stats.Skipped,
			"errors", stats.Errors,
			"last_id", stats.LastID,
		)

		if stats.PagesFetched == 0 {
			log.Info("no more pages; source exhausted")
			break
		}
	}

	pageCount, _ := wikiRepo.PageCount(context.Background(), *sourceID)
	chunkCount, _ := wikiRepo.ChunkCount(context.Background(), *sourceID)
	log.Info("lore ingestion done",
		"source", *sourceID,
		"run_pages", totals.Pages,
		"run_chunks_inserted", totals.Chunks,
		"run_skipped", totals.Skipped,
		"run_errors", totals.Errors,
		"db_pages_total", pageCount,
		"db_chunks_total", chunkCount,
	)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
