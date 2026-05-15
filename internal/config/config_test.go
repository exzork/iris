package config

import (
	"os"
	"testing"
	"time"
)

func setMemoryTestBaseEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DISCORD_TOKEN", "test-token")
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("POSTGRES_HOST", "localhost")
	t.Setenv("POSTGRES_PORT", "5432")
	t.Setenv("POSTGRES_USER", "user")
	t.Setenv("POSTGRES_PASSWORD", "pass")
	t.Setenv("POSTGRES_DB", "iris")
	t.Setenv("LLM_MODEL", "kr/claude-sonnet-4.5")
	for _, k := range []string{
		"MEMORY_SERVER_ENABLED",
		"MEMORY_SERVER_RECALL_THRESHOLD",
		"MEMORY_SERVER_RECALL_TOP_K",
		"MEMORY_SERVER_EMBED_BATCH_SIZE",
		"MEMORY_SERVER_EMBED_WORKERS",
		"MEMORY_SERVER_EMBED_BACKFILL_LIMIT",
	} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
}

func TestLoadConfig_MemoryServerDefaults(t *testing.T) {
	setMemoryTestBaseEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	ms := cfg.MemoryServer
	if !ms.Enabled {
		t.Errorf("MemoryServer.Enabled default = false, want true")
	}
	if ms.RecallThreshold != 0.55 {
		t.Errorf("RecallThreshold default = %v, want 0.55", ms.RecallThreshold)
	}
	if ms.RecallTopK != 5 {
		t.Errorf("RecallTopK default = %d, want 5", ms.RecallTopK)
	}
	if ms.EmbedBatchSize != 32 {
		t.Errorf("EmbedBatchSize default = %d, want 32", ms.EmbedBatchSize)
	}
	if ms.EmbedWorkers != 1 {
		t.Errorf("EmbedWorkers default = %d, want 1", ms.EmbedWorkers)
	}
	if ms.EmbedBackfillLimit != 500 {
		t.Errorf("EmbedBackfillLimit default = %d, want 500", ms.EmbedBackfillLimit)
	}
}

func TestLoadConfig_MemoryServerOverrides(t *testing.T) {
	setMemoryTestBaseEnv(t)
	t.Setenv("MEMORY_SERVER_ENABLED", "false")
	t.Setenv("MEMORY_SERVER_RECALL_THRESHOLD", "0.5")
	t.Setenv("MEMORY_SERVER_RECALL_TOP_K", "9")
	t.Setenv("MEMORY_SERVER_EMBED_BATCH_SIZE", "64")
	t.Setenv("MEMORY_SERVER_EMBED_WORKERS", "4")
	t.Setenv("MEMORY_SERVER_EMBED_BACKFILL_LIMIT", "0")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	ms := cfg.MemoryServer
	if ms.Enabled {
		t.Errorf("Enabled override = true, want false")
	}
	if ms.RecallThreshold != 0.5 {
		t.Errorf("RecallThreshold = %v, want 0.5", ms.RecallThreshold)
	}
	if ms.RecallTopK != 9 {
		t.Errorf("RecallTopK = %d, want 9", ms.RecallTopK)
	}
	if ms.EmbedBatchSize != 64 {
		t.Errorf("EmbedBatchSize = %d, want 64", ms.EmbedBatchSize)
	}
	if ms.EmbedWorkers != 4 {
		t.Errorf("EmbedWorkers = %d, want 4", ms.EmbedWorkers)
	}
	if ms.EmbedBackfillLimit != 0 {
		t.Errorf("EmbedBackfillLimit = %d, want 0", ms.EmbedBackfillLimit)
	}
}

func TestLoadConfig_MemoryServerInvalidValuesFallBack(t *testing.T) {
	setMemoryTestBaseEnv(t)
	t.Setenv("MEMORY_SERVER_RECALL_THRESHOLD", "2.5")
	t.Setenv("MEMORY_SERVER_RECALL_TOP_K", "-1")
	t.Setenv("MEMORY_SERVER_EMBED_BATCH_SIZE", "abc")
	t.Setenv("MEMORY_SERVER_EMBED_WORKERS", "0")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	ms := cfg.MemoryServer
	if ms.RecallThreshold != 0.55 {
		t.Errorf("invalid threshold should fall back to 0.55, got %v", ms.RecallThreshold)
	}
	if ms.RecallTopK != 5 {
		t.Errorf("invalid top_k should fall back to 5, got %d", ms.RecallTopK)
	}
	if ms.EmbedBatchSize != 32 {
		t.Errorf("invalid batch_size should fall back to 32, got %d", ms.EmbedBatchSize)
	}
	if ms.EmbedWorkers != 1 {
		t.Errorf("invalid workers should fall back to 1, got %d", ms.EmbedWorkers)
	}
}

func TestLoadConfig_Success(t *testing.T) {
	os.Setenv("DISCORD_TOKEN", "test-token")
	os.Setenv("OPENAI_API_KEY", "test-key")
	os.Setenv("DATABASE_URL", "postgres://localhost/test")
	os.Setenv("POSTGRES_HOST", "localhost")
	os.Setenv("POSTGRES_PORT", "5432")
	os.Setenv("POSTGRES_USER", "user")
	os.Setenv("POSTGRES_PASSWORD", "pass")
	os.Setenv("POSTGRES_DB", "iris")
	os.Setenv("LLM_MODEL", "kr/claude-sonnet-4.5")
	defer func() {
		os.Unsetenv("DISCORD_TOKEN")
		os.Unsetenv("OPENAI_API_KEY")
		os.Unsetenv("DATABASE_URL")
		os.Unsetenv("POSTGRES_HOST")
		os.Unsetenv("POSTGRES_PORT")
		os.Unsetenv("POSTGRES_USER")
		os.Unsetenv("POSTGRES_PASSWORD")
		os.Unsetenv("POSTGRES_DB")
		os.Unsetenv("LLM_MODEL")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.DiscordToken != "test-token" {
		t.Errorf("expected DiscordToken=test-token, got %s", cfg.DiscordToken)
	}
	if cfg.OpenAIAPIKey != "test-key" {
		t.Errorf("expected OpenAIAPIKey=test-key, got %s", cfg.OpenAIAPIKey)
	}
}

func TestLoadConfig_MissingDiscordToken(t *testing.T) {
	os.Unsetenv("DISCORD_TOKEN")
	os.Setenv("OPENAI_API_KEY", "test-key")
	os.Setenv("DATABASE_URL", "postgres://localhost/test")
	os.Setenv("POSTGRES_HOST", "localhost")
	os.Setenv("POSTGRES_PORT", "5432")
	os.Setenv("POSTGRES_USER", "user")
	os.Setenv("POSTGRES_PASSWORD", "pass")
	os.Setenv("POSTGRES_DB", "iris")
	defer func() {
		os.Unsetenv("OPENAI_API_KEY")
		os.Unsetenv("DATABASE_URL")
		os.Unsetenv("POSTGRES_HOST")
		os.Unsetenv("POSTGRES_PORT")
		os.Unsetenv("POSTGRES_USER")
		os.Unsetenv("POSTGRES_PASSWORD")
		os.Unsetenv("POSTGRES_DB")
	}()

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing DISCORD_TOKEN, got nil")
	}
	if err.Error() != "missing required env var: DISCORD_TOKEN" {
		t.Errorf("expected specific error message, got %v", err)
	}
}

func TestLoadConfig_MissingOpenAIKey(t *testing.T) {
	os.Setenv("DISCORD_TOKEN", "test-token")
	os.Unsetenv("OPENAI_API_KEY")
	os.Setenv("DATABASE_URL", "postgres://localhost/test")
	os.Setenv("POSTGRES_HOST", "localhost")
	os.Setenv("POSTGRES_PORT", "5432")
	os.Setenv("POSTGRES_USER", "user")
	os.Setenv("POSTGRES_PASSWORD", "pass")
	os.Setenv("POSTGRES_DB", "iris")
	defer func() {
		os.Unsetenv("DISCORD_TOKEN")
		os.Unsetenv("DATABASE_URL")
		os.Unsetenv("POSTGRES_HOST")
		os.Unsetenv("POSTGRES_PORT")
		os.Unsetenv("POSTGRES_USER")
		os.Unsetenv("POSTGRES_PASSWORD")
		os.Unsetenv("POSTGRES_DB")
	}()

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing OPENAI_API_KEY, got nil")
	}
}

func TestEmbedConfig_DefaultsWhenUnset(t *testing.T) {
	os.Setenv("DISCORD_TOKEN", "test-token")
	os.Setenv("OPENAI_API_KEY", "test-key")
	os.Setenv("DATABASE_URL", "postgres://localhost/test")
	os.Setenv("POSTGRES_HOST", "localhost")
	os.Setenv("POSTGRES_PORT", "5432")
	os.Setenv("POSTGRES_USER", "user")
	os.Setenv("POSTGRES_PASSWORD", "pass")
	os.Setenv("POSTGRES_DB", "iris")
	os.Setenv("LLM_MODEL_DEFAULT", "kr/claude-sonnet-4.5")
	os.Unsetenv("IRIS_EMBED_MODEL_PATH")
	os.Unsetenv("IRIS_EMBED_TOKENIZER_PATH")
	os.Unsetenv("IRIS_EMBED_SIM_THRESHOLD")
	defer func() {
		os.Unsetenv("DISCORD_TOKEN")
		os.Unsetenv("OPENAI_API_KEY")
		os.Unsetenv("DATABASE_URL")
		os.Unsetenv("POSTGRES_HOST")
		os.Unsetenv("POSTGRES_PORT")
		os.Unsetenv("POSTGRES_USER")
		os.Unsetenv("POSTGRES_PASSWORD")
		os.Unsetenv("POSTGRES_DB")
		os.Unsetenv("LLM_MODEL_DEFAULT")
		os.Unsetenv("IRIS_EMBED_MODEL_PATH")
		os.Unsetenv("IRIS_EMBED_TOKENIZER_PATH")
		os.Unsetenv("IRIS_EMBED_SIM_THRESHOLD")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.EmbedModelPath != "" {
		t.Errorf("expected EmbedModelPath empty, got %s", cfg.EmbedModelPath)
	}
	if cfg.EmbedTokenizerPath != "" {
		t.Errorf("expected EmbedTokenizerPath empty, got %s", cfg.EmbedTokenizerPath)
	}
	if cfg.EmbedSimThreshold != 0.55 {
		t.Errorf("expected EmbedSimThreshold=0.55, got %f", cfg.EmbedSimThreshold)
	}
}

func TestEmbedConfig_PathsAndThresholdSet(t *testing.T) {
	os.Setenv("DISCORD_TOKEN", "test-token")
	os.Setenv("OPENAI_API_KEY", "test-key")
	os.Setenv("DATABASE_URL", "postgres://localhost/test")
	os.Setenv("POSTGRES_HOST", "localhost")
	os.Setenv("POSTGRES_PORT", "5432")
	os.Setenv("POSTGRES_USER", "user")
	os.Setenv("POSTGRES_PASSWORD", "pass")
	os.Setenv("POSTGRES_DB", "iris")
	os.Setenv("LLM_MODEL_DEFAULT", "kr/claude-sonnet-4.5")
	os.Setenv("IRIS_EMBED_MODEL_PATH", "/opt/models/model.onnx")
	os.Setenv("IRIS_EMBED_TOKENIZER_PATH", "/opt/models/tokenizer.json")
	os.Setenv("IRIS_EMBED_SIM_THRESHOLD", "0.75")
	defer func() {
		os.Unsetenv("DISCORD_TOKEN")
		os.Unsetenv("OPENAI_API_KEY")
		os.Unsetenv("DATABASE_URL")
		os.Unsetenv("POSTGRES_HOST")
		os.Unsetenv("POSTGRES_PORT")
		os.Unsetenv("POSTGRES_USER")
		os.Unsetenv("POSTGRES_PASSWORD")
		os.Unsetenv("POSTGRES_DB")
		os.Unsetenv("LLM_MODEL_DEFAULT")
		os.Unsetenv("IRIS_EMBED_MODEL_PATH")
		os.Unsetenv("IRIS_EMBED_TOKENIZER_PATH")
		os.Unsetenv("IRIS_EMBED_SIM_THRESHOLD")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.EmbedModelPath != "/opt/models/model.onnx" {
		t.Errorf("expected EmbedModelPath=/opt/models/model.onnx, got %s", cfg.EmbedModelPath)
	}
	if cfg.EmbedTokenizerPath != "/opt/models/tokenizer.json" {
		t.Errorf("expected EmbedTokenizerPath=/opt/models/tokenizer.json, got %s", cfg.EmbedTokenizerPath)
	}
	if cfg.EmbedSimThreshold != 0.75 {
		t.Errorf("expected EmbedSimThreshold=0.75, got %f", cfg.EmbedSimThreshold)
	}
}

func TestEmbedConfig_InvalidThresholdFallsBack(t *testing.T) {
	os.Setenv("DISCORD_TOKEN", "test-token")
	os.Setenv("OPENAI_API_KEY", "test-key")
	os.Setenv("DATABASE_URL", "postgres://localhost/test")
	os.Setenv("POSTGRES_HOST", "localhost")
	os.Setenv("POSTGRES_PORT", "5432")
	os.Setenv("POSTGRES_USER", "user")
	os.Setenv("POSTGRES_PASSWORD", "pass")
	os.Setenv("POSTGRES_DB", "iris")
	os.Setenv("LLM_MODEL_DEFAULT", "kr/claude-sonnet-4.5")
	os.Setenv("IRIS_EMBED_SIM_THRESHOLD", "not-a-number")
	defer func() {
		os.Unsetenv("DISCORD_TOKEN")
		os.Unsetenv("OPENAI_API_KEY")
		os.Unsetenv("DATABASE_URL")
		os.Unsetenv("POSTGRES_HOST")
		os.Unsetenv("POSTGRES_PORT")
		os.Unsetenv("POSTGRES_USER")
		os.Unsetenv("POSTGRES_PASSWORD")
		os.Unsetenv("POSTGRES_DB")
		os.Unsetenv("LLM_MODEL_DEFAULT")
		os.Unsetenv("IRIS_EMBED_SIM_THRESHOLD")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.EmbedSimThreshold != 0.55 {
		t.Errorf("expected EmbedSimThreshold=0.55 (fallback), got %f", cfg.EmbedSimThreshold)
	}
}

func TestLoadConfig_MissingDatabaseURL(t *testing.T) {
	os.Setenv("DISCORD_TOKEN", "test-token")
	os.Setenv("OPENAI_API_KEY", "test-key")
	os.Unsetenv("DATABASE_URL")
	os.Setenv("POSTGRES_HOST", "localhost")
	os.Setenv("POSTGRES_PORT", "5432")
	os.Setenv("POSTGRES_USER", "user")
	os.Setenv("POSTGRES_PASSWORD", "pass")
	os.Setenv("POSTGRES_DB", "iris")
	defer func() {
		os.Unsetenv("DISCORD_TOKEN")
		os.Unsetenv("OPENAI_API_KEY")
		os.Unsetenv("POSTGRES_HOST")
		os.Unsetenv("POSTGRES_PORT")
		os.Unsetenv("POSTGRES_USER")
		os.Unsetenv("POSTGRES_PASSWORD")
		os.Unsetenv("POSTGRES_DB")
	}()

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing DATABASE_URL, got nil")
	}
	if err.Error() != "missing required env var: DATABASE_URL" {
		t.Errorf("expected specific error message, got %v", err)
	}
}

func TestDebugParsing_True(t *testing.T) {
	os.Setenv("DISCORD_TOKEN", "test-token")
	os.Setenv("OPENAI_API_KEY", "test-key")
	os.Setenv("DATABASE_URL", "postgres://localhost/test")
	os.Setenv("POSTGRES_HOST", "localhost")
	os.Setenv("POSTGRES_PORT", "5432")
	os.Setenv("POSTGRES_USER", "user")
	os.Setenv("POSTGRES_PASSWORD", "pass")
	os.Setenv("POSTGRES_DB", "iris")
	os.Setenv("LLM_MODEL_DEFAULT", "kr/claude-sonnet-4.5")
	os.Setenv("DEBUG", "true")
	defer func() {
		os.Unsetenv("DISCORD_TOKEN")
		os.Unsetenv("OPENAI_API_KEY")
		os.Unsetenv("DATABASE_URL")
		os.Unsetenv("POSTGRES_HOST")
		os.Unsetenv("POSTGRES_PORT")
		os.Unsetenv("POSTGRES_USER")
		os.Unsetenv("POSTGRES_PASSWORD")
		os.Unsetenv("POSTGRES_DB")
		os.Unsetenv("LLM_MODEL_DEFAULT")
		os.Unsetenv("DEBUG")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Debug {
		t.Errorf("expected Debug=true, got %v", cfg.Debug)
	}
}

func TestDebugParsing_One(t *testing.T) {
	os.Setenv("DISCORD_TOKEN", "test-token")
	os.Setenv("OPENAI_API_KEY", "test-key")
	os.Setenv("DATABASE_URL", "postgres://localhost/test")
	os.Setenv("POSTGRES_HOST", "localhost")
	os.Setenv("POSTGRES_PORT", "5432")
	os.Setenv("POSTGRES_USER", "user")
	os.Setenv("POSTGRES_PASSWORD", "pass")
	os.Setenv("POSTGRES_DB", "iris")
	os.Setenv("LLM_MODEL_DEFAULT", "kr/claude-sonnet-4.5")
	os.Setenv("DEBUG", "1")
	defer func() {
		os.Unsetenv("DISCORD_TOKEN")
		os.Unsetenv("OPENAI_API_KEY")
		os.Unsetenv("DATABASE_URL")
		os.Unsetenv("POSTGRES_HOST")
		os.Unsetenv("POSTGRES_PORT")
		os.Unsetenv("POSTGRES_USER")
		os.Unsetenv("POSTGRES_PASSWORD")
		os.Unsetenv("POSTGRES_DB")
		os.Unsetenv("LLM_MODEL_DEFAULT")
		os.Unsetenv("DEBUG")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Debug {
		t.Errorf("expected Debug=true, got %v", cfg.Debug)
	}
}

func TestDebugParsing_False(t *testing.T) {
	os.Setenv("DISCORD_TOKEN", "test-token")
	os.Setenv("OPENAI_API_KEY", "test-key")
	os.Setenv("DATABASE_URL", "postgres://localhost/test")
	os.Setenv("POSTGRES_HOST", "localhost")
	os.Setenv("POSTGRES_PORT", "5432")
	os.Setenv("POSTGRES_USER", "user")
	os.Setenv("POSTGRES_PASSWORD", "pass")
	os.Setenv("POSTGRES_DB", "iris")
	os.Setenv("LLM_MODEL_DEFAULT", "kr/claude-sonnet-4.5")
	os.Setenv("DEBUG", "false")
	defer func() {
		os.Unsetenv("DISCORD_TOKEN")
		os.Unsetenv("OPENAI_API_KEY")
		os.Unsetenv("DATABASE_URL")
		os.Unsetenv("POSTGRES_HOST")
		os.Unsetenv("POSTGRES_PORT")
		os.Unsetenv("POSTGRES_USER")
		os.Unsetenv("POSTGRES_PASSWORD")
		os.Unsetenv("POSTGRES_DB")
		os.Unsetenv("LLM_MODEL_DEFAULT")
		os.Unsetenv("DEBUG")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Debug {
		t.Errorf("expected Debug=false, got %v", cfg.Debug)
	}
}

func TestDebugParsing_Unset(t *testing.T) {
	os.Setenv("DISCORD_TOKEN", "test-token")
	os.Setenv("OPENAI_API_KEY", "test-key")
	os.Setenv("DATABASE_URL", "postgres://localhost/test")
	os.Setenv("POSTGRES_HOST", "localhost")
	os.Setenv("POSTGRES_PORT", "5432")
	os.Setenv("POSTGRES_USER", "user")
	os.Setenv("POSTGRES_PASSWORD", "pass")
	os.Setenv("POSTGRES_DB", "iris")
	os.Setenv("LLM_MODEL_DEFAULT", "kr/claude-sonnet-4.5")
	os.Unsetenv("DEBUG")
	defer func() {
		os.Unsetenv("DISCORD_TOKEN")
		os.Unsetenv("OPENAI_API_KEY")
		os.Unsetenv("DATABASE_URL")
		os.Unsetenv("POSTGRES_HOST")
		os.Unsetenv("POSTGRES_PORT")
		os.Unsetenv("POSTGRES_USER")
		os.Unsetenv("POSTGRES_PASSWORD")
		os.Unsetenv("POSTGRES_DB")
		os.Unsetenv("LLM_MODEL_DEFAULT")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Debug {
		t.Errorf("expected Debug=false when unset, got %v", cfg.Debug)
	}
}

func TestConvLockTTL_DefaultWhenUnset(t *testing.T) {
	os.Setenv("DISCORD_TOKEN", "test-token")
	os.Setenv("OPENAI_API_KEY", "test-key")
	os.Setenv("DATABASE_URL", "postgres://localhost/test")
	os.Setenv("POSTGRES_HOST", "localhost")
	os.Setenv("POSTGRES_PORT", "5432")
	os.Setenv("POSTGRES_USER", "user")
	os.Setenv("POSTGRES_PASSWORD", "pass")
	os.Setenv("POSTGRES_DB", "iris")
	os.Setenv("LLM_MODEL_DEFAULT", "kr/claude-sonnet-4.5")
	os.Unsetenv("IRIS_CONV_LOCK_TTL")
	defer func() {
		os.Unsetenv("DISCORD_TOKEN")
		os.Unsetenv("OPENAI_API_KEY")
		os.Unsetenv("DATABASE_URL")
		os.Unsetenv("POSTGRES_HOST")
		os.Unsetenv("POSTGRES_PORT")
		os.Unsetenv("POSTGRES_USER")
		os.Unsetenv("POSTGRES_PASSWORD")
		os.Unsetenv("POSTGRES_DB")
		os.Unsetenv("LLM_MODEL_DEFAULT")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ConversationLockTTL != 5*time.Minute {
		t.Errorf("expected ConversationLockTTL=5m, got %v", cfg.ConversationLockTTL)
	}
}

func TestConvLockTTL_ParsesDuration(t *testing.T) {
	os.Setenv("DISCORD_TOKEN", "test-token")
	os.Setenv("OPENAI_API_KEY", "test-key")
	os.Setenv("DATABASE_URL", "postgres://localhost/test")
	os.Setenv("POSTGRES_HOST", "localhost")
	os.Setenv("POSTGRES_PORT", "5432")
	os.Setenv("POSTGRES_USER", "user")
	os.Setenv("POSTGRES_PASSWORD", "pass")
	os.Setenv("POSTGRES_DB", "iris")
	os.Setenv("LLM_MODEL_DEFAULT", "kr/claude-sonnet-4.5")
	os.Setenv("IRIS_CONV_LOCK_TTL", "2m30s")
	defer func() {
		os.Unsetenv("DISCORD_TOKEN")
		os.Unsetenv("OPENAI_API_KEY")
		os.Unsetenv("DATABASE_URL")
		os.Unsetenv("POSTGRES_HOST")
		os.Unsetenv("POSTGRES_PORT")
		os.Unsetenv("POSTGRES_USER")
		os.Unsetenv("POSTGRES_PASSWORD")
		os.Unsetenv("POSTGRES_DB")
		os.Unsetenv("LLM_MODEL_DEFAULT")
		os.Unsetenv("IRIS_CONV_LOCK_TTL")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := 2*time.Minute + 30*time.Second
	if cfg.ConversationLockTTL != expected {
		t.Errorf("expected ConversationLockTTL=%v, got %v", expected, cfg.ConversationLockTTL)
	}
}

func TestConvLockTTL_InvalidFallsBackToDefault(t *testing.T) {
	os.Setenv("DISCORD_TOKEN", "test-token")
	os.Setenv("OPENAI_API_KEY", "test-key")
	os.Setenv("DATABASE_URL", "postgres://localhost/test")
	os.Setenv("POSTGRES_HOST", "localhost")
	os.Setenv("POSTGRES_PORT", "5432")
	os.Setenv("POSTGRES_USER", "user")
	os.Setenv("POSTGRES_PASSWORD", "pass")
	os.Setenv("POSTGRES_DB", "iris")
	os.Setenv("LLM_MODEL_DEFAULT", "kr/claude-sonnet-4.5")
	os.Setenv("IRIS_CONV_LOCK_TTL", "invalid-duration")
	defer func() {
		os.Unsetenv("DISCORD_TOKEN")
		os.Unsetenv("OPENAI_API_KEY")
		os.Unsetenv("DATABASE_URL")
		os.Unsetenv("POSTGRES_HOST")
		os.Unsetenv("POSTGRES_PORT")
		os.Unsetenv("POSTGRES_USER")
		os.Unsetenv("POSTGRES_PASSWORD")
		os.Unsetenv("POSTGRES_DB")
		os.Unsetenv("LLM_MODEL_DEFAULT")
		os.Unsetenv("IRIS_CONV_LOCK_TTL")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ConversationLockTTL != 5*time.Minute {
		t.Errorf("expected ConversationLockTTL=5m (default), got %v", cfg.ConversationLockTTL)
	}
}

func setRequiredEnvForLoad(t *testing.T) {
	t.Helper()

	required := map[string]string{
		"DISCORD_TOKEN":     "test-token",
		"OPENAI_API_KEY":    "test-key",
		"DATABASE_URL":      "postgres://localhost/test",
		"POSTGRES_HOST":     "localhost",
		"POSTGRES_PORT":     "5432",
		"POSTGRES_USER":     "user",
		"POSTGRES_PASSWORD": "pass",
		"POSTGRES_DB":       "iris",
		"LLM_MODEL_DEFAULT": "kr/claude-sonnet-4.5",
	}

	for key, value := range required {
		k := key
		if err := os.Setenv(k, value); err != nil {
			t.Fatalf("setenv %s: %v", k, err)
		}
		t.Cleanup(func() {
			_ = os.Unsetenv(k)
		})
	}
}

func TestMemoryServerConfig_DefaultsWhenUnset(t *testing.T) {
	setRequiredEnvForLoad(t)

	os.Unsetenv("MEMORY_SERVER_ENABLED")
	os.Unsetenv("MEMORY_SERVER_RECALL_THRESHOLD")
	os.Unsetenv("MEMORY_SERVER_RECALL_TOP_K")
	os.Unsetenv("MEMORY_SERVER_EMBED_BATCH_SIZE")
	os.Unsetenv("MEMORY_SERVER_EMBED_WORKERS")
	os.Unsetenv("MEMORY_SERVER_EMBED_BACKFILL_LIMIT")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !cfg.MemoryServer.Enabled {
		t.Errorf("expected MemoryServer.Enabled=true, got %v", cfg.MemoryServer.Enabled)
	}
	if cfg.MemoryServer.RecallThreshold != 0.55 {
		t.Errorf("expected MemoryServer.RecallThreshold=0.55, got %f", cfg.MemoryServer.RecallThreshold)
	}
	if cfg.MemoryServer.RecallTopK != 5 {
		t.Errorf("expected MemoryServer.RecallTopK=5, got %d", cfg.MemoryServer.RecallTopK)
	}
	if cfg.MemoryServer.EmbedBatchSize != 32 {
		t.Errorf("expected MemoryServer.EmbedBatchSize=32, got %d", cfg.MemoryServer.EmbedBatchSize)
	}
	if cfg.MemoryServer.EmbedWorkers != 1 {
		t.Errorf("expected MemoryServer.EmbedWorkers=1, got %d", cfg.MemoryServer.EmbedWorkers)
	}
	if cfg.MemoryServer.EmbedBackfillLimit != 500 {
		t.Errorf("expected MemoryServer.EmbedBackfillLimit=500, got %d", cfg.MemoryServer.EmbedBackfillLimit)
	}
}

func TestMemoryServerConfig_EnvOverrides(t *testing.T) {
	setRequiredEnvForLoad(t)

	os.Setenv("MEMORY_SERVER_ENABLED", "false")
	os.Setenv("MEMORY_SERVER_RECALL_THRESHOLD", "0.85")
	os.Setenv("MEMORY_SERVER_RECALL_TOP_K", "11")
	os.Setenv("MEMORY_SERVER_EMBED_BATCH_SIZE", "64")
	os.Setenv("MEMORY_SERVER_EMBED_WORKERS", "3")
	os.Setenv("MEMORY_SERVER_EMBED_BACKFILL_LIMIT", "1000")
	defer func() {
		os.Unsetenv("MEMORY_SERVER_ENABLED")
		os.Unsetenv("MEMORY_SERVER_RECALL_THRESHOLD")
		os.Unsetenv("MEMORY_SERVER_RECALL_TOP_K")
		os.Unsetenv("MEMORY_SERVER_EMBED_BATCH_SIZE")
		os.Unsetenv("MEMORY_SERVER_EMBED_WORKERS")
		os.Unsetenv("MEMORY_SERVER_EMBED_BACKFILL_LIMIT")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.MemoryServer.Enabled {
		t.Errorf("expected MemoryServer.Enabled=false, got %v", cfg.MemoryServer.Enabled)
	}
	if cfg.MemoryServer.RecallThreshold != 0.85 {
		t.Errorf("expected MemoryServer.RecallThreshold=0.85, got %f", cfg.MemoryServer.RecallThreshold)
	}
	if cfg.MemoryServer.RecallTopK != 11 {
		t.Errorf("expected MemoryServer.RecallTopK=11, got %d", cfg.MemoryServer.RecallTopK)
	}
	if cfg.MemoryServer.EmbedBatchSize != 64 {
		t.Errorf("expected MemoryServer.EmbedBatchSize=64, got %d", cfg.MemoryServer.EmbedBatchSize)
	}
	if cfg.MemoryServer.EmbedWorkers != 3 {
		t.Errorf("expected MemoryServer.EmbedWorkers=3, got %d", cfg.MemoryServer.EmbedWorkers)
	}
	if cfg.MemoryServer.EmbedBackfillLimit != 1000 {
		t.Errorf("expected MemoryServer.EmbedBackfillLimit=1000, got %d", cfg.MemoryServer.EmbedBackfillLimit)
	}
}

func TestMemoryServerConfig_ThresholdOutOfRangeFallsBack(t *testing.T) {
	setRequiredEnvForLoad(t)

	os.Setenv("MEMORY_SERVER_RECALL_THRESHOLD", "1.5")
	defer os.Unsetenv("MEMORY_SERVER_RECALL_THRESHOLD")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.MemoryServer.RecallThreshold != 0.55 {
		t.Errorf("expected MemoryServer.RecallThreshold=0.55 (fallback), got %f", cfg.MemoryServer.RecallThreshold)
	}
}

func TestLoadConfig_LoreLLMModelUnset(t *testing.T) {
	setRequiredEnvForLoad(t)
	os.Unsetenv("IRIS_LORE_LLM_MODEL")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.LoreLLMModel != cfg.LLMModelStrong {
		t.Errorf("LoreLLMModel unset should default to LLMModelStrong (%s), got %s", cfg.LLMModelStrong, cfg.LoreLLMModel)
	}
}

func TestLoadConfig_LoreLLMModelSet(t *testing.T) {
	setRequiredEnvForLoad(t)
	os.Setenv("IRIS_LORE_LLM_MODEL", "kr/claude-opus-4.7")
	defer os.Unsetenv("IRIS_LORE_LLM_MODEL")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.LoreLLMModel != "kr/claude-opus-4.7" {
		t.Errorf("LoreLLMModel set should use env value, got %s", cfg.LoreLLMModel)
	}
}

func TestLoadConfig_TimeoutDefaults(t *testing.T) {
	setRequiredEnvForLoad(t)
	os.Unsetenv("LLM_TIMEOUT")
	os.Unsetenv("LLM_CHAT_TIMEOUT")
	os.Unsetenv("LLM_TOOL_TIMEOUT")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.LLMTimeout != 2*time.Minute {
		t.Errorf("LLMTimeout default = %v, want 2m", cfg.LLMTimeout)
	}
	if cfg.LLMChatTimeout != 2*time.Minute {
		t.Errorf("LLMChatTimeout default = %v, want 2m", cfg.LLMChatTimeout)
	}
	if cfg.LLMToolTimeout != 10*time.Minute {
		t.Errorf("LLMToolTimeout default = %v, want 10m", cfg.LLMToolTimeout)
	}
}

func TestLoadConfig_TimeoutOnlyLLMTimeoutSet(t *testing.T) {
	setRequiredEnvForLoad(t)
	os.Setenv("LLM_TIMEOUT", "1m")
	os.Unsetenv("LLM_CHAT_TIMEOUT")
	os.Unsetenv("LLM_TOOL_TIMEOUT")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.LLMTimeout != 1*time.Minute {
		t.Errorf("LLMTimeout = %v, want 1m", cfg.LLMTimeout)
	}
	if cfg.LLMChatTimeout != 1*time.Minute {
		t.Errorf("LLMChatTimeout should inherit LLMTimeout = %v, want 1m", cfg.LLMChatTimeout)
	}
	if cfg.LLMToolTimeout != 4*time.Minute {
		t.Errorf("LLMToolTimeout should be LLMTimeout*4 = %v, want 4m", cfg.LLMToolTimeout)
	}
}

func TestLoadConfig_TimeoutChatAndToolSet(t *testing.T) {
	setRequiredEnvForLoad(t)
	os.Setenv("LLM_CHAT_TIMEOUT", "2m")
	os.Setenv("LLM_TOOL_TIMEOUT", "15m")
	os.Unsetenv("LLM_TIMEOUT")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.LLMChatTimeout != 2*time.Minute {
		t.Errorf("LLMChatTimeout = %v, want 2m", cfg.LLMChatTimeout)
	}
	if cfg.LLMToolTimeout != 15*time.Minute {
		t.Errorf("LLMToolTimeout = %v, want 15m", cfg.LLMToolTimeout)
	}
}

func TestLoadConfig_TimeoutToolRoundingUp(t *testing.T) {
	setRequiredEnvForLoad(t)
	os.Setenv("LLM_TIMEOUT", "35s")
	os.Unsetenv("LLM_CHAT_TIMEOUT")
	os.Unsetenv("LLM_TOOL_TIMEOUT")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.LLMToolTimeout != 3*time.Minute {
		t.Errorf("LLMToolTimeout (35s*4=140s rounded up) = %v, want 3m", cfg.LLMToolTimeout)
	}
}
