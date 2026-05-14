package logger

import (
	"log/slog"
	"os"
)

func New() *slog.Logger {
	return NewWithDebug(false)
}

func NewWithDebug(debug bool) *slog.Logger {
	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})
	return slog.New(handler)
}
