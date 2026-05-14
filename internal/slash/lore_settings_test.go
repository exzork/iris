package slash

import (
	"context"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/eko/iris-bot/internal/domain"
)

type mockLoreSettingsRepo struct {
	enabled bool
	cap     int
	err     error
}

func (m *mockLoreSettingsRepo) IsEnabled(ctx context.Context, guildID int64) (bool, error) {
	return m.enabled, m.err
}

func (m *mockLoreSettingsRepo) SetEnabled(ctx context.Context, guildID int64, enabled bool) error {
	if m.err != nil {
		return m.err
	}
	m.enabled = enabled
	return nil
}

func (m *mockLoreSettingsRepo) GetSettings(ctx context.Context, guildID int64) (*domain.LoreGuildSettings, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &domain.LoreGuildSettings{
		GuildID:          guildID,
		Enabled:          m.enabled,
		ThreadCapPerHour: m.cap,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}, nil
}

func (m *mockLoreSettingsRepo) SetThreadCapPerHour(ctx context.Context, guildID int64, cap int) error {
	if m.err != nil {
		return m.err
	}
	m.cap = cap
	return nil
}

func TestLoreSettingsEnable(t *testing.T) {
	repo := &mockLoreSettingsRepo{enabled: false, cap: 10}
	handler := NewLoreSettingsHandler(repo)
	cmd := newLoreSettingsCommand(handler)

	inv := &NativeInvocation{
		GuildID:   123,
		ChannelID: 456,
		UserID:    789,
		IsAdmin:   true,
		Options: []*discordgo.ApplicationCommandInteractionDataOption{
			{Name: "enable", Type: discordgo.ApplicationCommandOptionSubCommand},
		},
	}

	result, err := cmd.Execute(context.Background(), inv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !repo.enabled {
		t.Error("expected lore threads to be enabled")
	}

	if result != "✅ Lore threads diaktifkan untuk guild ini." {
		t.Errorf("unexpected result: %s", result)
	}
}

func TestLoreSettingsDisable(t *testing.T) {
	repo := &mockLoreSettingsRepo{enabled: true, cap: 10}
	handler := NewLoreSettingsHandler(repo)
	cmd := newLoreSettingsCommand(handler)

	inv := &NativeInvocation{
		GuildID:   123,
		ChannelID: 456,
		UserID:    789,
		IsAdmin:   true,
		Options: []*discordgo.ApplicationCommandInteractionDataOption{
			{Name: "disable", Type: discordgo.ApplicationCommandOptionSubCommand},
		},
	}

	result, err := cmd.Execute(context.Background(), inv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if repo.enabled {
		t.Error("expected lore threads to be disabled")
	}

	if result != "✅ Lore threads dinonaktifkan untuk guild ini." {
		t.Errorf("unexpected result: %s", result)
	}
}

func TestLoreSettingsNonAdminRejected(t *testing.T) {
	repo := &mockLoreSettingsRepo{enabled: false, cap: 10}
	handler := NewLoreSettingsHandler(repo)
	cmd := newLoreSettingsCommand(handler)

	inv := &NativeInvocation{
		GuildID:   123,
		ChannelID: 456,
		UserID:    789,
		IsAdmin:   false,
		Options: []*discordgo.ApplicationCommandInteractionDataOption{
			{Name: "enable", Type: discordgo.ApplicationCommandOptionSubCommand},
		},
	}

	result, err := cmd.Execute(context.Background(), inv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "Mohon maaf, hanya admin server yang dapat mengubah pengaturan lore threads." {
		t.Errorf("unexpected result: %s", result)
	}
}

func TestLoreSettingsStatus(t *testing.T) {
	repo := &mockLoreSettingsRepo{enabled: true, cap: 25}
	handler := NewLoreSettingsHandler(repo)
	cmd := newLoreSettingsCommand(handler)

	inv := &NativeInvocation{
		GuildID:   123,
		ChannelID: 456,
		UserID:    789,
		IsAdmin:   true,
		Options: []*discordgo.ApplicationCommandInteractionDataOption{
			{Name: "status", Type: discordgo.ApplicationCommandOptionSubCommand},
		},
	}

	result, err := cmd.Execute(context.Background(), inv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == "" {
		t.Error("expected non-empty status result")
	}

	if !contains(result, "Status Lore Threads") {
		t.Errorf("expected 'Status Lore Threads' in result: %s", result)
	}

	if !contains(result, "✅ Diaktifkan") {
		t.Errorf("expected enabled status in result: %s", result)
	}

	if !contains(result, "25") {
		t.Errorf("expected thread cap 25 in result: %s", result)
	}
}

func TestLoreSettingsCap(t *testing.T) {
	repo := &mockLoreSettingsRepo{enabled: true, cap: 10}
	handler := NewLoreSettingsHandler(repo)
	cmd := newLoreSettingsCommand(handler)

	inv := &NativeInvocation{
		GuildID:   123,
		ChannelID: 456,
		UserID:    789,
		IsAdmin:   true,
		Options: []*discordgo.ApplicationCommandInteractionDataOption{
			{
				Name: "cap",
				Type: discordgo.ApplicationCommandOptionSubCommand,
				Options: []*discordgo.ApplicationCommandInteractionDataOption{
					{
						Name:  "value",
						Type:  discordgo.ApplicationCommandOptionInteger,
						Value: float64(50),
					},
				},
			},
		},
	}

	result, err := cmd.Execute(context.Background(), inv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if repo.cap != 50 {
		t.Errorf("expected cap to be 50, got %d", repo.cap)
	}

	if !contains(result, "50") {
		t.Errorf("expected '50' in result: %s", result)
	}
}

func TestLoreSettingsCapOutOfRange(t *testing.T) {
	repo := &mockLoreSettingsRepo{enabled: true, cap: 10}
	handler := NewLoreSettingsHandler(repo)
	cmd := newLoreSettingsCommand(handler)

	inv := &NativeInvocation{
		GuildID:   123,
		ChannelID: 456,
		UserID:    789,
		IsAdmin:   true,
		Options: []*discordgo.ApplicationCommandInteractionDataOption{
			{
				Name: "cap",
				Type: discordgo.ApplicationCommandOptionSubCommand,
				Options: []*discordgo.ApplicationCommandInteractionDataOption{
					{
						Name:  "value",
						Type:  discordgo.ApplicationCommandOptionInteger,
						Value: float64(150),
					},
				},
			},
		},
	}

	result, err := cmd.Execute(context.Background(), inv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !contains(result, "antara 1 dan 100") {
		t.Errorf("expected range error in result: %s", result)
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
