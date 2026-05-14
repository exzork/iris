package lorethread

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
)

var (
	ErrNoOpenSession         = errors.New("no open lore session in this channel")
	ErrNotConversationStarter = errors.New("only the conversation starter can finalize the session")
)

// ForceFinalizeResult is the result of force-finalizing a lore session.
type ForceFinalizeResult struct {
	ThreadID       int64
	MessageID      int64
	Title          string
	SummaryPreview string
}

// Finalizer force-finalizes an open lore session on-demand.
type Finalizer struct {
	SessionStore       SessionStore
	MessageFetcher     MessageFetcher
	LoreSummarizer     LoreSummarizer
	TitleGenerator     TitleGenerator
	ThreadCreator      ThreadCreator
	ThreadAnchorStore  ThreadAnchorStore
	Clock              Clock
	Limiter            Limiter
	GuildSettingsStore GuildSettingsStore
	LoreClassifier     LoreClassifier
	MetricsHooks       *MetricsHooks
}

// ForceFinalize closes the current channel's open lore session and posts a summary thread NOW.
// Only callable by the user who started the lore conversation.
// Returns ErrNoOpenSession if no open session exists.
// Returns ErrNotConversationStarter if requesterUserID != session starter.
func (f *Finalizer) ForceFinalize(ctx context.Context, guildID int64, channelID int64, requesterUserID int64) (*ForceFinalizeResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Fetch the open session with starter info
	sessionStore, ok := f.SessionStore.(interface {
		GetOpenByChannelWithStarter(ctx context.Context, guildID, channelID int64) (*Session, int64, error)
	})
	if !ok {
		return nil, errors.New("session store does not support GetOpenByChannelWithStarter")
	}

	session, starterID, err := sessionStore.GetOpenByChannelWithStarter(ctx, guildID, channelID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNoOpenSession, err)
	}
	if session == nil {
		return nil, ErrNoOpenSession
	}

	// Check authorization: only the starter can finalize
	if starterID == 0 || starterID != requesterUserID {
		return nil, ErrNotConversationStarter
	}

	// Check guild settings
	enabled, err := f.GuildSettingsStore.GetLoreThreadEnabled(ctx, guildID)
	if err != nil {
		return nil, fmt.Errorf("guild settings check failed: %w", err)
	}
	if !enabled {
		return nil, errors.New("lore threads disabled for this guild")
	}

	// Check rate limit
	if !f.Limiter.Allow(ctx, guildID) {
		return nil, errors.New("rate limit exceeded for this guild")
	}

	// Get parent message
	parentMessageID := session.FirstLoreMessageID
	if parentMessageID == 0 {
		return nil, errors.New("session has no parent message ID")
	}

	parentMsg, err := f.MessageFetcher.FetchByID(ctx, guildID, parentMessageID)
	if err != nil {
		return nil, fmt.Errorf("fetch parent message failed: %w", err)
	}
	if parentMsg == nil {
		return nil, errors.New("parent message not found")
	}

	// Fetch recent messages
	recentMessages, err := f.MessageFetcher.FetchRecent(ctx, guildID, channelID, 200)
	if err != nil {
		return nil, fmt.Errorf("fetch recent messages failed: %w", err)
	}

	// Select and filter lore messages
	candidates := selectCandidateMessages(parentMsg, recentMessages, session)
	loreMessages, err := f.filterLoreMessages(ctx, guildID, candidates)
	if err != nil {
		return nil, fmt.Errorf("filter lore messages failed: %w", err)
	}

	// Generate title and summary
	title, summary, err := f.generateTitleAndSummary(ctx, guildID, loreMessages)
	if err != nil {
		return nil, fmt.Errorf("generate title/summary failed: %w", err)
	}

	// Create thread
	threadResult, err := f.ThreadCreator.Create(ctx, &ThreadCreateRequest{
		GuildID:         guildID,
		ChannelID:       channelID,
		ParentMessageID: parentMessageID,
		Title:           title,
		FirstMessage:    summary,
	})
	if err != nil {
		if errors.Is(err, ErrDMNotSupported) {
			return nil, errors.New("thread creation not supported in DM channels")
		}
		return nil, fmt.Errorf("thread creation failed: %w", err)
	}
	if threadResult == nil {
		return nil, errors.New("thread creation returned nil result")
	}

	// Store thread result
	dueStore, ok := f.SessionStore.(interface {
		SetThreadResult(ctx context.Context, id int64, threadID int64, summaryMsgID int64, title string, summary string) error
		MarkStatus(ctx context.Context, id int64, status string) error
	})
	if !ok {
		return nil, errors.New("session store does not support SetThreadResult")
	}

	if err := dueStore.SetThreadResult(ctx, session.ID, threadResult.ThreadID, threadResult.MessageID, title, summary); err != nil {
		return nil, fmt.Errorf("failed to store thread result: %w", err)
	}

	// Store anchor metadata
	if f.ThreadAnchorStore != nil {
		if err := f.ThreadAnchorStore.Create(ctx, session.ID, threadResult.ThreadID, threadResult.MessageID); err != nil {
			slog.WarnContext(ctx, "failed to store thread anchor", "error", err)
		}
	}

	if f.MetricsHooks != nil && f.MetricsHooks.OnThreadCreated != nil {
		f.MetricsHooks.OnThreadCreated()
	}

	// Truncate summary preview to 100 chars
	preview := summary
	if len(preview) > 100 {
		preview = preview[:100] + "…"
	}

	return &ForceFinalizeResult{
		ThreadID:       threadResult.ThreadID,
		MessageID:      threadResult.MessageID,
		Title:          title,
		SummaryPreview: preview,
	}, nil
}

// filterLoreMessages filters candidate messages to only lore-relevant ones.
func (f *Finalizer) filterLoreMessages(ctx context.Context, guildID int64, candidates []*Message) ([]*Message, error) {
	if f.LoreClassifier == nil {
		return candidates, nil
	}

	var loreMessages []*Message
	for _, msg := range candidates {
		result, err := f.LoreClassifier.Classify(ctx, guildID, msg)
		if err != nil {
			slog.WarnContext(ctx, "lore classification failed", "error", err)
			continue
		}
		if result != nil && result.IsLore {
			loreMessages = append(loreMessages, msg)
		}
	}
	return loreMessages, nil
}

// generateTitleAndSummary generates title and summary for the lore messages.
func (f *Finalizer) generateTitleAndSummary(ctx context.Context, guildID int64, messages []*Message) (string, string, error) {
	if len(messages) == 0 {
		return "Lore Discussion", "No lore messages found.", nil
	}

	// Generate summary
	summaryResult, err := f.LoreSummarizer.Summarize(ctx, &SummaryRequest{
		GuildID:  guildID,
		Messages: messages,
	})
	if err != nil {
		return "", "", fmt.Errorf("summarization failed: %w", err)
	}

	// Generate title
	title, err := f.TitleGenerator.Generate(ctx, guildID, messages)
	if err != nil {
		return "", "", fmt.Errorf("title generation failed: %w", err)
	}

	return title, summaryResult.Summary, nil
}
