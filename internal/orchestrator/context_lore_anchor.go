package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/eko/iris-bot/internal/domain"
)

// LoreAnchorResolver provides access to lore thread anchor metadata.
type LoreAnchorResolver interface {
	GetByThread(ctx context.Context, guildID int64, threadID int64) (*domain.LoreThreadAnchor, error)
}

// buildLoreAnchorLines formats the anchor summary message using the existing
// <channelname>|<threadname>|<userid>|<timestamp>|<message> format.
// Returns formatted lines ready for context injection, or an error if resolution fails.
// If the anchor cannot be resolved (sql.ErrNoRows), returns empty slice and nil error.
func buildLoreAnchorLines(
	ctx context.Context,
	resolver ChannelNameResolver,
	anchor *domain.LoreThreadAnchor,
) ([]string, error) {
	if anchor == nil {
		return nil, nil
	}

	// Resolve channel and thread names
	channelName := fmt.Sprintf("c%d", anchor.ChannelID)
	threadName := "-"
	if resolver != nil {
		if cn, tn, ok := resolver.Resolve(ctx, anchor.ThreadID); ok {
			if cn != "" {
				channelName = cn
			}
			if tn != "" {
				threadName = tn
			}
		}
	}

	// Use summary text from DB (fallback when Discord fetch is not available)
	summaryText := ""
	if anchor.SummaryText != nil {
		summaryText = *anchor.SummaryText
	}

	// Truncate if needed (match existing per-message cap behavior)
	// For now, use a reasonable default; this can be made configurable later
	const defaultSummaryCharCap = 500
	if utf8.RuneCountInString(summaryText) > defaultSummaryCharCap {
		summaryText = truncateRunesCB(summaryText, defaultSummaryCharCap)
	}

	// Normalize newlines to spaces (match existing behavior)
	summaryText = strings.ReplaceAll(summaryText, "\n", " ")

	// Format timestamp in RFC3339
	ts := anchor.CreatedAt.UTC().Format(time.RFC3339)

	// Use a synthetic user ID of 0 for the anchor summary (system-generated)
	line := fmt.Sprintf(tagFormatMessage, safeTag(channelName), safeTag(threadName), int64(0), ts, summaryText)

	// Prefix with [ANCHOR] marker so compactor can identify and preserve it
	anchorLine := "[ANCHOR] " + line

	return []string{anchorLine}, nil
}

// isThreadAllowed checks whether the parent channel of the lore thread
// is in the allowed channels list for the guild.
// Returns true if allowed, false if not allowed or if allowed channels are not configured.
func isThreadAllowed(
	ctx context.Context,
	allowedLister AllowedChannelLister,
	anchor *domain.LoreThreadAnchor,
) (bool, error) {
	if anchor == nil || allowedLister == nil {
		return false, nil
	}

	// If no allowed channels are configured, allow by default
	channelIDs, err := allowedLister.ListByGuild(ctx, anchor.GuildID)
	if err != nil {
		return false, fmt.Errorf("failed to list allowed channels: %w", err)
	}

	if len(channelIDs) == 0 {
		// No allowed channels configured; allow all
		return true, nil
	}

	// Check if parent channel is in the allowed list
	for _, cid := range channelIDs {
		if cid == anchor.ChannelID {
			return true, nil
		}
	}

	return false, nil
}
