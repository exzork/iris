package obs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

type corrKey struct{}

// NewCorrelationID returns a hex-encoded random 16-byte ID.
func NewCorrelationID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// WithCorrelationID returns ctx with the given correlation ID.
func WithCorrelationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, corrKey{}, id)
}

// CorrelationID retrieves the correlation ID from ctx, or empty string if not set.
func CorrelationID(ctx context.Context) string {
	v, _ := ctx.Value(corrKey{}).(string)
	return v
}

// EnsureCorrelationID returns ctx with an existing or freshly-generated correlation ID,
// and the ID value.
func EnsureCorrelationID(ctx context.Context) (context.Context, string) {
	id := CorrelationID(ctx)
	if id != "" {
		return ctx, id
	}
	id = NewCorrelationID()
	return WithCorrelationID(ctx, id), id
}
