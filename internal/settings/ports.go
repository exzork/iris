package settings

import "context"

type SettingsRepo interface {
	Get(ctx context.Context, guildID int64, key string) (value string, found bool, err error)
	Set(ctx context.Context, guildID int64, key, value string) error
	List(ctx context.Context, guildID int64) (map[string]string, error)
}
