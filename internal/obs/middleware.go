package obs

import (
	"context"
	"time"
)

// Stage represents a named pipeline step.
type Stage struct {
	Name string
	Run  func(ctx context.Context) error
}

// WithObservability wraps a function to attach correlation_id and log start/end/duration.
func (l *Logger) WithObservability(stage string, fn func(ctx context.Context) error) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		ctx, _ = EnsureCorrelationID(ctx)
		l.Info(ctx, "stage_start", "stage", stage)

		start := time.Now()
		err := fn(ctx)
		duration := time.Since(start)

		if err != nil {
			class := Classify(err)
			l.Error(ctx, "stage_error", "stage", stage, "error", err.Error(), "error_class", string(class), "duration_ms", duration.Milliseconds())
			return err
		}

		l.Info(ctx, "stage_end", "stage", stage, "duration_ms", duration.Milliseconds())
		return nil
	}
}
