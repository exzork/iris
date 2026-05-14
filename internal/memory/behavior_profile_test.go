package memory

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eko/iris-bot/internal/domain"
)

type fakeProfileStore struct {
	profiles map[string]*domain.UserBehaviorProfile
	getErr   error
	upsertErr error
	upserts  int
}

func newFakeProfileStore() *fakeProfileStore {
	return &fakeProfileStore{profiles: map[string]*domain.UserBehaviorProfile{}}
}

func profileKey(guildID, userID int64) string {
	return string(rune(guildID)) + "/" + string(rune(userID))
}

func (s *fakeProfileStore) GetByGuildUser(ctx context.Context, guildID int64, userID int64) (*domain.UserBehaviorProfile, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	if p, ok := s.profiles[profileKey(guildID, userID)]; ok {
		return p, nil
	}
	return nil, nil
}

func (s *fakeProfileStore) Upsert(ctx context.Context, p *domain.UserBehaviorProfile) error {
	s.upserts++
	if s.upsertErr != nil {
		return s.upsertErr
	}
	copyP := *p
	s.profiles[profileKey(p.GuildID, p.UserID)] = &copyP
	return nil
}

func sampleText(t string) MessageSample {
	return MessageSample{Content: t, CreatedAt: time.Now(), IsBot: false}
}

func TestBehaviorProfileService_RejectsMissingGuildOrUser(t *testing.T) {
	svc, err := NewBehaviorProfileService(newFakeProfileStore())
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if _, err := svc.UpdateFromSamples(context.Background(), 0, 1, []MessageSample{sampleText("hi")}); !errors.Is(err, ErrMissingGuildID) {
		t.Fatalf("want ErrMissingGuildID, got %v", err)
	}
	if _, err := svc.UpdateFromSamples(context.Background(), 1, 0, []MessageSample{sampleText("hi")}); !errors.Is(err, ErrMissingUserID) {
		t.Fatalf("want ErrMissingUserID, got %v", err)
	}
	if _, err := svc.Get(context.Background(), 0, 1); !errors.Is(err, ErrMissingGuildID) {
		t.Fatalf("get guild err: %v", err)
	}
	if _, err := svc.Get(context.Background(), 1, 0); !errors.Is(err, ErrMissingUserID) {
		t.Fatalf("get user err: %v", err)
	}
}

func TestBehaviorProfileService_ProducesNonSensitiveHints(t *testing.T) {
	store := newFakeProfileStore()
	svc, err := NewBehaviorProfileService(store)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	profile, err := svc.UpdateFromSamples(context.Background(), 42, 7, []MessageSample{
		sampleText("halo lore lore characters 🙂"),
		sampleText("lore characters strategy"),
		sampleText("lore lore characters!"),
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if profile == nil {
		t.Fatalf("expected profile, got nil")
	}
	if profile.GuildID != 42 || profile.UserID != 7 {
		t.Fatalf("guild/user not propagated: %+v", profile)
	}
	if profile.EvidenceCount != 3 {
		t.Fatalf("evidence count: %d", profile.EvidenceCount)
	}
	foundLore := false
	for _, tk := range profile.RecurringTopics {
		if tk == "lore" {
			foundLore = true
		}
	}
	if !foundLore {
		t.Fatalf("expected 'lore' in recurring topics: %v", profile.RecurringTopics)
	}
}

func TestBehaviorProfileService_DropsSensitiveSamples(t *testing.T) {
	store := newFakeProfileStore()
	svc, _ := NewBehaviorProfileService(store)

	profile, err := svc.UpdateFromSamples(context.Background(), 1, 1, []MessageSample{
		{Content: "My api_key is abc123", CreatedAt: time.Now()},
		{Content: "I was diagnosed with X", CreatedAt: time.Now()},
		{Content: "political opinions here", CreatedAt: time.Now()},
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if profile != nil {
		t.Fatalf("expected nil profile when all samples sensitive, got %+v", profile)
	}
}

func TestBehaviorProfileService_IgnoresBotMessagesAndEmpty(t *testing.T) {
	store := newFakeProfileStore()
	svc, _ := NewBehaviorProfileService(store)
	profile, _ := svc.UpdateFromSamples(context.Background(), 1, 2, []MessageSample{
		{Content: "hi there lore", CreatedAt: time.Now(), IsBot: false},
		{Content: "", CreatedAt: time.Now(), IsBot: false},
		{Content: "ignored bot message", CreatedAt: time.Now(), IsBot: true},
	})
	if profile == nil {
		t.Fatalf("expected profile from single non-bot sample, got nil")
	}
	if profile.EvidenceCount != 1 {
		t.Fatalf("expected 1 evidence after filtering bots/empty, got %d", profile.EvidenceCount)
	}
}

func TestBehaviorProfileService_ProfileIsGuildUserIsolated(t *testing.T) {
	store := newFakeProfileStore()
	svc, _ := NewBehaviorProfileService(store)

	if _, err := svc.UpdateFromSamples(context.Background(), 100, 1, []MessageSample{sampleText("hi hi lore lore")}); err != nil {
		t.Fatalf("update guild 100 user 1: %v", err)
	}
	if _, err := svc.UpdateFromSamples(context.Background(), 200, 1, []MessageSample{sampleText("halo strategy strategy")}); err != nil {
		t.Fatalf("update guild 200 user 1: %v", err)
	}
	if _, err := svc.UpdateFromSamples(context.Background(), 100, 2, []MessageSample{sampleText("hey map map")}); err != nil {
		t.Fatalf("update guild 100 user 2: %v", err)
	}

	got100_1, _ := svc.Get(context.Background(), 100, 1)
	got200_1, _ := svc.Get(context.Background(), 200, 1)
	got100_2, _ := svc.Get(context.Background(), 100, 2)

	if got100_1 == nil || got200_1 == nil || got100_2 == nil {
		t.Fatalf("missing profile: %+v %+v %+v", got100_1, got200_1, got100_2)
	}
	if got100_1.UserID == got100_2.UserID {
		t.Fatalf("same-guild different users should have different user IDs")
	}
	if got100_1.GuildID == got200_1.GuildID {
		t.Fatalf("same user across guilds should yield different guild IDs")
	}
}
