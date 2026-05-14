package orchestrator

import (
	"context"

	"github.com/eko/iris-bot/internal/mcp"
)

// WithCallerUserID returns ctx with the invoking Discord user ID attached so
// downstream owner-gated tools (e.g. mcp_add) can authenticate. The value is
// stored under the same key used by internal/mcp so both packages agree on
// the ctx-value shape without creating an import cycle.
func WithCallerUserID(ctx context.Context, userID int64) context.Context {
	return mcp.WithCallerUserID(ctx, userID)
}
