package settings

import (
	"context"
	"fmt"
)

type Resolver struct {
	Repo SettingsRepo
}

// Effective returns the effective value: guild override -> default -> ("", false).
func (r *Resolver) Effective(ctx context.Context, guildID int64, key Key) (string, error) {
	if !IsKnown(key) {
		return "", fmt.Errorf("unknown key: %s", key)
	}

	val, found, err := r.Repo.Get(ctx, guildID, string(key))
	if err != nil {
		return "", err
	}

	if found && val != "" {
		return val, nil
	}

	defaultVal, hasDefault := DefaultValue(key)
	if hasDefault {
		return defaultVal, nil
	}

	return "", nil
}
