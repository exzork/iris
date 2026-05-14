package admin

import (
	"context"
	"testing"
)

func TestExceptionAddSuccess(t *testing.T) {
	store := newFakeExceptionStore()
	audit := newFakeAuditLogger()

	handler := NewExceptionHandler(store, audit)

	cmd := &CommandContext{
		GuildID: 1,
		UserID:  100,
		IsOwner: true,
	}

	response, err := handler.Handle(context.Background(), cmd, []string{"add", "999"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if response != "Channel `999` telah ditambahkan ke daftar pengecualian." {
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

func TestExceptionRemoveSuccess(t *testing.T) {
	store := newFakeExceptionStore()
	audit := newFakeAuditLogger()

	handler := NewExceptionHandler(store, audit)

	cmd := &CommandContext{
		GuildID: 1,
		UserID:  100,
		IsOwner: true,
	}

	response, err := handler.Handle(context.Background(), cmd, []string{"remove", "999"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if response != "Channel `999` telah dihapus dari daftar pengecualian." {
		t.Errorf("unexpected response: %s", response)
	}

	if len(store.removeCalls) != 1 {
		t.Errorf("expected 1 remove call, got %d", len(store.removeCalls))
	}

	if len(audit.logs) != 1 {
		t.Errorf("expected 1 audit log, got %d", len(audit.logs))
	}
}

func TestExceptionListSuccess(t *testing.T) {
	store := newFakeExceptionStore()
	store.channels[1] = map[int64]bool{999: true, 888: true}
	audit := newFakeAuditLogger()

	handler := NewExceptionHandler(store, audit)

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

func TestExceptionAddInvalidArgs(t *testing.T) {
	store := newFakeExceptionStore()
	audit := newFakeAuditLogger()

	handler := NewExceptionHandler(store, audit)

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

func TestExceptionGuildScoping(t *testing.T) {
	store := newFakeExceptionStore()
	audit := newFakeAuditLogger()

	handler := NewExceptionHandler(store, audit)

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

	handler.Handle(context.Background(), cmd1, []string{"add", "999"})
	handler.Handle(context.Background(), cmd2, []string{"add", "888"})

	if len(store.channels[1]) != 1 || !store.channels[1][999] {
		t.Error("guild 1 should have channel 999")
	}

	if len(store.channels[2]) != 1 || !store.channels[2][888] {
		t.Error("guild 2 should have channel 888")
	}
}
