package wire

import (
	"context"
	"testing"
	"time"

	"github.com/eko/iris-bot/internal/domain"
	"github.com/eko/iris-bot/internal/memory"
	"github.com/eko/iris-bot/internal/orchestrator"
)

func TestAdapterTypesCompile(t *testing.T) {
	_ = &MemoryStoreAdapter{}
	_ = &LoreStoreAdapter{}
	_ = &ExceptionChannelAdapter{}
	_ = &GuildStoreAdapter{}
	_ = &SettingsStoreAdapter{}
	_ = &LLMAdapter{}
	_ = &ImageAdapter{}
	_ = &LoreAdapter{}
	_ = &TriggerAdapter{}
	_ = &DiscordSenderAdapter{}
	_ = &CrossChannelLLMAdapter{}
	_ = &CandidateStoreAdapter{}
	_ = &ChannelAllowAdapter{}
	_ = &MemoryWriterAdapter{}
	_ = &SafetyCheckerAdapter{}
}

func TestDiscordSenderAdapter_SatisfiesTypingSender(t *testing.T) {
	var _ orchestrator.TypingSender = (*DiscordSenderAdapter)(nil)
}

func TestSettingsStoreAdapterGetNotFound(t *testing.T) {
	adapter := &SettingsStoreAdapter{
		Repo: nil,
	}
	ctx := context.Background()
	val, found, err := adapter.Get(ctx, 123, "nonexistent")
	if val != "" || found || err != nil {
		t.Fatalf("expected ('', false, nil), got (%q, %v, %v)", val, found, err)
	}
}

func TestLoreStoreAdapterSearchSimilarEmpty(t *testing.T) {
	adapter := &LoreStoreAdapter{}
	ctx := context.Background()
	chunks, err := adapter.SearchSimilar(ctx, []float32{}, 5)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(chunks) > 0 {
		t.Fatalf("expected empty chunks, got %d", len(chunks))
	}
}

func TestGuildStoreAdapterUpsertIdempotent(t *testing.T) {
	adapter := &GuildStoreAdapter{
		Repo: nil,
	}
	ctx := context.Background()
	guild := &domain.Guild{ID: 123}
	err := adapter.Upsert(ctx, guild)
	if err != nil {
		t.Fatalf("expected no error with nil repo, got %v", err)
	}
}

type FakeEmbedder struct {
	embedding []float32
	err       error
	called    bool
	lastText  string
}

func (f *FakeEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	f.called = true
	f.lastText = text
	return f.embedding, f.err
}

type FakeGateway struct {
	typingCalled bool
	typingErr    error
}

func (f *FakeGateway) SendTyping(ctx context.Context, guildID, channelID int64) error {
	f.typingCalled = true
	return f.typingErr
}

func (f *FakeGateway) SendMessage(ctx context.Context, guildID, channelID int64, content string) error {
	return nil
}

func TestChannelCaptureAdapter_DoesNotEmbedSynchronously(t *testing.T) {
	fakeEmbedder := &FakeEmbedder{embedding: make([]float32, 384)}

	adapter := &ChannelCaptureAdapter{
		Repo: nil,
	}

	msg := &domain.ChannelMessage{
		GuildID:   123,
		ChannelID: 456,
		MessageID: 789,
		Content:   "test message content",
	}

	ctx := context.Background()
	err := adapter.Capture(ctx, msg)
	if err != nil {
		t.Fatalf("Capture failed: %v", err)
	}

	if fakeEmbedder.called {
		t.Fatal("embedder should not be called during capture; async worker handles embedding")
	}

	if len(msg.ContentEmbedding) > 0 {
		t.Fatal("message embedding should remain nil; async worker will populate it later")
	}
}

func TestBehaviorProfileUpdater_CallsServiceWithSamples(t *testing.T) {
	fakeService := &FakeBehaviorProfileService{}

	adapter := &BehaviorProfileUpdater{
		Service: fakeService,
	}

	ctx := context.Background()
	err := adapter.UpdateFromMessage(ctx, 123, 456, "test message", time.Now())
	if err != nil {
		t.Fatalf("UpdateFromMessage failed: %v", err)
	}
}

func TestBehaviorProfileUpdater_SkipsWhenNilService(t *testing.T) {
	adapter := &BehaviorProfileUpdater{
		Service: nil,
	}

	ctx := context.Background()
	err := adapter.UpdateFromMessage(ctx, 123, 456, "test message", time.Now())
	if err != nil {
		t.Fatalf("UpdateFromMessage should not error with nil service: %v", err)
	}
}

type FakeBehaviorProfileService struct {
	updateCalled bool
	lastGuildID  int64
	lastUserID   int64
}

func (f *FakeBehaviorProfileService) UpdateFromSamples(ctx context.Context, guildID, userID int64, samples []interface{}) error {
	f.updateCalled = true
	f.lastGuildID = guildID
	f.lastUserID = userID
	return nil
}

type FakeContextStore struct {
	listCalled bool
}

func (f *FakeContextStore) ListByUserAcrossChannels(ctx context.Context, guildID int64, userID int64, sinceMinutes int, limit int) ([]*domain.ChannelMessage, error) {
	f.listCalled = true
	return []*domain.ChannelMessage{
		{
			GuildID:   guildID,
			UserID:    userID,
			Content:   "sample message 1",
			CreatedAt: time.Now(),
			IsBot:     false,
		},
	}, nil
}

type FakeChannelMessageRepo struct {
	upsertCalled bool
}

func (f *FakeChannelMessageRepo) Upsert(ctx context.Context, msg *domain.ChannelMessage) error {
	f.upsertCalled = true
	return nil
}

type FakeBehaviorService struct {
	updateCalls []struct {
		guildID int64
		userID  int64
		samples []memory.MessageSample
	}
}

func (f *FakeBehaviorService) UpdateFromSamples(ctx context.Context, guildID, userID int64, samples []memory.MessageSample) (*domain.UserBehaviorProfile, error) {
	f.updateCalls = append(f.updateCalls, struct {
		guildID int64
		userID  int64
		samples []memory.MessageSample
	}{guildID, userID, samples})
	return nil, nil
}

func TestBehaviorProfileUpdateAdapter_BuffersAndFlushes(t *testing.T) {
	fakeService := &FakeBehaviorService{}

	adapter := NewBehaviorProfileUpdateAdapter(fakeService, 5, 10*time.Minute)

	ctx := context.Background()

	for range 4 {
		err := adapter.UpdateFromMessage(ctx, 123, 456, "message content", time.Now())
		if err != nil {
			t.Fatalf("UpdateFromMessage failed: %v", err)
		}
	}

	if len(fakeService.updateCalls) > 0 {
		t.Fatalf("service should not be called until buffer reaches threshold, got %d calls", len(fakeService.updateCalls))
	}

	err := adapter.UpdateFromMessage(ctx, 123, 456, "message content", time.Now())
	if err != nil {
		t.Fatalf("UpdateFromMessage failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	if len(fakeService.updateCalls) != 1 {
		t.Fatalf("service should be called after buffer reaches threshold, got %d calls", len(fakeService.updateCalls))
	}

	if fakeService.updateCalls[0].guildID != 123 || fakeService.updateCalls[0].userID != 456 {
		t.Fatalf("service called with wrong guild/user: %d/%d", fakeService.updateCalls[0].guildID, fakeService.updateCalls[0].userID)
	}
}
