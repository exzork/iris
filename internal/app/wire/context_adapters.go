package wire

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/eko/iris-bot/internal/domain"
	"github.com/eko/iris-bot/internal/embedder"
	"github.com/eko/iris-bot/internal/llm"
	"github.com/eko/iris-bot/internal/orchestrator"
	"github.com/eko/iris-bot/internal/repository"
)

type AllowedChannelListerAdapter struct {
	Repo *repository.AllowedChannelRepo
}

func (a *AllowedChannelListerAdapter) ListByGuild(ctx context.Context, guildID int64) ([]int64, error) {
	if a.Repo == nil {
		return nil, nil
	}
	return a.Repo.ListByGuild(ctx, guildID)
}

type channelNameCacheEntry struct {
	channelName string
	threadName  string
	expiresAt   time.Time
}

type SessionChannelNameResolver struct {
	Session *discordgo.Session
	TTL     time.Duration

	mu    sync.RWMutex
	cache map[int64]channelNameCacheEntry
}

func NewSessionChannelNameResolver(session *discordgo.Session) *SessionChannelNameResolver {
	return &SessionChannelNameResolver{
		Session: session,
		TTL:     5 * time.Minute,
		cache:   make(map[int64]channelNameCacheEntry),
	}
}

func (r *SessionChannelNameResolver) Resolve(ctx context.Context, channelID int64) (string, string, bool) {
	if r == nil || r.Session == nil || channelID == 0 {
		return "", "", false
	}

	now := time.Now()
	r.mu.RLock()
	if e, ok := r.cache[channelID]; ok && now.Before(e.expiresAt) {
		r.mu.RUnlock()
		return e.channelName, e.threadName, true
	}
	r.mu.RUnlock()

	ch, err := r.Session.State.Channel(strconv.FormatInt(channelID, 10))
	if err != nil || ch == nil {
		ch, err = r.Session.Channel(strconv.FormatInt(channelID, 10))
		if err != nil || ch == nil {
			return "", "", false
		}
	}

	channelName := ch.Name
	threadName := ""
	if isThreadType(ch.Type) {
		threadName = ch.Name
		channelName = ""
		if ch.ParentID != "" {
			parent, pErr := r.Session.State.Channel(ch.ParentID)
			if pErr != nil || parent == nil {
				parent, _ = r.Session.Channel(ch.ParentID)
			}
			if parent != nil {
				channelName = parent.Name
			}
		}
	}

	r.mu.Lock()
	r.cache[channelID] = channelNameCacheEntry{
		channelName: channelName,
		threadName:  threadName,
		expiresAt:   now.Add(r.TTL),
	}
	r.mu.Unlock()
	return channelName, threadName, true
}

func isThreadType(t discordgo.ChannelType) bool {
	switch t {
	case discordgo.ChannelTypeGuildPublicThread,
		discordgo.ChannelTypeGuildPrivateThread,
		discordgo.ChannelTypeGuildNewsThread:
		return true
	}
	return false
}

type LLMCompactor struct {
	Client *llm.Client
	Model  string
}

func (c *LLMCompactor) Compact(ctx context.Context, guildID int64, lines []string) (string, error) {
	if c.Client == nil || len(lines) == 0 {
		return "", nil
	}

	joined := joinLinesCapped(lines, 60000)

	system := "You compact Discord chat transcripts into dense factual summaries. " +
		"Preserve user IDs, channel names, thread names, timestamps, topics, decisions, and unresolved questions. " +
		"Drop pleasantries and filler. Output plain text, 15 short bullet-style lines max, English or Indonesian matching source."
	user := "Compact these Discord lines into a dense summary. Format of each line is " +
		"<channel>|<thread>|<userid>|<timestamp>|<message>. Retain who said what, when, and in which channel when material.\n\n" +
		joined

	messages := []map[string]string{
		{"role": "system", "content": system},
		{"role": "user", "content": user},
	}

	model := c.Model
	if model == "" {
		return c.Client.Chat(ctx, guildID, messages)
	}
	return c.Client.ChatWithModel(ctx, model, guildID, messages)
}

func joinLinesCapped(lines []string, maxBytes int) string {
	total := 0
	for _, l := range lines {
		total += len(l) + 1
	}
	if total <= maxBytes {
		return joinAll(lines)
	}

	for len(lines) > 1 && totalBytes(lines) > maxBytes {
		lines = lines[1:]
	}
	return joinAll(lines)
}

func joinAll(lines []string) string {
	out := ""
	for i, l := range lines {
		if i > 0 {
			out += "\n"
		}
		out += l
	}
	return out
}

func totalBytes(lines []string) int {
	n := 0
	for _, l := range lines {
		n += len(l) + 1
	}
	return n
}

var _ orchestrator.AllowedChannelLister = (*AllowedChannelListerAdapter)(nil)
var _ orchestrator.ChannelNameResolver = (*SessionChannelNameResolver)(nil)
var _ orchestrator.Compactor = (*LLMCompactor)(nil)

type EpisodeArchiverAdapter struct {
	Repo           *repository.EpisodeMemoryRepo
	Embedder       embedder.Embedder
	EmbeddingModel string
}

func (a *EpisodeArchiverAdapter) Archive(
	ctx context.Context,
	guildID int64,
	messages []*domain.ChannelMessage,
	taggedLines []string,
	resolver orchestrator.ChannelNameResolver,
) error {
	if a == nil || a.Repo == nil {
		return nil
	}
	if guildID == 0 || len(messages) == 0 {
		return nil
	}

	episodes := make([]*domain.EpisodeMemory, 0, len(messages))
	for i, m := range messages {
		if m == nil {
			continue
		}
		line := ""
		if i < len(taggedLines) {
			line = taggedLines[i]
		}
		author := ""
		if m.AuthorName != nil {
			author = *m.AuthorName
		}
		channelName, threadName := "", ""
		var threadID *int64
		if resolver != nil {
			if cn, tn, ok := resolver.Resolve(ctx, m.ChannelID); ok {
				channelName = cn
				threadName = tn
				if tn != "" {
					id := m.ChannelID
					threadID = &id
				}
			}
		}
		ep := &domain.EpisodeMemory{
			GuildID:     guildID,
			ChannelID:   m.ChannelID,
			ThreadID:    threadID,
			ChannelName: channelName,
			ThreadName:  threadName,
			UserID:      m.UserID,
			AuthorName:  author,
			MessageID:   m.MessageID,
			Content:     m.Content,
			TaggedLine:  line,
			OccurredAt:  m.CreatedAt,
		}
		if a.Embedder != nil {
			if vec, err := a.Embedder.Embed(ctx, m.Content); err == nil && len(vec) == repository.ExpectedEmbeddingDim {
				ep.Embedding = vec
				ep.EmbeddingModel = a.EmbeddingModel
			}
		}
		episodes = append(episodes, ep)
	}

	if len(episodes) == 0 {
		return nil
	}

	_, err := a.Repo.SaveBatch(ctx, episodes)
	return err
}

var _ orchestrator.EpisodeArchiver = (*EpisodeArchiverAdapter)(nil)

type LoreThreadAnchorResolverAdapter struct {
	Repo *repository.LoreThreadAnchorRepo
}

func (a *LoreThreadAnchorResolverAdapter) GetByThread(ctx context.Context, guildID int64, threadID int64) (*domain.LoreThreadAnchor, error) {
	if a == nil || a.Repo == nil {
		return nil, nil
	}
	return a.Repo.GetByThread(ctx, guildID, threadID)
}

var _ orchestrator.LoreAnchorResolver = (*LoreThreadAnchorResolverAdapter)(nil)

