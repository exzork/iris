package bootstrap

import (
	"context"
	"maps"
	"testing"

	"github.com/eko/iris-bot/internal/domain"
	"github.com/eko/iris-bot/internal/settings"
)

// fakeGuildStore implements GuildStore for testing.
type fakeGuildStore struct {
	guilds map[int64]*domain.Guild
}

func (f *fakeGuildStore) Upsert(ctx context.Context, g *domain.Guild) error {
	f.guilds[g.ID] = g
	return nil
}

func (f *fakeGuildStore) Get(ctx context.Context, id int64) (*domain.Guild, error) {
	g, ok := f.guilds[id]
	if !ok {
		return nil, nil
	}
	return g, nil
}

// fakeSettingsStore implements SettingsStore for testing.
type fakeSettingsStore struct {
	settings map[int64]map[string]string
}

func (f *fakeSettingsStore) Get(ctx context.Context, guildID int64, key string) (string, bool, error) {
	if _, ok := f.settings[guildID]; !ok {
		return "", false, nil
	}
	val, found := f.settings[guildID][key]
	return val, found, nil
}

func (f *fakeSettingsStore) Set(ctx context.Context, guildID int64, key, value string) error {
	if _, ok := f.settings[guildID]; !ok {
		f.settings[guildID] = make(map[string]string)
	}
	f.settings[guildID][key] = value
	return nil
}

func (f *fakeSettingsStore) List(ctx context.Context, guildID int64) (map[string]string, error) {
	if _, ok := f.settings[guildID]; !ok {
		return make(map[string]string), nil
	}
	result := make(map[string]string)
	maps.Copy(result, f.settings[guildID])
	return result, nil
}

// TestSeedCreatesGuildAndDefaults verifies that Seed creates guild and default settings.
func TestSeedCreatesGuildAndDefaults(t *testing.T) {
	ctx := context.Background()
	guildStore := &fakeGuildStore{guilds: make(map[int64]*domain.Guild)}
	settingsStore := &fakeSettingsStore{settings: make(map[int64]map[string]string)}

	bootstrapper := &Bootstrapper{
		Guilds:   guildStore,
		Settings: settingsStore,
	}

	result, err := bootstrapper.Seed(ctx, 42, "100,200")
	if err != nil {
		t.Fatalf("Seed failed: %v", err)
	}

	// Verify guild was created
	if !result.GuildCreated {
		t.Error("expected GuildCreated=true")
	}

	// Verify guild exists in store
	guild, err := guildStore.Get(ctx, 42)
	if err != nil {
		t.Fatalf("Get guild failed: %v", err)
	}
	if guild == nil {
		t.Error("expected guild to exist")
	}
	if guild.ID != 42 {
		t.Errorf("expected guild ID 42, got %d", guild.ID)
	}

	// Verify all default settings were added
	expectedDefaults := []settings.Key{
		settings.KeyDefaultLocale,
		settings.KeyMemoryEnabled,
		settings.KeyLoreCitationsRequired,
		settings.KeyMaxResponseChars,
		settings.KeyImageCooldownSec,
		settings.KeyMemesEnabled,
		settings.KeyRemindersEnabled,
		settings.KeyRateLimitUserPerMin,
		settings.KeyRateLimitGuildPerMin,
	}

	if len(result.SettingsAdded) != len(expectedDefaults) {
		t.Errorf("expected %d settings added, got %d", len(expectedDefaults), len(result.SettingsAdded))
	}

	// Verify admin role IDs were seeded
	if result.AdminsSeeded != 2 {
		t.Errorf("expected AdminsSeeded=2, got %d", result.AdminsSeeded)
	}

	// Verify idempotent flag
	if result.Idempotent {
		t.Error("expected Idempotent=false on first run")
	}

	// Verify settings values
	settingsList, err := settingsStore.List(ctx, 42)
	if err != nil {
		t.Fatalf("List settings failed: %v", err)
	}

	expectedValues := map[string]string{
		string(settings.KeyMemoryEnabled):         "true",
		string(settings.KeyImageCooldownSec):      "60",
		string(settings.KeyAdminRoleIDs):          "100,200",
		string(settings.KeyDefaultLocale):         "id-ID",
		string(settings.KeyLoreCitationsRequired): "true",
		string(settings.KeyMaxResponseChars):      "2000",
		string(settings.KeyMemesEnabled):          "true",
		string(settings.KeyRemindersEnabled):      "true",
		string(settings.KeyRateLimitUserPerMin):   "30",
		string(settings.KeyRateLimitGuildPerMin):  "120",
	}

	for key, expectedVal := range expectedValues {
		actualVal, ok := settingsList[key]
		if !ok {
			t.Errorf("expected setting %s to be set", key)
			continue
		}
		if actualVal != expectedVal {
			t.Errorf("setting %s: expected %q, got %q", key, expectedVal, actualVal)
		}
	}
}

// TestSeedIdempotent verifies that running Seed twice is idempotent.
func TestSeedIdempotent(t *testing.T) {
	ctx := context.Background()
	guildStore := &fakeGuildStore{guilds: make(map[int64]*domain.Guild)}
	settingsStore := &fakeSettingsStore{settings: make(map[int64]map[string]string)}

	bootstrapper := &Bootstrapper{
		Guilds:   guildStore,
		Settings: settingsStore,
	}

	// First run
	result1, err := bootstrapper.Seed(ctx, 42, "100,200")
	if err != nil {
		t.Fatalf("First Seed failed: %v", err)
	}

	if result1.GuildCreated != true {
		t.Error("first run: expected GuildCreated=true")
	}
	if result1.Idempotent != false {
		t.Error("first run: expected Idempotent=false")
	}

	// Second run
	result2, err := bootstrapper.Seed(ctx, 42, "100,200")
	if err != nil {
		t.Fatalf("Second Seed failed: %v", err)
	}

	if result2.GuildCreated {
		t.Error("second run: expected GuildCreated=false")
	}
	if len(result2.SettingsAdded) != 0 {
		t.Errorf("second run: expected SettingsAdded=[], got %v", result2.SettingsAdded)
	}
	if result2.AdminsSeeded != 0 {
		t.Errorf("second run: expected AdminsSeeded=0, got %d", result2.AdminsSeeded)
	}
	if !result2.Idempotent {
		t.Error("second run: expected Idempotent=true")
	}

	// Verify only one guild row exists
	if len(guildStore.guilds) != 1 {
		t.Errorf("expected 1 guild row, got %d", len(guildStore.guilds))
	}
}

// TestSeedPreservesExistingOverrides verifies that existing settings are not overwritten.
func TestSeedPreservesExistingOverrides(t *testing.T) {
	ctx := context.Background()
	guildStore := &fakeGuildStore{guilds: make(map[int64]*domain.Guild)}
	settingsStore := &fakeSettingsStore{settings: make(map[int64]map[string]string)}

	// Pre-set memory_enabled to false
	settingsStore.Set(ctx, 42, string(settings.KeyMemoryEnabled), "false")

	bootstrapper := &Bootstrapper{
		Guilds:   guildStore,
		Settings: settingsStore,
	}

	result, err := bootstrapper.Seed(ctx, 42, "")
	if err != nil {
		t.Fatalf("Seed failed: %v", err)
	}

	// Verify memory_enabled was not overwritten
	val, found, err := settingsStore.Get(ctx, 42, string(settings.KeyMemoryEnabled))
	if err != nil {
		t.Fatalf("Get setting failed: %v", err)
	}
	if !found {
		t.Error("expected memory_enabled to exist")
	}
	if val != "false" {
		t.Errorf("expected memory_enabled=false, got %q", val)
	}

	// Verify memory_enabled is not in SettingsAdded
	for _, key := range result.SettingsAdded {
		if key == settings.KeyMemoryEnabled {
			t.Error("expected KeyMemoryEnabled to not be in SettingsAdded")
		}
	}
}

// TestSeedEmptyAdminRoleIDsSkipsAdmins verifies that empty adminRoleIDs skips admin seeding.
func TestSeedEmptyAdminRoleIDsSkipsAdmins(t *testing.T) {
	ctx := context.Background()
	guildStore := &fakeGuildStore{guilds: make(map[int64]*domain.Guild)}
	settingsStore := &fakeSettingsStore{settings: make(map[int64]map[string]string)}

	bootstrapper := &Bootstrapper{
		Guilds:   guildStore,
		Settings: settingsStore,
	}

	result, err := bootstrapper.Seed(ctx, 42, "")
	if err != nil {
		t.Fatalf("Seed failed: %v", err)
	}

	if result.AdminsSeeded != 0 {
		t.Errorf("expected AdminsSeeded=0, got %d", result.AdminsSeeded)
	}

	// Verify admin_role_ids is not set
	val, found, err := settingsStore.Get(ctx, 42, string(settings.KeyAdminRoleIDs))
	if err != nil {
		t.Fatalf("Get setting failed: %v", err)
	}
	if found {
		t.Errorf("expected admin_role_ids to not be set, got %q", val)
	}
}

func TestMigrationsIdempotent(t *testing.T) {
	ctx := context.Background()
	guildStore := &fakeGuildStore{guilds: make(map[int64]*domain.Guild)}
	settingsStore := &fakeSettingsStore{settings: make(map[int64]map[string]string)}

	bootstrapper := &Bootstrapper{
		Guilds:   guildStore,
		Settings: settingsStore,
	}

	result1, err := bootstrapper.Seed(ctx, 42, "100")
	if err != nil {
		t.Fatalf("First Seed failed: %v", err)
	}

	result2, err := bootstrapper.Seed(ctx, 42, "100")
	if err != nil {
		t.Fatalf("Second Seed failed: %v", err)
	}

	if !result2.Idempotent {
		t.Error("expected second Seed to be idempotent")
	}

	if result1.GuildCreated != !result2.GuildCreated {
		t.Error("expected GuildCreated to differ between runs")
	}

	if len(result2.SettingsAdded) != 0 {
		t.Errorf("expected no settings added on second run, got %d", len(result2.SettingsAdded))
	}
}
