package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/eko/iris-bot/internal/app/wire"
	"github.com/eko/iris-bot/internal/config"
	"github.com/eko/iris-bot/internal/domain"
	"github.com/eko/iris-bot/internal/embedder"
	"github.com/eko/iris-bot/internal/llm"
	"github.com/eko/iris-bot/internal/memory"
	"github.com/eko/iris-bot/internal/orchestrator"
	"github.com/eko/iris-bot/internal/repository"
	"github.com/eko/iris-bot/internal/safety"
)

func main() {
	guildID := flag.Int64("guild", 0, "guild id (required)")
	userID := flag.Int64("user", 0, "user id (required)")
	channelID := flag.Int64("channel", 0, "channel id (required)")
	userMessage := flag.String("user-message", "iris, tolong ingat ingat, waifu gw denia", "user message text")
	botResponse := flag.String("bot-response", "Oke, noted. Waifu kamu Denia. Udah aku catat di arsip.", "simulated bot response")
	flag.Parse()

	if *guildID == 0 || *userID == 0 || *channelID == 0 {
		fmt.Fprintln(os.Stderr, "guild, user, channel are required")
		os.Exit(2)
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})))

	ctx := context.Background()
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config load", "err", err)
		os.Exit(1)
	}

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("pgxpool", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	db := repository.NewDB(pool)
	memoryRepo := repository.NewMemoryRepo(db)

	emb, err := embedder.NewONNX(embedder.ONNXConfig{
		ModelPath:     cfg.EmbedModelPath,
		TokenizerPath: cfg.EmbedTokenizerPath,
	})
	if err != nil {
		slog.Error("onnx embedder", "err", err)
		os.Exit(1)
	}
	defer emb.Close()

	memSvc := memory.NewMemoryService(memory.Config{
		Embed: emb,
		Store: &wire.MemoryStoreAdapter{Repo: memoryRepo},
		TopK:  5,
	})

	chatClient := llm.NewClient(&llm.Config{
		APIKey:     cfg.OpenAIAPIKey,
		BaseURL:    firstNonEmpty(cfg.LLMBaseURL, "https://api.openai.com"),
		Model:      firstNonEmpty(cfg.LLMModel, "gpt-4o-mini"),
		Timeout:    cfg.LLMChatTimeout,
		MaxRetries: cfg.LLMMaxRetries,
		RetryDelay: cfg.LLMRetryDelay,
	})

	pipeline := safety.NewSafetyPipeline()
	promoter := orchestrator.NewMemoryPromoter(orchestrator.MemoryPromoterConfig{
		LLM:    &wire.CrossChannelLLMAdapter{Client: chatClient},
		Model:  cfg.LLMModelRouter,
		Writer: &wire.MemoryWriterAdapter{Service: memSvc},
		Safety: &wire.SafetyCheckerAdapter{Injection: pipeline.Injection, Output: pipeline.Output},
	})

	authorName := "TestUser"
	event := &domain.DiscordEvent{
		GuildID:    *guildID,
		ChannelID:  *channelID,
		UserID:     *userID,
		AuthorName: &authorName,
		Message: &domain.DiscordMessage{
			ID:        time.Now().UnixNano(),
			GuildID:   *guildID,
			ChannelID: *channelID,
			UserID:    *userID,
			Content:   *userMessage,
		},
	}

	contextMessages := []*domain.ChannelMessage{
		{
			GuildID:   *guildID,
			ChannelID: *channelID,
			UserID:    *userID,
			Content:   *userMessage,
		},
	}

	before := mustCount(ctx, db, *guildID)
	slog.Info("memprobe rows before", "guild", *guildID, "rows", before)

	promoter.Consider(ctx, event, contextMessages, *botResponse)

	deadline := time.After(45 * time.Second)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	lastCount := before
	for {
		select {
		case <-deadline:
			final := mustCount(ctx, db, *guildID)
			slog.Error("memprobe timeout", "before", before, "final", final)
			if final > before {
				os.Exit(0)
			}
			os.Exit(1)
		case <-ticker.C:
			c := mustCount(ctx, db, *guildID)
			if c != lastCount {
				slog.Info("memprobe rows", "rows", c)
				lastCount = c
			}
			if c > before {
				slog.Info("memprobe success", "before", before, "after", c)
				time.Sleep(500 * time.Millisecond)
				return
			}
		}
	}
}

func mustCount(ctx context.Context, db *repository.DB, guildID int64) int {
	row := db.QueryRow(ctx, "SELECT COUNT(*) FROM memory_records WHERE guild_id = $1", guildID)
	var n int
	if err := row.Scan(&n); err != nil {
		if !errors.Is(err, context.Canceled) {
			slog.Warn("memprobe count error", "err", err)
		}
	}
	return n
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
