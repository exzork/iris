package settings

import (
	"context"
	"testing"
)

type fakeSettingsRepo struct {
	data map[int64]map[string]string
}

func newFakeSettingsRepo() *fakeSettingsRepo {
	return &fakeSettingsRepo{
		data: make(map[int64]map[string]string),
	}
}

func (f *fakeSettingsRepo) Get(ctx context.Context, guildID int64, key string) (value string, found bool, err error) {
	if guildSettings, ok := f.data[guildID]; ok {
		if val, ok := guildSettings[key]; ok {
			return val, true, nil
		}
	}
	return "", false, nil
}

func (f *fakeSettingsRepo) Set(ctx context.Context, guildID int64, key, value string) error {
	if _, ok := f.data[guildID]; !ok {
		f.data[guildID] = make(map[string]string)
	}
	f.data[guildID][key] = value
	return nil
}

func (f *fakeSettingsRepo) List(ctx context.Context, guildID int64) (map[string]string, error) {
	if guildSettings, ok := f.data[guildID]; ok {
		result := make(map[string]string)
		for k, v := range guildSettings {
			result[k] = v
		}
		return result, nil
	}
	return make(map[string]string), nil
}

func TestGuildOverrideBeatsDefault(t *testing.T) {
	repo := newFakeSettingsRepo()
	resolver := &Resolver{Repo: repo}
	ctx := context.Background()

	guildID := int64(1)
	repo.Set(ctx, guildID, string(KeyImageCooldownSec), "120")

	val, err := resolver.Effective(ctx, guildID, KeyImageCooldownSec)
	if err != nil {
		t.Fatalf("Effective() error = %v", err)
	}

	if val != "120" {
		t.Errorf("Effective() = %q, want 120", val)
	}

	cooldown, err := resolver.GetInt(ctx, guildID, KeyImageCooldownSec, 0)
	if err != nil {
		t.Fatalf("GetInt() error = %v", err)
	}
	if cooldown != 120 {
		t.Errorf("GetInt() = %d, want 120", cooldown)
	}
}

func TestGuildIsolation(t *testing.T) {
	repo := newFakeSettingsRepo()
	resolver := &Resolver{Repo: repo}
	ctx := context.Background()

	guildA := int64(1)
	guildB := int64(2)

	repo.Set(ctx, guildA, string(KeyMemesEnabled), "false")
	repo.Set(ctx, guildB, string(KeyMemesEnabled), "true")

	memesA, err := resolver.GetBool(ctx, guildA, KeyMemesEnabled, true)
	if err != nil {
		t.Fatalf("GetBool(guildA) error = %v", err)
	}
	if memesA != false {
		t.Errorf("GetBool(guildA) = %v, want false", memesA)
	}

	memesB, err := resolver.GetBool(ctx, guildB, KeyMemesEnabled, true)
	if err != nil {
		t.Fatalf("GetBool(guildB) error = %v", err)
	}
	if memesB != true {
		t.Errorf("GetBool(guildB) = %v, want true", memesB)
	}
}

func TestEffectiveFallsBackToDefault(t *testing.T) {
	repo := newFakeSettingsRepo()
	resolver := &Resolver{Repo: repo}
	ctx := context.Background()

	guildID := int64(1)

	val, err := resolver.Effective(ctx, guildID, KeyDefaultLocale)
	if err != nil {
		t.Fatalf("Effective() error = %v", err)
	}

	if val != "id-ID" {
		t.Errorf("Effective() = %q, want id-ID", val)
	}
}

func TestGetIntParseErrorReturnsFallback(t *testing.T) {
	repo := newFakeSettingsRepo()
	resolver := &Resolver{Repo: repo}
	ctx := context.Background()

	guildID := int64(1)
	repo.Set(ctx, guildID, string(KeyMaxResponseChars), "not_a_number")

	val, err := resolver.GetInt(ctx, guildID, KeyMaxResponseChars, 999)
	if err != nil {
		t.Fatalf("GetInt() error = %v", err)
	}

	if val != 999 {
		t.Errorf("GetInt() = %d, want 999 (fallback)", val)
	}
}
