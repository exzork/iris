package logger

import (
	"log/slog"
	"testing"
)

func TestNewLogger_Success(t *testing.T) {
	logger := New()
	if logger == nil {
		t.Fatal("expected logger to be non-nil")
	}
}

func TestLoggerStructured(t *testing.T) {
	logger := New()
	if logger == nil {
		t.Fatal("expected logger to be non-nil")
	}

	handler := logger.Handler()
	if handler == nil {
		t.Fatal("expected handler to be non-nil")
	}
}

func TestLoggerIsStructured(t *testing.T) {
	logger := New()
	if logger == nil {
		t.Fatal("expected logger to be non-nil")
	}

	_, ok := logger.Handler().(*slog.JSONHandler)
	if !ok {
		t.Error("expected JSONHandler for structured logging")
	}
}
