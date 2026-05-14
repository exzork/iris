package admin

import (
	"context"
	"testing"
)

func TestAllowedChannelAddSuccess(t *testing.T) {
	store := newFakeAllowedStore()
	audit := newFakeAuditLogger()

	handler := NewAllowedChannelHandler(store, audit)

	cmd := &CommandContext{
		GuildID: 1,
		UserID:  100,
		IsOwner: true,
	}

	response, err := handler.Handle(context.Background(), cmd, []string{"add", "999"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if response != "Channel `999` telah ditambahkan ke daftar saluran yang diizinkan." {
		t.Errorf("unexpected response: %s", response)
	}

	if len(store.addCalls) != 1 {
		t.Errorf("expected 1 add call, got %d", len(store.addCalls))
	}

	if store.addCalls[0].guildID != 1 || store.addCalls[0].channelID != 999 {
		t.Errorf("unexpected add call: %v", store.addCalls[0])
	}

	if len(audit.logs) != 1 {
		t.Errorf("expected 1 audit log, got %d", len(audit.logs))
	}

	if audit.logs[0].guildID != 1 || audit.logs[0].userID != 100 {
		t.Errorf("unexpected audit log: %v", audit.logs[0])
	}
}

func TestAllowedChannelRemoveSuccess(t *testing.T) {
	store := newFakeAllowedStore()
	audit := newFakeAuditLogger()

	handler := NewAllowedChannelHandler(store, audit)

	cmd := &CommandContext{
		GuildID: 1,
		UserID:  100,
		IsOwner: true,
	}

	response, err := handler.Handle(context.Background(), cmd, []string{"remove", "999"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if response != "Channel `999` telah dihapus dari daftar saluran yang diizinkan." {
		t.Errorf("unexpected response: %s", response)
	}

	if len(store.removeCalls) != 1 {
		t.Errorf("expected 1 remove call, got %d", len(store.removeCalls))
	}

	if len(audit.logs) != 1 {
		t.Errorf("expected 1 audit log, got %d", len(audit.logs))
	}
}

func TestAllowedChannelListSuccess(t *testing.T) {
	store := newFakeAllowedStore()
	store.channels[1] = map[int64]bool{999: true, 888: true}
	audit := newFakeAuditLogger()

	handler := NewAllowedChannelHandler(store, audit)

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

func TestAllowedChannelListEmpty(t *testing.T) {
	store := newFakeAllowedStore()
	audit := newFakeAuditLogger()

	handler := NewAllowedChannelHandler(store, audit)

	cmd := &CommandContext{
		GuildID: 1,
		UserID:  100,
		IsOwner: true,
	}

	response, err := handler.Handle(context.Background(), cmd, []string{"list"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if response != "Tidak ada saluran yang diizinkan untuk guild ini." {
		t.Errorf("unexpected response: %s", response)
	}
}

func TestAllowedChannelAddInvalidArgs(t *testing.T) {
	store := newFakeAllowedStore()
	audit := newFakeAuditLogger()

	handler := NewAllowedChannelHandler(store, audit)

	cmd := &CommandContext{
		GuildID: 1,
		UserID:  100,
		IsOwner: true,
	}

	response, err := handler.Handle(context.Background(), cmd, []string{"add"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if response != "Format perintah salah. Gunakan `!iris help` untuk melihat daftar perintah." {
		t.Errorf("unexpected response: %s", response)
	}

	if len(store.addCalls) != 0 {
		t.Errorf("expected no add calls, got %d", len(store.addCalls))
	}
}

func TestAllowedChannelAddInvalidChannelID(t *testing.T) {
	store := newFakeAllowedStore()
	audit := newFakeAuditLogger()

	handler := NewAllowedChannelHandler(store, audit)

	cmd := &CommandContext{
		GuildID: 1,
		UserID:  100,
		IsOwner: true,
	}

	response, err := handler.Handle(context.Background(), cmd, []string{"add", "not-a-number"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if response != "Format perintah salah. Gunakan `!iris help` untuk melihat daftar perintah." {
		t.Errorf("unexpected response: %s", response)
	}

	if len(store.addCalls) != 0 {
		t.Errorf("expected no add calls, got %d", len(store.addCalls))
	}
}
