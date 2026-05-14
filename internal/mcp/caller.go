package mcp

import "context"

type ctxKey int

const ctxKeyCallerUserID ctxKey = 1

// WithCallerUserID attaches the invoking Discord user ID to ctx so
// owner-gated MCP tools can authenticate the caller.
func WithCallerUserID(ctx context.Context, userID int64) context.Context {
	return context.WithValue(ctx, ctxKeyCallerUserID, userID)
}

// CallerUserID returns the Discord user ID previously stored via
// WithCallerUserID, or 0 if no ID was set.
func CallerUserID(ctx context.Context) int64 {
	if v, ok := ctx.Value(ctxKeyCallerUserID).(int64); ok {
		return v
	}
	return 0
}
