package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/eko/iris-bot/internal/admin"
	"github.com/eko/iris-bot/internal/app"
	wireadapters "github.com/eko/iris-bot/internal/app/wire"
	"github.com/eko/iris-bot/internal/bootstrap"
	"github.com/eko/iris-bot/internal/config"
	"github.com/eko/iris-bot/internal/discord"
	"github.com/eko/iris-bot/internal/domain"
	"github.com/eko/iris-bot/internal/embedder"
	"github.com/eko/iris-bot/internal/llm"
	"github.com/eko/iris-bot/internal/logger"
	"github.com/eko/iris-bot/internal/lore/rag"
	"github.com/eko/iris-bot/internal/lorethread"
	"github.com/eko/iris-bot/internal/mcp"
	"github.com/eko/iris-bot/internal/memory"
	"github.com/eko/iris-bot/internal/orchestrator"
	"github.com/eko/iris-bot/internal/persona"
	"github.com/eko/iris-bot/internal/repository"
	"github.com/eko/iris-bot/internal/slash"
	"github.com/eko/iris-bot/internal/router"
	"github.com/eko/iris-bot/internal/safety"
	"github.com/eko/iris-bot/internal/tools"
	"github.com/eko/iris-bot/internal/tools/escalate"
	lorethread_tool "github.com/eko/iris-bot/internal/tools/lorethread"
	"github.com/eko/iris-bot/internal/tools/memesearch"
	"github.com/eko/iris-bot/internal/tools/modelswitch"
	"github.com/eko/iris-bot/internal/tools/websearch"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	checkConfig := flag.Bool("check-config", false, "validate config and exit")
	runBootstrap := flag.Bool("bootstrap", false, "seed initial guild and exit")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	if *checkConfig {
		fmt.Println("config ok")
		return
	}

	log := logger.NewWithDebug(cfg.Debug)
	if cfg.Debug {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})))
	}
	log.Info("iris-bot starting")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Database
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("failed to connect to Postgres", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	db := repository.NewDB(pool)
	guildRepo := repository.NewGuildRepo(db)
	settingsRepo := repository.NewSettingsRepo(db)
	exceptionRepo := repository.NewExceptionChannelRepo(db)
	allowedRepo := repository.NewAllowedChannelRepo(db)
	channelMessageRepo := repository.NewChannelMessageRepo(db)
	episodeRepo := repository.NewEpisodeMemoryRepo(db)
	memoryRepo := repository.NewMemoryRepo(db)
	wikiRepo := repository.NewWikiRepo(db)
	auditRepo := repository.NewAuditRepo(db)
	globalSettingsRepo := repository.NewGlobalSettingsRepo(db)
	loreSessionRepo := repository.NewLoreSessionRepo(db)
	loreGuildSettingsRepo := repository.NewLoreGuildSettingsRepo(db)
	loreThreadAnchorRepo := repository.NewLoreThreadAnchorRepo(db)

	modelResolver := llm.NewModelResolver(
		globalSettingsRepo,
		config.ValidateModelName,
		cfg.LLMModelDefault,
		cfg.LLMModelStrong,
		cfg.LLMModelRouter,
	)
	if loadErr := modelResolver.Load(ctx); loadErr != nil {
		log.Warn("failed to load model overrides from global_settings; using env fallbacks", "err", loadErr)
	} else {
		log.Info("model resolver ready",
			"default", modelResolver.Default(),
			"strong", modelResolver.Strong(),
			"router", modelResolver.Router(),
		)
	}

	// Bootstrap-only mode
	if *runBootstrap {
		guildIDStr := os.Getenv("INITIAL_GUILD_ID")
		if guildIDStr == "" {
			log.Error("INITIAL_GUILD_ID env var is required for --bootstrap")
			os.Exit(1)
		}
		gid, parseErr := strconv.ParseInt(guildIDStr, 10, 64)
		if parseErr != nil {
			log.Error("invalid INITIAL_GUILD_ID", "err", parseErr)
			os.Exit(1)
		}
		boot := &bootstrap.Bootstrapper{
			Guilds:   &wireadapters.GuildStoreAdapter{Repo: guildRepo},
			Settings: &wireadapters.SettingsStoreAdapter{Repo: settingsRepo},
		}
		res, err := boot.Seed(ctx, gid, os.Getenv("ADMIN_ROLE_IDS"))
		if err != nil {
			log.Error("bootstrap failed", "err", err)
			os.Exit(1)
		}
		log.Info("bootstrap complete", "created", res.GuildCreated, "settings_added", len(res.SettingsAdded), "admins_seeded", res.AdminsSeeded, "idempotent", res.Idempotent)
		return
	}

	// LLM clients
	chatCfg := &llm.Config{
		APIKey:     cfg.OpenAIAPIKey,
		BaseURL:    firstNonEmpty(cfg.LLMBaseURL, "https://api.openai.com"),
		Model:      firstNonEmpty(cfg.LLMModel, "gpt-4o-mini"),
		Timeout:    cfg.LLMChatTimeout,
		MaxRetries: cfg.LLMMaxRetries,
		RetryDelay: cfg.LLMRetryDelay,
	}
	chatClient := llm.NewClient(chatCfg)

	toolsCfg := &llm.Config{
		APIKey:     cfg.OpenAIAPIKey,
		BaseURL:    firstNonEmpty(cfg.LLMBaseURL, "https://api.openai.com"),
		Model:      firstNonEmpty(cfg.LLMModel, "gpt-4o-mini"),
		Timeout:    cfg.LLMToolTimeout,
		MaxRetries: cfg.LLMMaxRetries,
		RetryDelay: cfg.LLMRetryDelay,
	}
	toolsClient := llm.NewClient(toolsCfg)

	log.Info("llm_clients_ready", "chat_timeout", cfg.LLMChatTimeout.String(), "tool_timeout", cfg.LLMToolTimeout.String())

	embedCfg := &llm.EmbeddingConfig{
		APIKey:     cfg.OpenAIAPIKey,
		BaseURL:    chatCfg.BaseURL,
		Model:      "text-embedding-3-small",
		Timeout:    30 * time.Second,
		MaxRetries: 3,
		RetryDelay: 1 * time.Second,
	}
	embedClient := llm.NewEmbeddingClient(embedCfg)

	imageCfg := &llm.ImageConfig{
		APIKey:     cfg.OpenAIAPIKey,
		BaseURL:    chatCfg.BaseURL,
		Model:      "dall-e-3",
		Timeout:    60 * time.Second,
		MaxRetries: 3,
		RetryDelay: 1 * time.Second,
	}
	imageClient := llm.NewImageClient(imageCfg)

	// Construct embedder lazily if paths are configured
	var emb embedder.Embedder
	if cfg.EmbedModelPath != "" && cfg.EmbedTokenizerPath != "" {
		onnxCfg := embedder.ONNXConfig{
			ModelPath:     cfg.EmbedModelPath,
			TokenizerPath: cfg.EmbedTokenizerPath,
		}
		var embErr error
		emb, embErr = embedder.NewONNX(onnxCfg)
		if embErr != nil {
			log.Warn("failed to load embedder, falling back to LLM classifiers", "err", embErr)
			emb = nil
		}
	}

	// Memory service. ONNX-backed because memory_records.embedding is
	// vector(384) and the local LLM proxy does not serve OpenAI embeddings.
	var memoryEmbedder memory.EmbeddingProvider
	if emb != nil {
		memoryEmbedder = emb
	} else {
		memoryEmbedder = embedClient
		log.Warn("memory using remote embedder; vector dim mismatch likely")
	}
	memSvc := memory.NewMemoryService(memory.Config{
		Embed: memoryEmbedder,
		Store: &wireadapters.MemoryStoreAdapter{Repo: memoryRepo},
		TopK:  5,
	})

	// RAG retrieval (wiki). Uses ONNX embedder so queries match the 384-dim wiki_chunks.
	var ragEmbedder rag.EmbeddingProvider
	if emb != nil {
		ragEmbedder = emb
	} else {
		ragEmbedder = embedClient
		log.Warn("ONNX embedder unavailable; wiki RAG falling back to remote embedder (vector dim mismatch likely)")
	}
	retriever := &rag.Retriever{
		Embed:    ragEmbedder,
		Store:    &wireadapters.WikiRetrievalAdapter{Repo: wikiRepo, SourceID: "fandom_wutheringwaves"},
		MinScore: 0.0,
	}
	composer := &rag.Composer{Retriever: retriever, MinChunks: 1}

	// Safety pipeline
	pipeline := safety.NewSafetyPipeline()

	memoryRuntime := memory.BuildRuntimeConfig(cfg.MemoryServer, emb)
	if err := memory.ValidateServerMemoryStartup(ctx, log, cfg, memoryRuntime.Embedder, db); err != nil {
		log.Error("memory startup validation failed", "err", err)
		os.Exit(1)
	}

	// Task-6 orchestrator components (optional wiring, nil-safe)
	var inWindowRelevance orchestrator.InWindowRelevance
	var crossChannelClassifier orchestrator.CrossChannelClassifier
	var memoryPromoter orchestrator.MemoryPromoter

	if emb != nil {
		inWindowRelevance = orchestrator.NewSimilarityInWindowRelevance(orchestrator.SimilarityInWindowConfig{
			Embedder:   emb,
			Threshold:  cfg.EmbedSimThreshold,
			MinContext: 1,
		})
		crossChannelClassifier = orchestrator.NewSimilarityCrossChannelClassifier(orchestrator.SimilarityCrossChannelConfig{
			Store:         &wireadapters.CandidateStoreAdapter{Repo: channelMessageRepo},
			Allowed:       &wireadapters.ChannelAllowAdapter{Repo: allowedRepo},
			Embedder:      emb,
			Threshold:     cfg.EmbedSimThreshold,
			MaxCandidates: 10,
			WindowMinutes: 30,
		})
		log.Info("context classifier backend=similarity", "threshold", cfg.EmbedSimThreshold)
	} else {
		inWindowRelevance = orchestrator.NewLLMInWindowRelevance(orchestrator.LLMInWindowRelevanceConfig{
			LLM:   &wireadapters.CrossChannelLLMAdapter{Client: chatClient},
			Model: cfg.LLMModelRouter,
		})
		crossChannelClassifier = orchestrator.NewLLMCrossChannelClassifier(orchestrator.LLMCrossChannelConfig{
			Store:   &wireadapters.CandidateStoreAdapter{Repo: channelMessageRepo},
			Allowed: &wireadapters.ChannelAllowAdapter{Repo: allowedRepo},
			LLM:     &wireadapters.CrossChannelLLMAdapter{Client: chatClient},
			Model:   cfg.LLMModelRouter,
		})
		log.Info("context classifier backend=llm (embedder disabled)")
	}

	if memSvc != nil {
		memoryPromoter = orchestrator.NewMemoryPromoter(orchestrator.MemoryPromoterConfig{
			LLM:    &wireadapters.CrossChannelLLMAdapter{Client: chatClient},
			Model:  cfg.LLMModelRouter,
			Writer: &wireadapters.MemoryWriterAdapter{Service: memSvc},
			Safety: &wireadapters.SafetyCheckerAdapter{Injection: pipeline.Injection, Output: pipeline.Output},
		})
	}

	baseCapture := &wireadapters.ChannelCaptureAdapter{
		Repo:         channelMessageRepo,
		GuildEnsurer: &wireadapters.GuildEnsurerAdapter{Repo: guildRepo},
	}
	var capturePort orchestrator.ChannelCapture = baseCapture
	if memoryRuntime.WorkerEnabled {
		embeddingQueue := memory.NewBoundedQueue(cfg.MemoryServer.EmbedBatchSize)
		embeddingWorker, workerErr := memory.NewEmbeddingWorker(
			embeddingQueue,
			memoryRuntime.Embedder,
			channelMessageRepo,
			memoryRuntime.WorkerConfig,
		)
		if workerErr != nil {
			log.Error("failed to create memory embedding worker", "err", workerErr)
			os.Exit(1)
		}
		embeddingWorker.Start(ctx)
		defer embeddingWorker.Stop()
		defer embeddingQueue.Close()

		capturePort = &captureChain{
			primary: baseCapture,
			secondary: &enqueueIfMissingCapture{
				next: memory.NewNonBlockingCaptureAdapter(embeddingQueue),
			},
		}
	}

	// Parse bot ID if provided
	botID := int64(0)
	if v := os.Getenv("DISCORD_BOT_ID"); v != "" {
		if id, perr := strconv.ParseInt(v, 10, 64); perr == nil {
			botID = id
		}
	}

	// Conversation repository
	convRepo := repository.NewChannelConversationRepo(db)

	// Trigger router with conversation support
	routerSvc := router.NewTriggerRouterWithConversation(
		&wireadapters.ExceptionChannelAdapter{Repo: exceptionRepo},
		allowedRepo,
		convRepo,
		botID,
	)

	exceptionHandler := admin.NewExceptionHandler(exceptionRepo, auditRepo)
	allowedChannelHandler := admin.NewAllowedChannelHandler(allowedRepo, auditRepo)

	var appInstance *app.App
	var gateway *discord.GatewayAdapter
	var orch *orchestrator.Orchestrator

	gatewayCallback := func(ctx context.Context, event *domain.DiscordEvent) error {
		if event.GuildID > 0 {
			if ensureErr := guildRepo.EnsureGuild(ctx, event.GuildID); ensureErr != nil {
				log.WarnContext(ctx, "failed to ensure guild row", "guild_id", event.GuildID, "error", ensureErr)
			}
		}

		if orch != nil {
			return orch.Enqueue(ctx, event)
		}
		_, handleErr := appInstance.Handle(ctx, event)
		return handleErr
	}

	gateway, err = discord.NewGatewayAdapter(cfg.DiscordToken, botID, gatewayCallback)
	if err != nil {
		log.Error("failed to create Discord gateway", "err", err)
		os.Exit(1)
	}

	// Wire app instance
	appInstance = app.New(
		&wireadapters.TriggerAdapter{Router: routerSvc},
		memSvc,
		&wireadapters.LoreAdapter{Composer: composer},
		&wireadapters.LLMAdapter{Client: chatClient},
		app.NewImagePipeline(&wireadapters.ImageAdapter{Client: imageClient}),
		&wireadapters.DiscordSenderAdapter{Gateway: gateway},
		pipeline,
		persona.BuildSystemPrompt(persona.PromptInput{}),
		nil,
	)

	tierRouter := &llm.TierRouter{
		Classifier: chatClient,
		Router:     cfg.LLMModelRouter,
		Default:    cfg.LLMModelDefault,
		Strong:     cfg.LLMModelStrong,
		Resolver:   modelResolver,
	}
	appInstance.TierRouter = tierRouter

	registry := tools.NewRegistry(nil)

	escalateTool := escalate.New()
	if err := registry.Register(&tools.ToolDefinition{Tool: escalateTool, Timeout: 1 * time.Second, MaxOutput: 1024}); err != nil {
		log.Warn("failed to register escalate tool", "err", err)
	} else {
		log.Info("escalate tool registered")
	}

	searxngURL := os.Getenv("IRIS_SEARXNG_URL")
	if searxngURL != "" {
		provider := websearch.NewSearXNGProvider(searxngURL, 10*time.Second)
		tool := websearch.New(provider)
		if err := registry.Register(&tools.ToolDefinition{Tool: tool, Timeout: 10 * time.Second, MaxOutput: 16 * 1024}); err != nil {
			log.Warn("failed to register websearch tool", "err", err)
		} else {
			log.Info("websearch tool registered", "provider", provider.Name(), "base_url", searxngURL)
		}
	} else {
		searchBaseURL := os.Getenv("SEARCH_BASE_URL")
		searchAPIKey := os.Getenv("SEARCH_API_KEY")
		if searchBaseURL != "" && searchAPIKey != "" {
			provider := websearch.NewHTTPProvider("web_search", searchBaseURL, searchAPIKey, 10*time.Second)
			tool := websearch.New(provider)
			if err := registry.Register(&tools.ToolDefinition{Tool: tool, Timeout: 10 * time.Second, MaxOutput: 16 * 1024}); err != nil {
				log.Warn("failed to register websearch tool", "err", err)
			} else {
				log.Info("websearch tool registered", "provider", provider.Name(), "base_url", searchBaseURL)
			}
		} else {
			log.Warn("websearch tool not registered", "reason", "IRIS_SEARXNG_URL, SEARCH_BASE_URL, or SEARCH_API_KEY unset")
		}
	}

	{
		stickerIndex := memesearch.NewGuildStickerIndex(gateway.Session())
		var social []memesearch.SocialAdapter
		if giphyKey := os.Getenv("GIPHY_API_KEY"); giphyKey != "" {
			social = append(social, memesearch.NewGiphyAdapter(giphyKey))
			log.Info("giphy adapter registered")
		} else {
			log.Info("giphy adapter not registered", "reason", "GIPHY_API_KEY unset")
		}
		memeSearchTool := memesearch.New(nil, stickerIndex, social, memesearch.NewDefaultSafetyClassifier())
		if err := registry.Register(&tools.ToolDefinition{Tool: memeSearchTool, Timeout: 10 * time.Second, MaxOutput: 8 * 1024}); err != nil {
			log.Warn("failed to register meme_search tool", "err", err)
		} else {
			log.Info("meme_search tool registered", "stickers", true, "giphy", len(social) > 0)
		}
	}

	mcpConfigPath := os.Getenv("MCP_CONFIG_PATH")
	if mcpConfigPath == "" {
		mcpConfigPath = "mcps.json"
	}
	var mcpOwnerID int64
	if s := os.Getenv("IRIS_OWNER_ID"); s != "" {
		if id, parseErr := strconv.ParseInt(s, 10, 64); parseErr == nil {
			mcpOwnerID = id
		} else {
			log.Warn("IRIS_OWNER_ID is set but unparseable", "value", s, "err", parseErr)
		}
	}
	mcpSupervisor, mcpErr := mcp.NewSupervisor(ctx, mcpConfigPath, mcpOwnerID, registry)
	if mcpErr != nil {
		log.Warn("mcp supervisor init failed", "path", mcpConfigPath, "err", mcpErr)
	} else {
		log.Info("mcp supervisor ready", "path", mcpConfigPath, "owner_id", mcpOwnerID)
		if mcpOwnerID != 0 {
			if err := registry.Register(&tools.ToolDefinition{Tool: mcp.NewAddTool(mcpSupervisor), Timeout: 60 * time.Second, MaxOutput: 2048}); err != nil {
				log.Warn("failed to register mcp_add tool", "err", err)
			}
			if err := registry.Register(&tools.ToolDefinition{Tool: mcp.NewRemoveTool(mcpSupervisor), Timeout: 10 * time.Second, MaxOutput: 2048}); err != nil {
				log.Warn("failed to register mcp_remove tool", "err", err)
			}
			if err := registry.Register(&tools.ToolDefinition{Tool: mcp.NewListTool(mcpSupervisor), Timeout: 5 * time.Second, MaxOutput: 4096}); err != nil {
				log.Warn("failed to register mcp_list tool", "err", err)
			}
		}
	}
	if mcpSupervisor != nil {
		defer mcpSupervisor.Close()
	}

	log.Info("tools registered", "count", len(registry.OpenAIFunctions()))

	userBehaviorRepo := repository.NewUserBehaviorRepo(db)

	var guildRecallSvc *memory.GuildRecallService
	if memoryRuntime.RecallEnabled {
		guildRecallSvc, err = memory.NewGuildRecallService(
			memoryRuntime.Embedder,
			channelMessageRepo,
			memoryRuntime.RecallConfig,
		)
		if err != nil {
			log.Error("failed to create guild recall service", "err", err)
			os.Exit(1)
		}
	} else {
		log.Info("guild recall wiring skipped", "reason", "memory disabled")
	}

	behaviorProfileSvc, err := memory.NewBehaviorProfileService(userBehaviorRepo)
	if err != nil {
		log.Error("failed to create behavior profile service", "err", err)
		os.Exit(1)
	}

	behaviorUpdateAdapter := wireadapters.NewBehaviorProfileUpdateAdapter(
		behaviorProfileSvc,
		5,
		10*time.Minute,
	)

	var loreCapturer orchestrator.LoreCapturer
	var loreWorker *lorethread.Worker
	if cfg.LoreThreadsEnabled {
		loreSessionStoreAdapter := wireadapters.NewLoreSessionStoreAdapter(loreSessionRepo)
		loreSettingsStoreAdapter := wireadapters.NewLoreSettingsStoreAdapter(loreGuildSettingsRepo)
		loreClassifier := &wireadapters.LoreClassifierAdapter{
			Client: chatClient,
			Model:  cfg.LoreLLMModel,
		}

		loreCapturer = wireadapters.NewLoreCapturer(
			lorethread.NewCapturer(lorethread.CapturerDeps{
				SessionStore:       loreSessionStoreAdapter,
				GuildSettingsStore: loreSettingsStoreAdapter,
				LoreClassifier:     loreClassifier,
				Clock:              &lorethread.RealClock{},
				IdleDuration:       cfg.LoreIdleDuration,
				Enabled:            cfg.LoreThreadsEnabled,
			}),
			allowedRepo,
			botID,
		)

		loreSummarizer := lorethread.NewLLMSummarizer(
			&wireadapters.LoreThreadLLMCallerAdapter{
				Client: chatClient,
				Model:  cfg.LoreLLMModel,
			},
			lorethread.NewDefaultRedactor(),
			cfg.LoreLLMTimeout,
		)
		loreTitleGen := lorethread.NewLLMTitleGenerator(
			&wireadapters.LoreThreadLLMCallerAdapter{
				Client: chatClient,
				Model:  cfg.LoreLLMModel,
			},
			&lorethread.RealClock{},
			cfg.LoreLLMTimeout,
		)
		loreFetcher := wireadapters.NewLoreMessageFetcherAdapter(channelMessageRepo)
		loreThreadCreator := wireadapters.NewDiscordThreadCreator(gateway)
		loreLimiter := wireadapters.NewHourlyLimiter(cfg.LoreThreadCapPerHour, &lorethread.RealClock{})
		loreAnchorStore := wireadapters.NewLoreThreadAnchorStoreAdapter(loreThreadAnchorRepo, loreSessionRepo)

		loreWorker = &lorethread.Worker{
			SessionStore:       loreSessionStoreAdapter,
			ThreadAnchorStore:  loreAnchorStore,
			GuildSettingsStore: loreSettingsStoreAdapter,
			LoreSummarizer:     loreSummarizer,
			TitleGenerator:     loreTitleGen,
			ThreadCreator:      loreThreadCreator,
			MessageFetcher:     loreFetcher,
			Clock:              &lorethread.RealClock{},
			Limiter:            loreLimiter,
			LoreClassifier:     loreClassifier,
			PollInterval:       cfg.LoreWorkerPollInterval,
			LLMTimeout:         cfg.LoreLLMTimeout,
			ThreadCapPerHour:   cfg.LoreThreadCapPerHour,
		}
		if err := loreWorker.Start(ctx); err != nil {
			log.Warn("lore worker start failed", "err", err)
		} else {
			defer loreWorker.Stop()
			log.Info("lore_worker_started", "poll_interval", cfg.LoreWorkerPollInterval.String(), "llm_timeout", cfg.LoreLLMTimeout.String())
		}

		// Register lore finalize tool
		loreFinalizer := &lorethread.Finalizer{
			SessionStore:       loreSessionStoreAdapter,
			MessageFetcher:     loreFetcher,
			LoreSummarizer:     loreSummarizer,
			TitleGenerator:     loreTitleGen,
			ThreadCreator:      loreThreadCreator,
			ThreadAnchorStore:  loreAnchorStore,
			Clock:              &lorethread.RealClock{},
			Limiter:            loreLimiter,
			GuildSettingsStore: loreSettingsStoreAdapter,
			LoreClassifier:     loreClassifier,
			MetricsHooks:       loreWorker.MetricsHooks,
		}
		loreFinalizeTool := lorethread_tool.New(loreFinalizer)
		if err := registry.Register(&tools.ToolDefinition{Tool: loreFinalizeTool, Timeout: 30 * time.Second, MaxOutput: 2048}); err != nil {
			log.Warn("failed to register lore_finalize_now tool", "err", err)
		} else {
			log.Info("lore_finalize_now tool registered")
		}
	} else {
		log.Info("lore_worker_disabled")
	}

	orchCfg := orchestrator.Config{
		Router:                &wireadapters.DeciderAdapter{Router: routerSvc},
		LLM:                   &wireadapters.LLMCallerAdapter{Client: chatClient},
		Discord:               &wireadapters.DiscordSenderAdapter{Gateway: gateway},
		ContextStore:          &wireadapters.ContextStoreAdapter{Repo: channelMessageRepo},
		AllowedQuerier:        allowedRepo,
		ConversationRefresher: convRepo,
		Capture:               capturePort,
		GuildMemory:           guildRecallSvc,
		CuratedMemory:         &wireadapters.CuratedMemoryAdapter{Repo: memoryRepo, Embedder: emb, MinScore: 0.40},
		UserBehavior:          behaviorProfileSvc,
		BehaviorUpdater:       behaviorUpdateAdapter,
		LoreCapturer:          loreCapturer,
		ConversationLockTTL:   cfg.ConversationLockTTL,
		InWindowRelevance:     inWindowRelevance,
		CrossChannel:          crossChannelClassifier,
		Promoter:              memoryPromoter,
		SystemPrompt:          persona.BuildSystemPrompt(persona.PromptInput{}),
		QueueSize:             128,
		WorkerCount:           4,
		EnqueueLimit:          10 * time.Millisecond,
		DedupeTTL:             30 * time.Second,
		TypingRepeat:          5 * time.Second,
		JobTimeout:            5 * time.Minute,
		ImmediateTyping:       true,
		TierRouter:            tierRouter,
		ToolsLLM:              &wireadapters.ToolCallingLLMAdapter{Client: toolsClient},
		ToolExecutor:          wireadapters.NewEscalationAwareExecutor(&wireadapters.RegistryExecutor{Reg: registry}),
		Tools:                 registry.OpenAIFunctions(),
		Streaming:             cfg.Streaming,
		StreamLLM:             &wireadapters.StreamLLMAdapter{Client: chatClient},
		StreamToolsLLM:        &wireadapters.StreamToolsLLMAdapter{Client: toolsClient},
		RateLimiter:           orchestrator.NewChannelRateLimiter(5, 5*time.Second),
		StrongModel:           cfg.LLMModelStrong,
		AllowedChannelLister:  &wireadapters.AllowedChannelListerAdapter{Repo: allowedRepo},
		ChannelNameResolver:   wireadapters.NewSessionChannelNameResolver(gateway.Session()),
		Compactor:             &wireadapters.LLMCompactor{Client: chatClient, Model: cfg.LLMModelDefault},
		EpisodeArchiver:       &wireadapters.EpisodeArchiverAdapter{Repo: episodeRepo, Embedder: emb, EmbeddingModel: "onnx-e5-small-384"},
		LoreAnchorResolver:    &wireadapters.LoreThreadAnchorResolverAdapter{Repo: loreThreadAnchorRepo},
		LoreContext:           &wireadapters.WikiLoreContextAdapter{Retriever: retriever},
		LoreCaptureTimeout:    cfg.LoreCaptureTimeout,
	}

	// Wire lore compactor if lore threads are enabled
	if cfg.LoreThreadsEnabled {
		loreCompactionLimit := 40000
		orchCfg.LoreCompactor = &orchestrator.LoreCompactor{
			Limit:           loreCompactionLimit,
			RetentionTarget: cfg.LoreCompactionTarget,
			Archiver:        &wireadapters.EpisodeArchiverAdapter{Repo: episodeRepo, Embedder: emb, EmbeddingModel: "onnx-e5-small-384"},
			LLMCompactor:    &wireadapters.LLMCompactor{Client: chatClient, Model: cfg.LoreLLMModel},
		}
	}
	orch = orchestrator.New(orchCfg)
	orch.Start()
	defer orch.Stop()

	slashStore, slashErr := slash.NewStore(mcpConfigPath)
	if slashErr != nil {
		log.Warn("slash store init failed", "err", slashErr)
	}
	var slashRegistrar *slash.Registrar
	if slashStore != nil {
		loreSettingsHandler := slash.NewLoreSettingsHandler(loreGuildSettingsRepo)
		natives := slash.NewNativeCommands(exceptionHandler, allowedChannelHandler, nil, nil, loreSettingsHandler)
		slashRegistrar = slash.NewRegistrar(slashStore, natives)
		toolExec := registry
		router := slash.NewRouter(slashRegistrar, toolExec, mcpOwnerID, mcp.WithCallerUserID)
		router.SetSynthesizer(&wireadapters.SynthesizerAdapter{Client: chatClient, Model: cfg.LLMModelDefault})
		gateway.SetInteractionHandler(router)
		gateway.SetGuildAvailableCallback(func(ctx context.Context, guildID int64) {
			if slashRegistrar == nil {
				return
			}
			if err := slashRegistrar.RegisterGuild(ctx, guildID); err != nil {
				log.Warn("slash register guild failed", "guild", guildID, "err", err)
			}
		})
		slashStore.OnMutate(func() {
			if slashRegistrar == nil {
				return
			}
			slashRegistrar.ReloadAll(context.Background())
		})
		if mcpOwnerID != 0 {
			if err := registry.Register(&tools.ToolDefinition{Tool: slash.NewBindSlashTool(slashStore, mcpOwnerID), Timeout: 10 * time.Second, MaxOutput: 2048}); err != nil {
				log.Warn("failed to register mcp_bind_slash tool", "err", err)
			}
			if err := registry.Register(&tools.ToolDefinition{Tool: slash.NewUnbindSlashTool(slashStore, mcpOwnerID), Timeout: 10 * time.Second, MaxOutput: 2048}); err != nil {
				log.Warn("failed to register mcp_unbind_slash tool", "err", err)
			}
			if err := registry.Register(&tools.ToolDefinition{Tool: slash.NewListSlashBindingsTool(slashStore, mcpOwnerID), Timeout: 5 * time.Second, MaxOutput: 8192}); err != nil {
				log.Warn("failed to register mcp_list_slash tool", "err", err)
			}
		}
	}

	if mcpOwnerID != 0 {
		if err := registry.Register(&tools.ToolDefinition{Tool: modelswitch.NewSetTool(modelResolver, mcpOwnerID, auditRepo), Timeout: 5 * time.Second, MaxOutput: 1024}); err != nil {
			log.Warn("failed to register iris_set_model tool", "err", err)
		}
		if err := registry.Register(&tools.ToolDefinition{Tool: modelswitch.NewResetTool(modelResolver, mcpOwnerID, auditRepo), Timeout: 5 * time.Second, MaxOutput: 1024}); err != nil {
			log.Warn("failed to register iris_reset_model tool", "err", err)
		}
	}
	if err := registry.Register(&tools.ToolDefinition{Tool: modelswitch.NewGetTool(modelResolver), Timeout: 2 * time.Second, MaxOutput: 1024}); err != nil {
		log.Warn("failed to register iris_get_models tool", "err", err)
	}

	log.Info("all tools registered", "count", len(registry.OpenAIFunctions()), "names", registry.List())

	if err := gateway.Connect(ctx); err != nil {
		log.Error("failed to connect Discord gateway", "err", err)
		os.Exit(1)
	}
	log.Info("Discord gateway connected")

	if botID == 0 {
		derivedBotID := gateway.BotID()
		if derivedBotID == 0 {
			log.Warn("failed to derive bot ID from session, botID remains 0")
		} else {
			botID = derivedBotID
			log.Info("auto-derived bot ID from session", "botID", botID)
			gateway.SetNormalizerBotID(botID)
			routerSvc.SetBotID(botID)
		}
	}

	if slashRegistrar != nil && botID != 0 {
		slashRegistrar.Attach(gateway.Session(), strconv.FormatInt(botID, 10))
		log.Info("slash registrar attached", "bot_id", botID)
	}

	// Shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	log.Info("shutdown signal received")
	cancel()
	if err := gateway.Close(); err != nil {
		log.Error("gateway close error", "err", err)
	}
	pool.Close()
	log.Info("iris-bot stopped")
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

type captureChain struct {
	primary   orchestrator.ChannelCapture
	secondary orchestrator.ChannelCapture
}

func (c *captureChain) Capture(ctx context.Context, msg *domain.ChannelMessage) error {
	if c.primary != nil {
		if err := c.primary.Capture(ctx, msg); err != nil {
			return err
		}
	}
	if c.secondary != nil {
		if err := c.secondary.Capture(ctx, msg); err != nil {
			return err
		}
	}
	return nil
}

type enqueueIfMissingCapture struct {
	next orchestrator.ChannelCapture
}

func (c *enqueueIfMissingCapture) Capture(ctx context.Context, msg *domain.ChannelMessage) error {
	if msg == nil || strings.TrimSpace(msg.Content) == "" {
		return nil
	}
	if len(msg.ContentEmbedding) > 0 {
		return nil
	}
	if c.next == nil {
		return nil
	}
	return c.next.Capture(ctx, msg)
}
