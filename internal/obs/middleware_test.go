package obs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"
)

func TestWithObservabilityPropagatesCorrelationID(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewLogger(buf, slog.LevelInfo)
	ctx := context.Background()

	stage1 := logger.WithObservability("stage1", func(ctx context.Context) error {
		return nil
	})

	stage2 := logger.WithObservability("stage2", func(ctx context.Context) error {
		return nil
	})

	stage3 := logger.WithObservability("stage3", func(ctx context.Context) error {
		return nil
	})

	if err := stage1(ctx); err != nil {
		t.Fatalf("stage1 failed: %v", err)
	}
	if err := stage2(ctx); err != nil {
		t.Fatalf("stage2 failed: %v", err)
	}
	if err := stage3(ctx); err != nil {
		t.Fatalf("stage3 failed: %v", err)
	}

	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	if len(lines) < 6 {
		t.Fatalf("expected at least 6 log lines, got %d", len(lines))
	}

	var corrIDs []string
	for _, line := range lines {
		var entry map[string]any
		if err := json.Unmarshal(line, &entry); err != nil {
			t.Fatalf("failed to parse log line: %v", err)
		}
		if corrID, ok := entry["correlation_id"].(string); ok {
			corrIDs = append(corrIDs, corrID)
		}
	}

	if len(corrIDs) < 6 {
		t.Fatalf("expected at least 6 correlation IDs in logs, got %d", len(corrIDs))
	}

	first := corrIDs[0]
	for i, id := range corrIDs {
		if id != first {
			t.Errorf("log line %d has different correlation_id: %s vs %s", i, id, first)
		}
	}
}

func TestWithObservabilityLogsDuration(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewLogger(buf, slog.LevelInfo)
	ctx := context.Background()

	wrapped := logger.WithObservability("test_stage", func(ctx context.Context) error {
		time.Sleep(10 * time.Millisecond)
		return nil
	})

	if err := wrapped(ctx); err != nil {
		t.Fatalf("wrapped function failed: %v", err)
	}

	output := buf.String()
	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))

	var foundDuration bool
	for _, line := range lines {
		var entry map[string]any
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		if durationMs, ok := entry["duration_ms"].(float64); ok && durationMs >= 10 {
			foundDuration = true
			break
		}
	}

	if !foundDuration {
		t.Errorf("expected duration_ms >= 10 in logs, got: %s", output)
	}
}

func TestWithObservabilityLogsError(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewLogger(buf, slog.LevelInfo)
	ctx := context.Background()

	testErr := errors.New("test error")
	wrapped := logger.WithObservability("failing_stage", func(ctx context.Context) error {
		return testErr
	})

	err := wrapped(ctx)
	if err != testErr {
		t.Errorf("expected error to be propagated, got %v", err)
	}

	output := buf.String()
	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))

	var foundError bool
	for _, line := range lines {
		var entry map[string]any
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		if msg, ok := entry["msg"].(string); ok && msg == "stage_error" {
			if errMsg, ok := entry["error"].(string); ok && errMsg == "test error" {
				if errClass, ok := entry["error_class"].(string); ok && errClass == string(ErrClassInternal) {
					foundError = true
					break
				}
			}
		}
	}

	if !foundError {
		t.Errorf("expected stage_error log with error_class, got: %s", output)
	}
}
