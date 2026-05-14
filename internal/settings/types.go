package settings

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

func (r *Resolver) GetInt(ctx context.Context, guildID int64, key Key, fallback int) (int, error) {
	val, err := r.Effective(ctx, guildID, key)
	if err != nil {
		return fallback, nil
	}
	if val == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(val)
	if err != nil {
		return fallback, nil
	}
	return parsed, nil
}

func (r *Resolver) GetBool(ctx context.Context, guildID int64, key Key, fallback bool) (bool, error) {
	val, err := r.Effective(ctx, guildID, key)
	if err != nil {
		return fallback, nil
	}
	if val == "" {
		return fallback, nil
	}
	return parseBool(val), nil
}

func (r *Resolver) GetInt64(ctx context.Context, guildID int64, key Key, fallback int64) (int64, error) {
	val, err := r.Effective(ctx, guildID, key)
	if err != nil {
		return fallback, nil
	}
	if val == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return fallback, nil
	}
	return parsed, nil
}

func (r *Resolver) GetInt64Slice(ctx context.Context, guildID int64, key Key) ([]int64, error) {
	val, err := r.Effective(ctx, guildID, key)
	if err != nil {
		return nil, err
	}
	if val == "" {
		return []int64{}, nil
	}
	return parseInt64Slice(val)
}

func parseBool(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "true", "yes", "1":
		return true
	case "false", "no", "0", "":
		return false
	default:
		return false
	}
}

func parseInt64Slice(s string) ([]int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return []int64{}, nil
	}

	parts := strings.Split(s, ",")
	result := make([]int64, 0, len(parts))

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		val, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse int64 from %q: %w", part, err)
		}
		result = append(result, val)
	}

	return result, nil
}
