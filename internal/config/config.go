package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DiscordToken        string
	OpenAIAPIKey        string
	DatabaseURL         string
	PostgresHost        string
	PostgresPort        string
	PostgresUser        string
	PostgresPassword    string
	PostgresDB          string
	LLMModel            string
	LLMModelRouter      string
	LLMModelDefault     string
	LLMModelStrong      string
	LLMBaseURL          string
	LLMTemperature      float32
	LLMMaxTokens        int
	LLMTimeout          time.Duration
	LLMChatTimeout      time.Duration
	LLMToolTimeout      time.Duration
	LLMMaxRetries       int
	LLMRetryDelay       time.Duration
	ConversationLockTTL time.Duration
	EmbedModelPath      string
	EmbedTokenizerPath  string
	EmbedSimThreshold   float64
	Debug               bool
	Streaming           bool

	// Server-shared persistent memory settings (per-guild vector recall).
	MemoryServer MemoryServerConfig

	// Lore thread protocol settings
	LoreThreadsEnabled      bool
	LoreIdleDuration        time.Duration
	LoreCompactionTarget    float64
	LoreThreadCapPerHour    int
	LoreWorkerPollInterval  time.Duration
	LoreLLMTimeout          time.Duration
	LoreLLMModel            string
	LoreCaptureTimeout      time.Duration
}

// MemoryServerConfig controls guild-shared long-term memory and per-user
// behavior recognition. All fields are scoped by (guild_id) or (guild_id,
// user_id); nothing here is ever used across guilds.
type MemoryServerConfig struct {
	Enabled            bool
	RecallThreshold    float64
	RecallTopK         int
	EmbedBatchSize     int
	EmbedWorkers       int
	EmbedBackfillLimit int
}

func Load() (*Config, error) {
	requiredVars := []string{
		"DISCORD_TOKEN",
		"OPENAI_API_KEY",
		"DATABASE_URL",
		"POSTGRES_HOST",
		"POSTGRES_PORT",
		"POSTGRES_USER",
		"POSTGRES_PASSWORD",
		"POSTGRES_DB",
	}

	for _, v := range requiredVars {
		if os.Getenv(v) == "" {
			return nil, fmt.Errorf("missing required env var: %s", v)
		}
	}

	llmModel := os.Getenv("LLM_MODEL")
	if llmModel == "" {
		llmModel = "gpt-4"
	}

	llmBaseURL := os.Getenv("LLM_BASE_URL")
	if llmBaseURL == "" {
		llmBaseURL = "https://api.openai.com"
	}

	llmTemperature := float32(0.7)
	if temp := os.Getenv("LLM_TEMPERATURE"); temp != "" {
		if f, err := strconv.ParseFloat(temp, 32); err == nil {
			llmTemperature = float32(f)
		}
	}

	llmMaxTokens := 2048
	if maxTokens := os.Getenv("LLM_MAX_TOKENS"); maxTokens != "" {
		if n, err := strconv.Atoi(maxTokens); err == nil {
			llmMaxTokens = n
		}
	}

	// LLM_TIMEOUT is the legacy fallback; default changed to 2m for backward compat
	llmTimeout := 2 * time.Minute
	if timeout := os.Getenv("LLM_TIMEOUT"); timeout != "" {
		if d, err := time.ParseDuration(timeout); err == nil {
			llmTimeout = d
		}
	}

	// LLM_CHAT_TIMEOUT: timeout for plain chat calls. Falls back to LLM_TIMEOUT, then 2m.
	llmChatTimeout := llmTimeout
	if chatTimeout := os.Getenv("LLM_CHAT_TIMEOUT"); chatTimeout != "" {
		if d, err := time.ParseDuration(chatTimeout); err == nil {
			llmChatTimeout = d
		}
	}
	// If neither LLM_CHAT_TIMEOUT nor LLM_TIMEOUT set, use 2m
	if llmChatTimeout == 0 {
		llmChatTimeout = 2 * time.Minute
	}

	// LLM_TOOL_TIMEOUT: timeout for tool-call streams. Falls back to LLM_TIMEOUT * 4 (rounded up to minute), then 10m.
	llmToolTimeout := 10 * time.Minute
	if toolTimeout := os.Getenv("LLM_TOOL_TIMEOUT"); toolTimeout != "" {
		if d, err := time.ParseDuration(toolTimeout); err == nil {
			llmToolTimeout = d
		}
	} else if os.Getenv("LLM_TIMEOUT") != "" {
		fallback := llmTimeout * 4
		if fallback%time.Minute != 0 {
			fallback = ((fallback / time.Minute) + 1) * time.Minute
		}
		llmToolTimeout = fallback
	}

	llmMaxRetries := 3
	if retries := os.Getenv("LLM_MAX_RETRIES"); retries != "" {
		if n, err := strconv.Atoi(retries); err == nil {
			llmMaxRetries = n
		}
	}

	llmRetryDelay := 1 * time.Second
	if delay := os.Getenv("LLM_RETRY_DELAY"); delay != "" {
		if d, err := time.ParseDuration(delay); err == nil {
			llmRetryDelay = d
		}
	}

	convLockTTL := 5 * time.Minute
	if ttl := os.Getenv("IRIS_CONV_LOCK_TTL"); ttl != "" {
		if d, err := time.ParseDuration(ttl); err == nil {
			convLockTTL = d
		}
	}

	// Tier model configuration with defaults
	llmModelRouter := os.Getenv("LLM_MODEL_ROUTER")
	if llmModelRouter == "" {
		llmModelRouter = "kr/claude-haiku-4.5"
	}

	llmModelDefault := os.Getenv("LLM_MODEL_DEFAULT")
	if llmModelDefault == "" {
		// Fall back to LLM_MODEL if LLM_MODEL_DEFAULT not set
		llmModelDefault = llmModel
		if llmModelDefault == "" {
			llmModelDefault = "kr/claude-sonnet-4.5"
		}
	}

	llmModelStrong := os.Getenv("LLM_MODEL_STRONG")
	if llmModelStrong == "" {
		llmModelStrong = "kr/claude-opus-4.7"
	}

	if err := ValidateModelName(llmModelRouter); err != nil {
		return nil, fmt.Errorf("invalid LLM_MODEL_ROUTER: %w", err)
	}
	if err := ValidateModelName(llmModelDefault); err != nil {
		return nil, fmt.Errorf("invalid LLM_MODEL_DEFAULT: %w", err)
	}
	if err := ValidateModelName(llmModelStrong); err != nil {
		return nil, fmt.Errorf("invalid LLM_MODEL_STRONG: %w", err)
	}

	// Parse DEBUG flag
	debug := false
	if debugStr := os.Getenv("DEBUG"); debugStr != "" {
		debugStr = strings.ToLower(strings.TrimSpace(debugStr))
		debug = debugStr == "true" || debugStr == "1"
	}

	// Parse embedding config
	embedModelPath := os.Getenv("IRIS_EMBED_MODEL_PATH")
	embedTokenizerPath := os.Getenv("IRIS_EMBED_TOKENIZER_PATH")

	embedSimThreshold := 0.55
	if threshStr := os.Getenv("IRIS_EMBED_SIM_THRESHOLD"); threshStr != "" {
		if f, err := strconv.ParseFloat(threshStr, 64); err == nil {
			embedSimThreshold = f
		}
	}

	streaming := true
	if streamStr := os.Getenv("IRIS_STREAMING"); streamStr != "" {
		streamStr = strings.ToLower(strings.TrimSpace(streamStr))
		streaming = streamStr != "false" && streamStr != "0"
	}

	memoryServer := loadMemoryServerConfig()

	loreThreadsEnabled := false
	if v := os.Getenv("IRIS_LORE_THREADS_ENABLED"); v != "" {
		v = strings.ToLower(strings.TrimSpace(v))
		loreThreadsEnabled = v == "true" || v == "1"
	}

	loreIdleDuration := 5 * time.Minute
	if v := os.Getenv("IRIS_LORE_IDLE_DURATION"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			loreIdleDuration = d
		}
	}

	loreCompactionTarget := 0.70
	if v := os.Getenv("IRIS_LORE_COMPACTION_TARGET"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 && f <= 1 {
			loreCompactionTarget = f
		}
	}

	loreThreadCapPerHour := 6
	if v := os.Getenv("IRIS_LORE_THREAD_CAP_PER_HOUR"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			loreThreadCapPerHour = n
		}
	}

	loreWorkerPollInterval := 30 * time.Second
	if v := os.Getenv("IRIS_LORE_WORKER_POLL_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			loreWorkerPollInterval = d
		}
	}

	loreLLMTimeout := 30 * time.Second
	if v := os.Getenv("IRIS_LORE_LLM_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			loreLLMTimeout = d
		}
	}

	loreLLMModel := os.Getenv("IRIS_LORE_LLM_MODEL")
	if loreLLMModel == "" {
		loreLLMModel = llmModelStrong
	}
	if err := ValidateModelName(loreLLMModel); err != nil {
		return nil, fmt.Errorf("invalid IRIS_LORE_LLM_MODEL: %w", err)
	}

	loreCaptureTimeout := 60 * time.Second
	if v := os.Getenv("IRIS_LORE_CAPTURE_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			loreCaptureTimeout = d
		}
	}

	return &Config{
		DiscordToken:        os.Getenv("DISCORD_TOKEN"),
		OpenAIAPIKey:        os.Getenv("OPENAI_API_KEY"),
		DatabaseURL:         os.Getenv("DATABASE_URL"),
		PostgresHost:        os.Getenv("POSTGRES_HOST"),
		PostgresPort:        os.Getenv("POSTGRES_PORT"),
		PostgresUser:        os.Getenv("POSTGRES_USER"),
		PostgresPassword:    os.Getenv("POSTGRES_PASSWORD"),
		PostgresDB:          os.Getenv("POSTGRES_DB"),
		LLMModel:            llmModel,
		LLMModelRouter:      llmModelRouter,
		LLMModelDefault:     llmModelDefault,
		LLMModelStrong:      llmModelStrong,
		LLMBaseURL:          llmBaseURL,
		LLMTemperature:      llmTemperature,
		LLMMaxTokens:        llmMaxTokens,
		LLMTimeout:          llmTimeout,
		LLMChatTimeout:      llmChatTimeout,
		LLMToolTimeout:      llmToolTimeout,
		LLMMaxRetries:       llmMaxRetries,
		LLMRetryDelay:       llmRetryDelay,
		ConversationLockTTL: convLockTTL,
		EmbedModelPath:      embedModelPath,
		EmbedTokenizerPath:  embedTokenizerPath,
		EmbedSimThreshold:   embedSimThreshold,
		Debug:               debug,
		Streaming:           streaming,
		MemoryServer:        memoryServer,
		LoreThreadsEnabled:      loreThreadsEnabled,
		LoreIdleDuration:        loreIdleDuration,
		LoreCompactionTarget:    loreCompactionTarget,
		LoreThreadCapPerHour:    loreThreadCapPerHour,
		LoreWorkerPollInterval:  loreWorkerPollInterval,
		LoreLLMTimeout:          loreLLMTimeout,
		LoreLLMModel:            loreLLMModel,
		LoreCaptureTimeout:      loreCaptureTimeout,
	}, nil
}

// loadMemoryServerConfig parses MEMORY_SERVER_* env vars. Invalid values fall
// back to defaults so a malformed variable cannot disable safety-critical
// guild isolation.
func loadMemoryServerConfig() MemoryServerConfig {
	cfg := MemoryServerConfig{
		Enabled:            true,
		RecallThreshold:    0.55,
		RecallTopK:         5,
		EmbedBatchSize:     32,
		EmbedWorkers:       1,
		EmbedBackfillLimit: 500,
	}

	if v := os.Getenv("MEMORY_SERVER_ENABLED"); v != "" {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "false", "0", "no", "off":
			cfg.Enabled = false
		case "true", "1", "yes", "on":
			cfg.Enabled = true
		}
	}

	if v := os.Getenv("MEMORY_SERVER_RECALL_THRESHOLD"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid MEMORY_SERVER_RECALL_THRESHOLD %q: %v; using default %.2f\n", v, err, cfg.RecallThreshold)
		} else if f < 0 || f > 1 {
			fmt.Fprintf(os.Stderr, "invalid MEMORY_SERVER_RECALL_THRESHOLD %q: must be in [0,1]; using default %.2f\n", v, cfg.RecallThreshold)
		} else {
			cfg.RecallThreshold = f
		}
	}

	if v := os.Getenv("MEMORY_SERVER_RECALL_TOP_K"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.RecallTopK = n
		}
	}

	if v := os.Getenv("MEMORY_SERVER_EMBED_BATCH_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.EmbedBatchSize = n
		}
	}

	if v := os.Getenv("MEMORY_SERVER_EMBED_WORKERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.EmbedWorkers = n
		}
	}

	if v := os.Getenv("MEMORY_SERVER_EMBED_BACKFILL_LIMIT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			cfg.EmbedBackfillLimit = n
		}
	}

	return cfg
}

// ValidateModelName checks that a model name is acceptable. The only hard
// rule is that eno/* models are explicitly disallowed; any other non-empty
// model name is accepted so operators can point at arbitrary
// OpenAI-compatible providers without rebuilding the bot.
func ValidateModelName(model string) error {
	if strings.TrimSpace(model) == "" {
		return fmt.Errorf("model name must not be empty")
	}
	if strings.HasPrefix(model, "eno/") {
		return fmt.Errorf("eno/* models are not allowed")
	}
	return nil
}
