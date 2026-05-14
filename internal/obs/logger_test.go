package obs

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
)

func TestLogWritesCorrelationID(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewLogger(buf, slog.LevelInfo)
	ctx := WithCorrelationID(context.Background(), "abc123")
	logger.Info(ctx, "test message")

	var logEntry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("failed to parse log JSON: %v", err)
	}

	corrID, ok := logEntry["correlation_id"]
	if !ok {
		t.Error("correlation_id not found in log entry")
	}
	if corrID != "abc123" {
		t.Errorf("expected correlation_id=abc123, got %v", corrID)
	}
}

func TestLogRedactsSecrets(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewLogger(buf, slog.LevelInfo)
	ctx := context.Background()
	logger.Info(ctx, "test", "token", "sk-test-1234567890abcdef1234567890abcdef")

	output := buf.String()
	if bytes.Contains([]byte(output), []byte("sk-")) {
		t.Errorf("secret key not redacted in output: %s", output)
	}
	if !bytes.Contains([]byte(output), []byte("[REDACTED_TOKEN]")) {
		t.Errorf("expected [REDACTED_TOKEN] in output: %s", output)
	}
}

func TestLogLevelsInfoWarnError(t *testing.T) {
	tests := []struct {
		name  string
		level slog.Level
		fn    func(*Logger, context.Context)
		want  string
	}{
		{
			name:  "info",
			level: slog.LevelInfo,
			fn: func(l *Logger, ctx context.Context) {
				l.Info(ctx, "info message")
			},
			want: "info message",
		},
		{
			name:  "warn",
			level: slog.LevelInfo,
			fn: func(l *Logger, ctx context.Context) {
				l.Warn(ctx, "warn message")
			},
			want: "warn message",
		},
		{
			name:  "error",
			level: slog.LevelInfo,
			fn: func(l *Logger, ctx context.Context) {
				l.Error(ctx, "error message")
			},
			want: "error message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			logger := NewLogger(buf, tt.level)
			ctx := context.Background()
			tt.fn(logger, ctx)

			if !bytes.Contains(buf.Bytes(), []byte(tt.want)) {
				t.Errorf("expected %q in output, got: %s", tt.want, buf.String())
			}
		})
	}
}

func TestLogDebugLevel(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewLogger(buf, slog.LevelDebug)
	ctx := context.Background()
	logger.Debug(ctx, "debug message")

	if !bytes.Contains(buf.Bytes(), []byte("debug message")) {
		t.Errorf("expected debug message in output, got: %s", buf.String())
	}
}
