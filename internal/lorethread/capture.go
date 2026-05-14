package lorethread

import (
	"context"
	"log/slog"
	"time"
)

// CapturerDeps holds dependencies for the Capturer.
type CapturerDeps struct {
	SessionStore       SessionStore
	GuildSettingsStore GuildSettingsStore
	LoreClassifier     LoreClassifier
	Clock              Clock
	IdleDuration       time.Duration
	Enabled            bool
	MetricsHooks       *MetricsHooks
}

// Capturer captures lore messages and maintains lore sessions.
type Capturer struct {
	sessionStore       SessionStore
	guildSettingsStore GuildSettingsStore
	loreClassifier     LoreClassifier
	clock              Clock
	idleDuration       time.Duration
	enabled            bool
	metricsHooks       *MetricsHooks
}

// NewCapturer creates a new Capturer.
func NewCapturer(deps CapturerDeps) *Capturer {
	hooks := deps.MetricsHooks
	if hooks == nil {
		hooks = NoOpMetricsHooks()
	}
	return &Capturer{
		sessionStore:       deps.SessionStore,
		guildSettingsStore: deps.GuildSettingsStore,
		loreClassifier:     deps.LoreClassifier,
		clock:              deps.Clock,
		idleDuration:       deps.IdleDuration,
		enabled:            deps.Enabled,
		metricsHooks:       hooks,
	}
}

// OnMessage processes an incoming message for lore capture.
// Returns nil if the message is not lore-relevant or capture is disabled.
// Does not block on classifier calls; errors are logged at DEBUG level.
func (c *Capturer) OnMessage(ctx context.Context, msg *Message) error {
	// Feature disabled
	if !c.enabled {
		return nil
	}

	// Skip DMs
	if msg.GuildID == 0 {
		return nil
	}

	// Skip bot messages (v1: exclude all bot messages unless classifier says lore-relevant)
	if msg.AuthorIsBot {
		return nil
	}

	// Classify the message
	classifyResult, err := c.loreClassifier.Classify(ctx, msg.GuildID, msg)
	if err != nil {
		c.metricsHooks.OnClassifierFailure()
		slog.DebugContext(ctx, "lore_classify_error", "guild", msg.GuildID, "channel", msg.ChannelID, "error", err)
		return nil
	}

	// Not lore-relevant
	if !classifyResult.IsLore {
		return nil
	}

	// Check if lore threads are enabled for this guild
	enabled, err := c.guildSettingsStore.GetLoreThreadEnabled(ctx, msg.GuildID)
	if err != nil {
		slog.DebugContext(ctx, "lore_settings_check_error", "guild", msg.GuildID, "error", err)
		return nil
	}

	if !enabled {
		return nil
	}

	// Get or create active session
	session, err := c.sessionStore.GetActive(ctx, msg.GuildID, msg.ChannelID)
	if err != nil {
		slog.DebugContext(ctx, "lore_session_get_error", "guild", msg.GuildID, "channel", msg.ChannelID, "error", err)
		return nil
	}

	now := c.clock.Now()

	if session == nil {
		// Create new session
		newSession := &Session{
			GuildID:      msg.GuildID,
			ChannelID:    msg.ChannelID,
			FirstMessage: msg,
			Messages:     []*Message{msg},
			CreatedAt:    now,
			UpdatedAt:    now,
			IsActive:     true,
		}

		if err := c.sessionStore.Create(ctx, newSession); err != nil {
			slog.DebugContext(ctx, "lore_session_create_error", "guild", msg.GuildID, "channel", msg.ChannelID, "error", err)
			return nil
		}
		c.metricsHooks.OnSessionOpened()
	} else {
		// Refresh existing session
		session.Messages = append(session.Messages, msg)
		session.UpdatedAt = now

		if err := c.sessionStore.Update(ctx, session); err != nil {
			slog.DebugContext(ctx, "lore_session_update_error", "guild", msg.GuildID, "channel", msg.ChannelID, "error", err)
			return nil
		}
	}

	return nil
}
