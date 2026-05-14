package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/eko/iris-bot/internal/domain"
)

type fakeAllowedLister struct {
	channels []int64
	err      error
}

func (f *fakeAllowedLister) ListByGuild(ctx context.Context, guildID int64) ([]int64, error) {
	return f.channels, f.err
}

type fakeNameResolver struct {
	names map[int64][2]string
}

func (f *fakeNameResolver) Resolve(ctx context.Context, channelID int64) (string, string, bool) {
	v, ok := f.names[channelID]
	if !ok {
		return "", "", false
	}
	return v[0], v[1], true
}

type multiChannelStore struct {
	byChannel map[int64][]*domain.ChannelMessage
	byID      map[int64]*domain.ChannelMessage
}

func (s *multiChannelStore) ListRecent(ctx context.Context, guildID, channelID int64, limit int) ([]*domain.ChannelMessage, error) {
	msgs := s.byChannel[channelID]
	if len(msgs) > limit {
		msgs = msgs[len(msgs)-limit:]
	}
	return msgs, nil
}

func (s *multiChannelStore) GetByID(ctx context.Context, guildID, messageID int64) (*domain.ChannelMessage, error) {
	return s.byID[messageID], nil
}

func (s *multiChannelStore) ListByUserAcrossChannels(ctx context.Context, guildID int64, userID int64, sinceMinutes int, limit int) ([]*domain.ChannelMessage, error) {
	return nil, nil
}

type fakeCompactor struct {
	summary string
	calls   int
	got     []string
}

func (c *fakeCompactor) Compact(ctx context.Context, guildID int64, lines []string) (string, error) {
	c.calls++
	c.got = lines
	return c.summary, nil
}

func makeMsg(channelID, userID, msgID int64, content string, createdAt time.Time) *domain.ChannelMessage {
	name := "user"
	return &domain.ChannelMessage{
		ChannelID:  channelID,
		UserID:     userID,
		MessageID:  msgID,
		AuthorName: &name,
		Content:    content,
		CreatedAt:  createdAt,
	}
}

func TestContextBuilder_AllowedChannels_TaggedFormat(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	store := &multiChannelStore{
		byChannel: map[int64][]*domain.ChannelMessage{
			111: {
				makeMsg(111, 501, 1001, "halo dari general", t0),
				makeMsg(111, 502, 1002, "halo juga", t0.Add(10*time.Second)),
			},
			222: {
				makeMsg(222, 503, 2001, "balasan di thread", t0.Add(20*time.Second)),
			},
		},
	}

	cb := NewContextBuilder(ContextBuilderConfig{
		CurrentChannelMax:         10,
		PerMessageCharCap:         200,
		IncludeAllAllowedChannels: true,
		PerChannelLimit:           20,
		TotalCharBudget:           40000,
		CompactionKeepRecent:      10,
	})
	cb.WithAllowedChannels(&fakeAllowedLister{channels: []int64{111, 222}})
	cb.WithChannelNames(&fakeNameResolver{
		names: map[int64][2]string{
			111: {"general", ""},
			222: {"general", "thread-lore"},
		},
	})

	event := &domain.DiscordEvent{
		GuildID:   1,
		ChannelID: 111,
		UserID:    500,
		Message: &domain.DiscordMessage{
			ID:      9999,
			Content: "pesan terbaru",
		},
	}

	msgs, err := cb.Build(context.Background(), event, store, "sys")
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}

	combined := ""
	for _, m := range msgs {
		combined += "\n" + m["content"]
	}

	for _, want := range []string{
		"<general|-|501|",
		"<general|-|502|",
		"<general|thread-lore|503|",
		"halo dari general",
		"halo juga",
		"balasan di thread",
	} {
		if !strings.Contains(combined, want) {
			t.Errorf("missing %q in rendered context:\n%s", want, combined)
		}
	}

	if !strings.Contains(combined, "ALLOWED-CHANNELS CONTEXT") {
		t.Error("expected header block for allowed-channels context")
	}
}

func TestContextBuilder_AllowedChannels_Compaction(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	var msgs []*domain.ChannelMessage
	for i := 0; i < 50; i++ {
		msgs = append(msgs, makeMsg(111, 500, int64(1000+i), "line with enough content "+strings.Repeat("x", 80), t0.Add(time.Duration(i)*time.Second)))
	}
	store := &multiChannelStore{
		byChannel: map[int64][]*domain.ChannelMessage{111: msgs},
	}

	compactor := &fakeCompactor{summary: "compacted summary of older chats"}

	cb := NewContextBuilder(ContextBuilderConfig{
		CurrentChannelMax:         50,
		PerMessageCharCap:         200,
		IncludeAllAllowedChannels: true,
		PerChannelLimit:           50,
		TotalCharBudget:           2000,
		CompactionKeepRecent:      5,
	})
	cb.WithAllowedChannels(&fakeAllowedLister{channels: []int64{111}})
	cb.WithCompactor(compactor)

	event := &domain.DiscordEvent{
		GuildID:   1,
		ChannelID: 111,
		UserID:    500,
		Message: &domain.DiscordMessage{
			ID:      9999,
			Content: "trigger",
		},
	}

	built, err := cb.Build(context.Background(), event, store, "sys")
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}

	if compactor.calls != 1 {
		t.Fatalf("expected compactor called once, got %d", compactor.calls)
	}
	if len(compactor.got) != 45 {
		t.Errorf("expected 45 older lines to compactor, got %d", len(compactor.got))
	}

	var big string
	for _, m := range built {
		big += "\n" + m["content"]
	}
	if !strings.Contains(big, "[SUMMARY of 45 older messages]") {
		t.Errorf("expected summary header, got:\n%s", big)
	}
	if !strings.Contains(big, "compacted summary of older chats") {
		t.Errorf("expected compactor output in context:\n%s", big)
	}
	if !strings.Contains(big, fmt.Sprintf("%d", msgs[len(msgs)-1].UserID)) {
		t.Error("expected the last message userID to be present (recent window preserved)")
	}
}

func TestContextBuilder_AllowedChannels_NoCompactionUnderBudget(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	store := &multiChannelStore{
		byChannel: map[int64][]*domain.ChannelMessage{
			111: {
				makeMsg(111, 500, 1001, "hi", t0),
				makeMsg(111, 501, 1002, "hello", t0.Add(5*time.Second)),
			},
		},
	}

	compactor := &fakeCompactor{summary: "should not be used"}

	cb := NewContextBuilder(ContextBuilderConfig{
		CurrentChannelMax:         10,
		PerMessageCharCap:         200,
		IncludeAllAllowedChannels: true,
		PerChannelLimit:           20,
		TotalCharBudget:           40000,
		CompactionKeepRecent:      10,
	})
	cb.WithAllowedChannels(&fakeAllowedLister{channels: []int64{111}})
	cb.WithCompactor(compactor)

	event := &domain.DiscordEvent{
		GuildID:   1,
		ChannelID: 111,
		Message: &domain.DiscordMessage{
			ID:      9999,
			Content: "trigger",
		},
	}

	if _, err := cb.Build(context.Background(), event, store, "sys"); err != nil {
		t.Fatalf("Build error: %v", err)
	}

	if compactor.calls != 0 {
		t.Errorf("compactor should not fire under budget, called %d times", compactor.calls)
	}
}

func TestSafeTag(t *testing.T) {
	cases := map[string]string{
		"general":        "general",
		"lore|stuff":     "lore/stuff",
		"<channel>":      "(channel)",
		strings.Repeat("a", 80): strings.Repeat("a", 64),
	}
	for in, want := range cases {
		if got := safeTag(in); got != want {
			t.Errorf("safeTag(%q) = %q, want %q", in, got, want)
		}
	}
}

type fakeArchiver struct {
	calls    int
	messages []*domain.ChannelMessage
	lines    []string
	err      error
}

func (f *fakeArchiver) Archive(
	ctx context.Context,
	guildID int64,
	messages []*domain.ChannelMessage,
	taggedLines []string,
	resolver ChannelNameResolver,
) error {
	f.calls++
	f.messages = messages
	f.lines = taggedLines
	return f.err
}

func TestContextBuilder_AllowedChannels_ArchivesBeforeCompaction(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	var msgs []*domain.ChannelMessage
	for i := 0; i < 50; i++ {
		msgs = append(msgs, makeMsg(111, 500, int64(1000+i),
			"line "+strings.Repeat("x", 80),
			t0.Add(time.Duration(i)*time.Second)))
	}

	store := &multiChannelStore{
		byChannel: map[int64][]*domain.ChannelMessage{111: msgs},
	}

	archiver := &fakeArchiver{}
	compactor := &fakeCompactor{summary: "summary"}

	cb := NewContextBuilder(ContextBuilderConfig{
		CurrentChannelMax:         50,
		PerMessageCharCap:         200,
		IncludeAllAllowedChannels: true,
		PerChannelLimit:           50,
		TotalCharBudget:           2000,
		CompactionKeepRecent:      5,
	})
	cb.WithAllowedChannels(&fakeAllowedLister{channels: []int64{111}})
	cb.WithCompactor(compactor)
	cb.WithEpisodeArchiver(archiver)

	event := &domain.DiscordEvent{
		GuildID:   1,
		ChannelID: 111,
		UserID:    500,
		Message: &domain.DiscordMessage{
			ID:      9999,
			Content: "trigger",
		},
	}

	if _, err := cb.Build(context.Background(), event, store, "sys"); err != nil {
		t.Fatalf("Build error: %v", err)
	}

	if archiver.calls != 1 {
		t.Fatalf("expected archiver called once, got %d", archiver.calls)
	}
	if len(archiver.messages) != 45 {
		t.Errorf("expected 45 messages archived, got %d", len(archiver.messages))
	}
	if len(archiver.lines) != 45 {
		t.Errorf("expected 45 tagged lines archived, got %d", len(archiver.lines))
	}
	if archiver.messages[0].MessageID != int64(1000) {
		t.Errorf("expected oldest message in archive batch, got %d", archiver.messages[0].MessageID)
	}
	if archiver.messages[len(archiver.messages)-1].MessageID != int64(1044) {
		t.Errorf("expected boundary message at idx 44, got %d", archiver.messages[len(archiver.messages)-1].MessageID)
	}
	if compactor.calls != 1 {
		t.Errorf("expected compactor still called, got %d", compactor.calls)
	}
}

func TestContextBuilder_AllowedChannels_NoArchiveWithoutCompaction(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	store := &multiChannelStore{
		byChannel: map[int64][]*domain.ChannelMessage{
			111: {
				makeMsg(111, 500, 1001, "hi", t0),
			},
		},
	}

	archiver := &fakeArchiver{}

	cb := NewContextBuilder(ContextBuilderConfig{
		CurrentChannelMax:         10,
		PerMessageCharCap:         200,
		IncludeAllAllowedChannels: true,
		PerChannelLimit:           20,
		TotalCharBudget:           40000,
		CompactionKeepRecent:      10,
	})
	cb.WithAllowedChannels(&fakeAllowedLister{channels: []int64{111}})
	cb.WithEpisodeArchiver(archiver)

	event := &domain.DiscordEvent{
		GuildID:   1,
		ChannelID: 111,
		Message: &domain.DiscordMessage{
			ID:      9999,
			Content: "trigger",
		},
	}

	if _, err := cb.Build(context.Background(), event, store, "sys"); err != nil {
		t.Fatalf("Build error: %v", err)
	}

	if archiver.calls != 0 {
		t.Errorf("archiver should not fire under budget, called %d times", archiver.calls)
	}
}
