package obs

import (
	"context"
	"io"
	"log/slog"

	"github.com/eko/iris-bot/internal/memory"
)

type Logger struct {
	base     *slog.Logger
	redactor *memory.Redactor
}

// NewLogger wraps a slog.Logger and adds secret redaction to every string attribute.
func NewLogger(w io.Writer, level slog.Level) *Logger {
	handler := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level})
	return &Logger{
		base:     slog.New(handler),
		redactor: memory.NewRedactor(),
	}
}

// With adds key=value pairs. String values are redacted if they match sensitive patterns.
func (l *Logger) With(args ...any) *Logger {
	redactedArgs := l.redactArgs(args)
	return &Logger{
		base:     l.base.With(redactedArgs...),
		redactor: l.redactor,
	}
}

// redactArgs walks key-value pairs and redacts string values.
func (l *Logger) redactArgs(args []any) []any {
	result := make([]any, len(args))
	for i := 0; i < len(args); i++ {
		result[i] = args[i]
		if i+1 < len(args) && i%2 == 0 {
			if str, ok := args[i+1].(string); ok {
				result[i+1] = l.redactor.Redact(str)
			} else {
				result[i+1] = args[i+1]
			}
			i++
		}
	}
	return result
}

// Info logs at info level, auto-attaching correlation_id if present in ctx.
func (l *Logger) Info(ctx context.Context, msg string, args ...any) {
	args = l.attachCorrelationID(ctx, args)
	redactedArgs := l.redactArgs(args)
	l.base.InfoContext(ctx, msg, redactedArgs...)
}

// Warn logs at warn level, auto-attaching correlation_id if present in ctx.
func (l *Logger) Warn(ctx context.Context, msg string, args ...any) {
	args = l.attachCorrelationID(ctx, args)
	redactedArgs := l.redactArgs(args)
	l.base.WarnContext(ctx, msg, redactedArgs...)
}

// Error logs at error level, auto-attaching correlation_id if present in ctx.
func (l *Logger) Error(ctx context.Context, msg string, args ...any) {
	args = l.attachCorrelationID(ctx, args)
	redactedArgs := l.redactArgs(args)
	l.base.ErrorContext(ctx, msg, redactedArgs...)
}

// Debug logs at debug level, auto-attaching correlation_id if present in ctx.
func (l *Logger) Debug(ctx context.Context, msg string, args ...any) {
	args = l.attachCorrelationID(ctx, args)
	redactedArgs := l.redactArgs(args)
	l.base.DebugContext(ctx, msg, redactedArgs...)
}

// attachCorrelationID prepends correlation_id to args if present in ctx.
func (l *Logger) attachCorrelationID(ctx context.Context, args []any) []any {
	corrID := CorrelationID(ctx)
	if corrID == "" {
		return args
	}
	return append([]any{"correlation_id", corrID}, args...)
}
