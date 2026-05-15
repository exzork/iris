package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestBuildBinary(t *testing.T) {
	os.Remove("/tmp/iris-bot-test")
	cmd := exec.Command("go", "build", "-o", "/tmp/iris-bot-test", "./cmd/iris-bot")
	cmd.Dir = getProjectRoot(t)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to build binary: %v", err)
	}

	if _, err := os.Stat("/tmp/iris-bot-test"); err != nil {
		t.Fatalf("binary not found after build: %v", err)
	}
}

func TestCheckConfigFlag(t *testing.T) {
	ensureBinaryBuilt(t)

	cmd := exec.Command("/tmp/iris-bot-test", "--check-config")
	cmd.Env = []string{
		"DISCORD_TOKEN=test-token",
		"OPENAI_API_KEY=test-key",
		"DATABASE_URL=postgres://user:pass@localhost/db",
		"POSTGRES_HOST=localhost",
		"POSTGRES_PORT=5432",
		"POSTGRES_USER=user",
		"POSTGRES_PASSWORD=pass",
		"POSTGRES_DB=db",
		"LLM_MODEL=kr/claude-sonnet-4.5",
	}
	if err := cmd.Run(); err != nil {
		t.Fatalf("--check-config failed: %v", err)
	}
}

func TestMissingConfigFails(t *testing.T) {
	ensureBinaryBuilt(t)

	cmd := exec.Command("/tmp/iris-bot-test", "--check-config")
	cmd.Env = []string{}
	if err := cmd.Run(); err == nil {
		t.Fatal("expected error with missing config, got none")
	}
}

func ensureBinaryBuilt(t *testing.T) {
	if _, err := os.Stat("/tmp/iris-bot-test"); err != nil {
		cmd := exec.Command("go", "build", "-o", "/tmp/iris-bot-test", "./cmd/iris-bot")
		cmd.Dir = getProjectRoot(t)
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
		if err := cmd.Run(); err != nil {
			t.Fatalf("failed to build binary: %v", err)
		}
	}
}

func getProjectRoot(t *testing.T) string {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			t.Fatal("could not find project root (go.mod)")
		}
		wd = parent
	}
}
