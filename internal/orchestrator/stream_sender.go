package orchestrator

import (
	"context"
	"log/slog"
	"strings"
	"sync"
)

type DiscordChunkSender interface {
	SendMessage(ctx context.Context, guildID, channelID int64, content string) error
}

type DiscordEditSender interface {
	SendMessageReturningID(ctx context.Context, guildID, channelID int64, content string) (int64, error)
	EditMessage(ctx context.Context, guildID, channelID, messageID int64, content string) error
}

const (
	streamFlushMinChars   = 200
	streamPendingFlushCap = 1500
)

type StreamingSender struct {
	out       DiscordChunkSender
	guildID   int64
	channelID int64

	mu      sync.Mutex
	pending strings.Builder
	active  strings.Builder
	activeID int64
	hasActive bool
	sent     int
	closed   bool
	lastErr  error

	limiter *ChannelRateLimiter
}

func NewStreamingSender(out DiscordChunkSender, limiter *ChannelRateLimiter, guildID, channelID int64) *StreamingSender {
	return &StreamingSender{
		out:       out,
		guildID:   guildID,
		channelID: channelID,
		limiter:   limiter,
	}
}

func (s *StreamingSender) Push(ctx context.Context, fragment string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.lastErr != nil {
		return s.lastErr
	}
	if s.closed {
		return nil
	}

	s.pending.WriteString(fragment)

	for {
		flushed, err := s.flushOnceLocked(ctx)
		if err != nil {
			s.lastErr = err
			slog.WarnContext(ctx, "stream_sender_emit_error", "guild", s.guildID, "channel", s.channelID, "err", err)
			return err
		}
		if !flushed {
			break
		}
	}
	return nil
}

func (s *StreamingSender) flushOnceLocked(ctx context.Context) (bool, error) {
	pendingLen := s.pending.Len()
	if pendingLen == 0 {
		return false, nil
	}

	pending := s.pending.String()
	pIdx := strings.LastIndex(pending, "\n\n")

	if pIdx < 0 {
		if s.activeRoomLocked()+pendingLen <= DiscordMessageLimit && pendingLen >= streamPendingFlushCap {
			return s.appendToActiveLocked(ctx, pending, true)
		}
		return false, nil
	}

	chunk := pending[:pIdx+2]
	rest := pending[pIdx+2:]

	if s.activeRoomLocked() >= len(chunk) && (s.active.Len()+len(chunk) >= streamFlushMinChars || s.active.Len() == 0) {
		s.pending.Reset()
		s.pending.WriteString(rest)
		if _, err := s.appendToActiveLocked(ctx, chunk, false); err != nil {
			return true, err
		}
		return true, nil
	}

	if s.active.Len() > 0 {
		if _, err := s.finalizeActiveLocked(ctx); err != nil {
			return true, err
		}
	}

	for len(chunk) > DiscordMessageLimit {
		head := chunk[:DiscordMessageLimit]
		if err := s.startNewMessageLocked(ctx, head); err != nil {
			return true, err
		}
		s.activeID, s.hasActive = 0, false
		s.active.Reset()
		chunk = chunk[DiscordMessageLimit:]
	}

	if chunk != "" {
		if err := s.startNewMessageLocked(ctx, chunk); err != nil {
			return true, err
		}
	}

	s.pending.Reset()
	s.pending.WriteString(rest)
	return true, nil
}

func (s *StreamingSender) activeRoomLocked() int {
	return DiscordMessageLimit - s.active.Len()
}

func (s *StreamingSender) appendToActiveLocked(ctx context.Context, text string, fromBudget bool) (bool, error) {
	if !s.hasActive {
		if err := s.startNewMessageLocked(ctx, text); err != nil {
			return false, err
		}
		if fromBudget {
			s.pending.Reset()
		}
		return true, nil
	}

	s.active.WriteString(text)
	if fromBudget {
		s.pending.Reset()
	}
	if err := s.editActiveLocked(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func (s *StreamingSender) startNewMessageLocked(ctx context.Context, content string) error {
	if err := s.limiter.Wait(ctx, s.channelID); err != nil {
		return err
	}
	editor, ok := s.out.(DiscordEditSender)
	if !ok {
		if err := s.out.SendMessage(ctx, s.guildID, s.channelID, content); err != nil {
			return err
		}
		s.active.Reset()
		s.active.WriteString(content)
		s.activeID, s.hasActive = 0, false
		s.sent++
		return nil
	}
	id, err := editor.SendMessageReturningID(ctx, s.guildID, s.channelID, content)
	if err != nil {
		return err
	}
	s.active.Reset()
	s.active.WriteString(content)
	s.activeID = id
	s.hasActive = id != 0
	s.sent++
	return nil
}

func (s *StreamingSender) editActiveLocked(ctx context.Context) error {
	editor, ok := s.out.(DiscordEditSender)
	if !ok || !s.hasActive {
		return nil
	}
	if err := s.limiter.Wait(ctx, s.channelID); err != nil {
		return err
	}
	return editor.EditMessage(ctx, s.guildID, s.channelID, s.activeID, s.active.String())
}

func (s *StreamingSender) finalizeActiveLocked(ctx context.Context) (bool, error) {
	if s.active.Len() == 0 {
		return false, nil
	}
	if s.hasActive {
		if err := s.editActiveLocked(ctx); err != nil {
			return false, err
		}
	}
	s.active.Reset()
	s.activeID, s.hasActive = 0, false
	return true, nil
}

func (s *StreamingSender) Flush(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}
	s.closed = true

	pending := s.pending.String()
	s.pending.Reset()

	for pending != "" {
		room := s.activeRoomLocked()
		if room <= 0 {
			if _, err := s.finalizeActiveLocked(ctx); err != nil {
				slog.WarnContext(ctx, "stream_sender_flush_error", "guild", s.guildID, "channel", s.channelID, "err", err)
				return nil
			}
			room = DiscordMessageLimit
		}

		take := len(pending)
		if take > room {
			take = boundaryWithin(pending, room)
		}
		piece := pending[:take]
		pending = pending[take:]

		if !s.hasActive && s.active.Len() == 0 {
			if err := s.startNewMessageLocked(ctx, piece); err != nil {
				slog.WarnContext(ctx, "stream_sender_flush_error", "guild", s.guildID, "channel", s.channelID, "err", err)
				return nil
			}
			continue
		}

		s.active.WriteString(piece)
		if err := s.editActiveLocked(ctx); err != nil {
			slog.WarnContext(ctx, "stream_sender_flush_error", "guild", s.guildID, "channel", s.channelID, "err", err)
			return nil
		}

		if pending != "" {
			s.active.Reset()
			s.activeID, s.hasActive = 0, false
		}
	}

	return nil
}

func boundaryWithin(s string, limit int) int {
	if limit >= len(s) {
		return len(s)
	}
	if i := strings.LastIndex(s[:limit], "\n\n"); i > 0 {
		return i + 2
	}
	if i := strings.LastIndex(s[:limit], "\n"); i > 0 {
		return i + 1
	}
	if i := strings.LastIndex(s[:limit], " "); i > 0 {
		return i + 1
	}
	return limit
}

func (s *StreamingSender) Discard() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}
	s.closed = true
	s.pending.Reset()
	s.active.Reset()
	s.activeID, s.hasActive = 0, false
	return nil
}

func (s *StreamingSender) SentCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sent
}
