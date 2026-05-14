package memory

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/eko/iris-bot/internal/domain"
)

// ErrMissingUserID is returned when a per-user profile operation is attempted
// without a Discord user id.
var ErrMissingUserID = errors.New("memory: user_id is required")

// BehaviorProfileStore is the repository shape used by BehaviorProfileService.
type BehaviorProfileStore interface {
	GetByGuildUser(ctx context.Context, guildID int64, userID int64) (*domain.UserBehaviorProfile, error)
	Upsert(ctx context.Context, p *domain.UserBehaviorProfile) error
}

// MessageSample is a single observed message fed into profile synthesis.
type MessageSample struct {
	Content   string
	CreatedAt time.Time
	IsBot     bool
}

// BehaviorProfileService owns guild-scoped personality recognition. It only
// ever writes profiles keyed by (guild_id, user_id) and only synthesizes
// non-sensitive interaction hints (style, formality, response-length
// preference, formatting preference, recurring neutral topics).
type BehaviorProfileService struct {
	store BehaviorProfileStore
}

func NewBehaviorProfileService(store BehaviorProfileStore) (*BehaviorProfileService, error) {
	if store == nil {
		return nil, errors.New("behavior profile: store is nil")
	}
	return &BehaviorProfileService{store: store}, nil
}

// Get returns the same-guild same-user profile or nil.
func (s *BehaviorProfileService) Get(ctx context.Context, guildID int64, userID int64) (*domain.UserBehaviorProfile, error) {
	if guildID == 0 {
		return nil, ErrMissingGuildID
	}
	if userID == 0 {
		return nil, ErrMissingUserID
	}
	return s.store.GetByGuildUser(ctx, guildID, userID)
}

// UpdateFromSamples synthesizes a profile from a slice of the user's messages
// inside the given guild. Samples from other guilds must be filtered by the
// caller; this function refuses to operate without a guildID.
func (s *BehaviorProfileService) UpdateFromSamples(ctx context.Context, guildID int64, userID int64, samples []MessageSample) (*domain.UserBehaviorProfile, error) {
	if guildID == 0 {
		return nil, ErrMissingGuildID
	}
	if userID == 0 {
		return nil, ErrMissingUserID
	}

	filtered := make([]MessageSample, 0, len(samples))
	var lastObserved time.Time
	for _, m := range samples {
		if m.IsBot {
			continue
		}
		c := strings.TrimSpace(m.Content)
		if c == "" {
			continue
		}
		if containsSensitive(c) {
			continue
		}
		filtered = append(filtered, MessageSample{Content: c, CreatedAt: m.CreatedAt, IsBot: false})
		if m.CreatedAt.After(lastObserved) {
			lastObserved = m.CreatedAt
		}
	}
	if len(filtered) == 0 {
		return nil, nil
	}

	profile := &domain.UserBehaviorProfile{
		GuildID:                  guildID,
		UserID:                   userID,
		CommunicationStyle:       summarizeStyle(filtered),
		Formality:                summarizeFormality(filtered),
		ResponseLengthPreference: summarizeLengthPreference(filtered),
		FormattingPreference:     summarizeFormatting(filtered),
		RecurringTopics:          topRecurringTopics(filtered, 5),
		EvidenceCount:            len(filtered),
		LastObservedAt:           lastObserved,
	}
	if err := s.store.Upsert(ctx, profile); err != nil {
		return nil, err
	}
	return profile, nil
}

var sensitivePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(race|religion|gender|sexuality|political|politics|orient(ed|ation))\b`),
	regexp.MustCompile(`(?i)\b(depressed|suicide|self[- ]?harm|diagnos(ed|is)|medication|illness)\b`),
	regexp.MustCompile(`(?i)\b(password|token|secret|api[_\- ]?key|credit[_\- ]?card)\b`),
	regexp.MustCompile(`(?i)\b(minor|underage|age\s*\d{1,2})\b`),
}

func containsSensitive(s string) bool {
	for _, re := range sensitivePatterns {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}

func summarizeStyle(samples []MessageSample) string {
	playful, technical := 0, 0
	for _, m := range samples {
		if strings.ContainsAny(m.Content, ":)!?😂🙂😀🤣") {
			playful++
		}
		if strings.ContainsAny(m.Content, "`{}[]") || strings.Contains(m.Content, "http") {
			technical++
		}
	}
	switch {
	case playful > technical && playful > 0:
		return "playful"
	case technical > playful && technical > 0:
		return "technical"
	default:
		return "neutral"
	}
}

func summarizeFormality(samples []MessageSample) string {
	informal := 0
	formal := 0
	for _, m := range samples {
		lc := strings.ToLower(m.Content)
		if strings.Contains(lc, "lol") || strings.Contains(lc, "lmao") || strings.Contains(lc, "wkwk") {
			informal++
		}
		if strings.HasPrefix(strings.TrimSpace(m.Content), "Hello") ||
			strings.HasPrefix(strings.TrimSpace(m.Content), "Halo") {
			formal++
		}
	}
	switch {
	case informal > formal && informal > 0:
		return "informal"
	case formal > informal && formal > 0:
		return "formal"
	default:
		return "neutral"
	}
}

func summarizeLengthPreference(samples []MessageSample) string {
	var total int
	for _, m := range samples {
		total += len(m.Content)
	}
	avg := total / len(samples)
	switch {
	case avg <= 40:
		return "concise"
	case avg >= 200:
		return "long"
	default:
		return "medium"
	}
}

func summarizeFormatting(samples []MessageSample) string {
	markdown := 0
	for _, m := range samples {
		if strings.Contains(m.Content, "**") || strings.Contains(m.Content, "```") ||
			strings.Contains(m.Content, "- ") || strings.Contains(m.Content, "#") {
			markdown++
		}
	}
	if markdown*2 >= len(samples) {
		return "markdown"
	}
	return "plain"
}

var tokenSplit = regexp.MustCompile(`[^\p{L}\p{N}_]+`)

var stopwords = map[string]struct{}{
	"the": {}, "and": {}, "you": {}, "for": {}, "with": {}, "that": {},
	"this": {}, "are": {}, "was": {}, "but": {}, "not": {}, "have": {},
	"your": {}, "its": {}, "from": {}, "just": {}, "like": {}, "what": {},
	"yang": {}, "dan": {}, "atau": {}, "ini": {}, "itu": {}, "aku": {},
	"kamu": {}, "gue": {}, "lu": {}, "sih": {}, "lah": {},
}

func topRecurringTopics(samples []MessageSample, max int) []string {
	counts := map[string]int{}
	for _, m := range samples {
		for _, tok := range tokenSplit.Split(strings.ToLower(m.Content), -1) {
			if len(tok) < 4 {
				continue
			}
			if _, ok := stopwords[tok]; ok {
				continue
			}
			counts[tok]++
		}
	}
	type kv struct {
		k string
		v int
	}
	list := make([]kv, 0, len(counts))
	for k, v := range counts {
		if v < 2 {
			continue
		}
		list = append(list, kv{k, v})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].v != list[j].v {
			return list[i].v > list[j].v
		}
		return list[i].k < list[j].k
	})
	if len(list) > max {
		list = list[:max]
	}
	out := make([]string, 0, len(list))
	for _, kv := range list {
		out = append(out, kv.k)
	}
	return out
}
