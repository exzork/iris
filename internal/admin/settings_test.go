package admin

import (
	"context"
	"testing"
)

func TestSettingsSetSuccess(t *testing.T) {
	store := newFakeSettingsStore()
	audit := newFakeAuditLogger()

	handler := NewSettingsHandler(store, audit)

	cmd := &CommandContext{
		GuildID: 1,
		UserID:  100,
		IsOwner: true,
	}

	response, err := handler.Handle(context.Background(), cmd, []string{"set", "default_locale", "id"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if response != "Konfigurasi `default_locale` telah diperbarui." {
		t.Errorf("unexpected response: %s", response)
	}

	if len(store.setCalls) != 1 {
		t.Errorf("expected 1 set call, got %d", len(store.setCalls))
	}

	if store.setCalls[0].guildID != 1 || store.setCalls[0].key != "default_locale" || store.setCalls[0].value != "id" {
		t.Errorf("unexpected set call: %v", store.setCalls[0])
	}

	if len(audit.logs) != 1 {
		t.Errorf("expected 1 audit log, got %d", len(audit.logs))
	}
}

func TestSettingsSetUnknownKey(t *testing.T) {
	store := newFakeSettingsStore()
	audit := newFakeAuditLogger()

	handler := NewSettingsHandler(store, audit)

	cmd := &CommandContext{
		GuildID: 1,
		UserID:  100,
		IsOwner: true,
	}

	response, err := handler.Handle(context.Background(), cmd, []string{"set", "unknown_key", "value"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if response != "Kunci konfigurasi tidak dikenali: `unknown_key`. Lihat `!iris help`." {
		t.Errorf("unexpected response: %s", response)
	}

	if len(store.setCalls) != 0 {
		t.Errorf("expected no set calls, got %d", len(store.setCalls))
	}

	if len(audit.logs) != 0 {
		t.Errorf("expected no audit logs, got %d", len(audit.logs))
	}
}

func TestSettingsGetSuccess(t *testing.T) {
	store := newFakeSettingsStore()
	store.settings[1] = map[string]string{"default_locale": "id"}
	audit := newFakeAuditLogger()

	handler := NewSettingsHandler(store, audit)

	cmd := &CommandContext{
		GuildID: 1,
		UserID:  100,
		IsOwner: true,
	}

	response, err := handler.Handle(context.Background(), cmd, []string{"get", "default_locale"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if response != "default_locale: id" {
		t.Errorf("unexpected response: %s", response)
	}

	if len(audit.logs) != 0 {
		t.Errorf("expected no audit logs for get, got %d", len(audit.logs))
	}
}

func TestSettingsGetNotFound(t *testing.T) {
	store := newFakeSettingsStore()
	audit := newFakeAuditLogger()

	handler := NewSettingsHandler(store, audit)

	cmd := &CommandContext{
		GuildID: 1,
		UserID:  100,
		IsOwner: true,
	}

	response, err := handler.Handle(context.Background(), cmd, []string{"get", "default_locale"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if response != "Konfigurasi tidak ditemukan: `default_locale`." {
		t.Errorf("unexpected response: %s", response)
	}
}

func TestSettingsListSuccess(t *testing.T) {
	store := newFakeSettingsStore()
	store.settings[1] = map[string]string{
		"default_locale": "id",
		"memory_enabled": "true",
	}
	audit := newFakeAuditLogger()

	handler := NewSettingsHandler(store, audit)

	cmd := &CommandContext{
		GuildID: 1,
		UserID:  100,
		IsOwner: true,
	}

	response, err := handler.Handle(context.Background(), cmd, []string{"list"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if response == "" {
		t.Error("expected non-empty response")
	}

	if len(audit.logs) != 0 {
		t.Errorf("expected no audit logs for list, got %d", len(audit.logs))
	}
}

func TestRatelimitSetSuccess(t *testing.T) {
	store := newFakeSettingsStore()
	audit := newFakeAuditLogger()

	handler := NewRatelimitHandler(store, audit)

	cmd := &CommandContext{
		GuildID: 1,
		UserID:  100,
		IsOwner: true,
	}

	response, err := handler.Handle(context.Background(), cmd, []string{"set", "user", "10"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if response != "Rate limit untuk `user` telah diperbarui menjadi `10` permintaan per menit." {
		t.Errorf("unexpected response: %s", response)
	}

	if len(store.setCalls) != 1 {
		t.Errorf("expected 1 set call, got %d", len(store.setCalls))
	}

	if store.setCalls[0].key != "ratelimit_user_per_min" {
		t.Errorf("unexpected key: %s", store.setCalls[0].key)
	}

	if len(audit.logs) != 1 {
		t.Errorf("expected 1 audit log, got %d", len(audit.logs))
	}
}

func TestRatelimitSetInvalidScope(t *testing.T) {
	store := newFakeSettingsStore()
	audit := newFakeAuditLogger()

	handler := NewRatelimitHandler(store, audit)

	cmd := &CommandContext{
		GuildID: 1,
		UserID:  100,
		IsOwner: true,
	}

	response, err := handler.Handle(context.Background(), cmd, []string{"set", "invalid", "10"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if response != "Scope tidak valid. Gunakan `user` atau `guild`." {
		t.Errorf("unexpected response: %s", response)
	}

	if len(store.setCalls) != 0 {
		t.Errorf("expected no set calls, got %d", len(store.setCalls))
	}
}

func TestRatelimitGetSuccess(t *testing.T) {
	store := newFakeSettingsStore()
	store.settings[1] = map[string]string{
		"ratelimit_user_per_min": "10",
		"ratelimit_guild_per_min": "20",
	}
	audit := newFakeAuditLogger()

	handler := NewRatelimitHandler(store, audit)

	cmd := &CommandContext{
		GuildID: 1,
		UserID:  100,
		IsOwner: true,
	}

	response, err := handler.Handle(context.Background(), cmd, []string{"get"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if response == "" {
		t.Error("expected non-empty response")
	}

	if len(audit.logs) != 0 {
		t.Errorf("expected no audit logs for get, got %d", len(audit.logs))
	}
}

func TestSettingsGuildScoping(t *testing.T) {
	store := newFakeSettingsStore()
	audit := newFakeAuditLogger()

	handler := NewSettingsHandler(store, audit)

	cmd1 := &CommandContext{
		GuildID: 1,
		UserID:  100,
		IsOwner: true,
	}

	cmd2 := &CommandContext{
		GuildID: 2,
		UserID:  100,
		IsOwner: true,
	}

	handler.Handle(context.Background(), cmd1, []string{"set", "default_locale", "id"})
	handler.Handle(context.Background(), cmd2, []string{"set", "default_locale", "en"})

	if store.settings[1]["default_locale"] != "id" {
		t.Error("guild 1 should have locale id")
	}

	if store.settings[2]["default_locale"] != "en" {
		t.Error("guild 2 should have locale en")
	}
}
