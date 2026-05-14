package config

import (
	"os"
	"testing"
	"time"
)

func setupConfigTestEnv() {
	os.Setenv("DISCORD_TOKEN", "test-token")
	os.Setenv("OPENAI_API_KEY", "test-key")
	os.Setenv("DATABASE_URL", "postgres://test")
	os.Setenv("POSTGRES_HOST", "localhost")
	os.Setenv("POSTGRES_PORT", "5432")
	os.Setenv("POSTGRES_USER", "test")
	os.Setenv("POSTGRES_PASSWORD", "test")
	os.Setenv("POSTGRES_DB", "test")
	os.Setenv("LLM_MODEL", "kr/claude-sonnet-4.5")
	os.Setenv("LLM_MODEL_ROUTER", "kr/claude-haiku-4.5")
	os.Setenv("LLM_MODEL_DEFAULT", "kr/claude-sonnet-4.5")
	os.Setenv("LLM_MODEL_STRONG", "kr/claude-opus-4.7")
}

func TestLoreConfigDefaults(t *testing.T) {
	setupConfigTestEnv()

	t.Run("LoreThreadsEnabledDefaultsFalse", func(t *testing.T) {
		os.Unsetenv("IRIS_LORE_THREADS_ENABLED")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load failed: %v", err)
		}
		if cfg.LoreThreadsEnabled {
			t.Error("expected LoreThreadsEnabled to default to false")
		}
	})

	t.Run("LoreIdleDurationDefaults5m", func(t *testing.T) {
		os.Unsetenv("IRIS_LORE_IDLE_DURATION")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load failed: %v", err)
		}
		if cfg.LoreIdleDuration != 5*time.Minute {
			t.Errorf("expected LoreIdleDuration 5m, got %v", cfg.LoreIdleDuration)
		}
	})

	t.Run("LoreCompactionTargetDefaults0_70", func(t *testing.T) {
		os.Unsetenv("IRIS_LORE_COMPACTION_TARGET")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load failed: %v", err)
		}
		if cfg.LoreCompactionTarget != 0.70 {
			t.Errorf("expected LoreCompactionTarget 0.70, got %v", cfg.LoreCompactionTarget)
		}
	})

	t.Run("LoreThreadCapPerHourDefaults6", func(t *testing.T) {
		os.Unsetenv("IRIS_LORE_THREAD_CAP_PER_HOUR")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load failed: %v", err)
		}
		if cfg.LoreThreadCapPerHour != 6 {
			t.Errorf("expected LoreThreadCapPerHour 6, got %d", cfg.LoreThreadCapPerHour)
		}
	})

	t.Run("LoreWorkerPollIntervalDefaults30s", func(t *testing.T) {
		os.Unsetenv("IRIS_LORE_WORKER_POLL_INTERVAL")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load failed: %v", err)
		}
		if cfg.LoreWorkerPollInterval != 30*time.Second {
			t.Errorf("expected LoreWorkerPollInterval 30s, got %v", cfg.LoreWorkerPollInterval)
		}
	})

	t.Run("LoreLLMTimeoutDefaults30s", func(t *testing.T) {
		os.Unsetenv("IRIS_LORE_LLM_TIMEOUT")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load failed: %v", err)
		}
		if cfg.LoreLLMTimeout != 30*time.Second {
			t.Errorf("expected LoreLLMTimeout 30s, got %v", cfg.LoreLLMTimeout)
		}
	})

	t.Run("LoreCaptureTimeoutDefaults60s", func(t *testing.T) {
		os.Unsetenv("IRIS_LORE_CAPTURE_TIMEOUT")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load failed: %v", err)
		}
		if cfg.LoreCaptureTimeout != 60*time.Second {
			t.Errorf("expected LoreCaptureTimeout 60s, got %v", cfg.LoreCaptureTimeout)
		}
	})
}

func TestLoreConfigEnvVars(t *testing.T) {
	setupConfigTestEnv()

	t.Run("LoreThreadsEnabledFromEnv", func(t *testing.T) {
		os.Setenv("IRIS_LORE_THREADS_ENABLED", "true")
		defer os.Unsetenv("IRIS_LORE_THREADS_ENABLED")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load failed: %v", err)
		}
		if !cfg.LoreThreadsEnabled {
			t.Error("expected LoreThreadsEnabled to be true from env")
		}
	})

	t.Run("LoreIdleDurationFromEnv", func(t *testing.T) {
		os.Setenv("IRIS_LORE_IDLE_DURATION", "10m")
		defer os.Unsetenv("IRIS_LORE_IDLE_DURATION")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load failed: %v", err)
		}
		if cfg.LoreIdleDuration != 10*time.Minute {
			t.Errorf("expected LoreIdleDuration 10m, got %v", cfg.LoreIdleDuration)
		}
	})

	t.Run("LoreCompactionTargetFromEnv", func(t *testing.T) {
		os.Setenv("IRIS_LORE_COMPACTION_TARGET", "0.85")
		defer os.Unsetenv("IRIS_LORE_COMPACTION_TARGET")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load failed: %v", err)
		}
		if cfg.LoreCompactionTarget != 0.85 {
			t.Errorf("expected LoreCompactionTarget 0.85, got %v", cfg.LoreCompactionTarget)
		}
	})

	t.Run("LoreThreadCapPerHourFromEnv", func(t *testing.T) {
		os.Setenv("IRIS_LORE_THREAD_CAP_PER_HOUR", "12")
		defer os.Unsetenv("IRIS_LORE_THREAD_CAP_PER_HOUR")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load failed: %v", err)
		}
		if cfg.LoreThreadCapPerHour != 12 {
			t.Errorf("expected LoreThreadCapPerHour 12, got %d", cfg.LoreThreadCapPerHour)
		}
	})

	t.Run("LoreWorkerPollIntervalFromEnv", func(t *testing.T) {
		os.Setenv("IRIS_LORE_WORKER_POLL_INTERVAL", "1m")
		defer os.Unsetenv("IRIS_LORE_WORKER_POLL_INTERVAL")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load failed: %v", err)
		}
		if cfg.LoreWorkerPollInterval != 1*time.Minute {
			t.Errorf("expected LoreWorkerPollInterval 1m, got %v", cfg.LoreWorkerPollInterval)
		}
	})

	t.Run("LoreLLMTimeoutFromEnv", func(t *testing.T) {
		os.Setenv("IRIS_LORE_LLM_TIMEOUT", "60s")
		defer os.Unsetenv("IRIS_LORE_LLM_TIMEOUT")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load failed: %v", err)
		}
		if cfg.LoreLLMTimeout != 60*time.Second {
			t.Errorf("expected LoreLLMTimeout 60s, got %v", cfg.LoreLLMTimeout)
		}
	})

	t.Run("LoreCaptureTimeoutFromEnv", func(t *testing.T) {
		os.Setenv("IRIS_LORE_CAPTURE_TIMEOUT", "120s")
		defer os.Unsetenv("IRIS_LORE_CAPTURE_TIMEOUT")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load failed: %v", err)
		}
		if cfg.LoreCaptureTimeout != 120*time.Second {
			t.Errorf("expected LoreCaptureTimeout 120s, got %v", cfg.LoreCaptureTimeout)
		}
	})

	t.Run("LoreCaptureTimeoutZeroMeansNoDeadline", func(t *testing.T) {
		os.Setenv("IRIS_LORE_CAPTURE_TIMEOUT", "0")
		defer os.Unsetenv("IRIS_LORE_CAPTURE_TIMEOUT")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load failed: %v", err)
		}
		if cfg.LoreCaptureTimeout != 0 {
			t.Errorf("expected LoreCaptureTimeout 0, got %v", cfg.LoreCaptureTimeout)
		}
	})
}
