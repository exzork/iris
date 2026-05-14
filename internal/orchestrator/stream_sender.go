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

// DiscordReplyEditSender is the optional capability used by the streaming
// sender to anchor the very first outbound message as a Discord reply with
// ping enabled. Any subsequent flush/edit goes through the regular
// DiscordEditSender path so we never re-ping the user mid-stream.
type DiscordReplyEditSender interface {
	ReplyMessageReturningID(ctx context.Context, guildID, channelID, replyToMessageID int64, content string, mentionRepliedUser bool) (int64, error)
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

	replyToMessageID   int64
	mentionRepliedUser bool
	replyConsumed      bool

	outboundTransform func(string) string
}

func NewStreamingSender(out DiscordChunkSender, limiter *ChannelRateLimiter, guildID, channelID int64) *StreamingSender {
	return &StreamingSender{
		out:       out,
		guildID:   guildID,
		channelID: channelID,
		limiter:   limiter,
	}
}

// WithReply configures the streaming sender to send the FIRST outbound
// message as a Discord reply pinging the original triggering message.
// Subsequent flushes use the regular send/edit path so the user is not
// re-pinged. Calling this after a message has already been sent is a no-op.
func (s *StreamingSender) WithReply(replyToMessageID int64, mentionRepliedUser bool) *StreamingSender {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sent > 0 {
		return s
	}
	s.replyToMessageID = replyToMessageID
	s.mentionRepliedUser = mentionRepliedUser
	s.replyConsumed = false
	return s
}

// WithOutboundTransform configures the streaming sender to apply a transform
// function to every outbound chunk before sending to Discord. The transform
// is applied only on the way out; the internal active buffer remains raw so
// subsequent appends/edits see the same growing buffer. If fn is nil, no
// transform is applied. Returns s for chaining.
func (s *StreamingSender) WithOutboundTransform(fn func(string) string) *StreamingSender {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.outboundTransform = fn
	return s
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

	outbound := content
	if s.outboundTransform != nil {
		outbound = s.outboundTransform(content)
	}

	useReply := !s.replyConsumed && s.replyToMessageID != 0

	if useReply {
		if replier, ok := s.out.(DiscordReplyEditSender); ok {
			id, err := replier.ReplyMessageReturningID(ctx, s.guildID, s.channelID, s.replyToMessageID, outbound, s.mentionRepliedUser)
			if err != nil {
				return err
			}
			s.active.Reset()
			s.active.WriteString(content)
			s.activeID = id
			s.hasActive = id != 0
			s.sent++
			s.replyConsumed = true
			return nil
		}
	}

	editor, ok := s.out.(DiscordEditSender)
	if !ok {
		if err := s.out.SendMessage(ctx, s.guildID, s.channelID, outbound); err != nil {
			return err
		}
		s.active.Reset()
		s.active.WriteString(content)
		s.activeID, s.hasActive = 0, false
		s.sent++
		s.replyConsumed = true
		return nil
	}
	id, err := editor.SendMessageReturningID(ctx, s.guildID, s.channelID, outbound)
	if err != nil {
		return err
	}
	s.active.Reset()
	s.active.WriteString(content)
	s.activeID = id
	s.hasActive = id != 0
	s.sent++
	s.replyConsumed = true
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
	outbound := s.active.String()
	if s.outboundTransform != nil {
		outbound = s.outboundTransform(outbound)
	}
	return editor.EditMessage(ctx, s.guildID, s.channelID, s.activeID, outbound)
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
