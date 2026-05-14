package memesearch

import "context"

type SocialAdapter interface {
	Search(ctx context.Context, query string, limit int) ([]MediaItem, error)
	Source() Source
}
