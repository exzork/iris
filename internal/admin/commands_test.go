package admin

import (
	"context"
	"testing"
)

func TestParseCommandBasic(t *testing.T) {
	parsed := parseCommand("!iris exception add 999")
	if parsed == nil {
		t.Fatal("expected parsed command")
	}
	if parsed.Verb != "exception" {
		t.Errorf("expected verb 'exception', got '%s'", parsed.Verb)
	}
	if parsed.Sub != "add" {
		t.Errorf("expected sub 'add', got '%s'", parsed.Sub)
	}
	if len(parsed.Args) != 1 || parsed.Args[0] != "999" {
		t.Errorf("expected args ['999'], got %v", parsed.Args)
	}
}

func TestParseCommandWithQuotes(t *testing.T) {
	parsed := parseCommand(`!iris config set key "value with spaces"`)
	if parsed == nil {
		t.Fatal("expected parsed command")
	}
	if parsed.Verb != "config" {
		t.Errorf("expected verb 'config', got '%s'", parsed.Verb)
	}
	if parsed.Sub != "set" {
		t.Errorf("expected sub 'set', got '%s'", parsed.Sub)
	}
	if len(parsed.Args) != 2 {
		t.Errorf("expected 2 args, got %d: %v", len(parsed.Args), parsed.Args)
	}
}

func TestParseCommandInvalidPrefix(t *testing.T) {
	parsed := parseCommand("!bot exception add 999")
	if parsed != nil {
		t.Error("expected nil for invalid prefix")
	}
}

func TestParseCommandEmpty(t *testing.T) {
	parsed := parseCommand("")
	if parsed != nil {
		t.Error("expected nil for empty command")
	}
}

func TestDispatcherRegisterAndDispatch(t *testing.T) {
	dispatcher := NewDispatcher()

	called := false
	handler := HandlerFunc(func(ctx context.Context, cmd *CommandContext, args []string) (string, error) {
		called = true
		return "success", nil
	})

	dispatcher.Register("test", handler)

	cmd := &CommandContext{
		GuildID: 1,
		UserID:  100,
		Raw:     "!iris test",
	}

	response, err := dispatcher.Dispatch(context.Background(), cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !called {
		t.Error("expected handler to be called")
	}

	if response != "success" {
		t.Errorf("expected 'success', got '%s'", response)
	}
}

func TestDispatcherUnknownCommand(t *testing.T) {
	dispatcher := NewDispatcher()

	cmd := &CommandContext{
		GuildID: 1,
		UserID:  100,
		Raw:     "!iris unknown",
	}

	response, err := dispatcher.Dispatch(context.Background(), cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if response != "Perintah tidak dikenali. Gunakan `!iris help` untuk melihat daftar perintah." {
		t.Errorf("unexpected response: %s", response)
	}
}

func TestDispatcherInvalidFormat(t *testing.T) {
	dispatcher := NewDispatcher()

	cmd := &CommandContext{
		GuildID: 1,
		UserID:  100,
		Raw:     "invalid",
	}

	response, err := dispatcher.Dispatch(context.Background(), cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if response != "Format perintah salah. Gunakan `!iris help` untuk melihat daftar perintah." {
		t.Errorf("unexpected response: %s", response)
	}
}

func TestDispatcherWithSubcommand(t *testing.T) {
	dispatcher := NewDispatcher()

	var capturedArgs []string
	handler := HandlerFunc(func(ctx context.Context, cmd *CommandContext, args []string) (string, error) {
		capturedArgs = args
		return "success", nil
	})

	dispatcher.Register("exception", handler)

	cmd := &CommandContext{
		GuildID: 1,
		UserID:  100,
		Raw:     "!iris exception add 999",
	}

	response, err := dispatcher.Dispatch(context.Background(), cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(capturedArgs) != 2 || capturedArgs[0] != "add" || capturedArgs[1] != "999" {
		t.Errorf("expected args ['add', '999'], got %v", capturedArgs)
	}

	if response != "success" {
		t.Errorf("expected 'success', got '%s'", response)
	}
}

func TestDispatcherIntegrationWithAuth(t *testing.T) {
	dispatcher := NewDispatcher()
	store := newFakeExceptionStore()
	audit := newFakeAuditLogger()

	exceptionHandler := NewExceptionHandler(store, audit)
	wrapped := RequireAdmin(exceptionHandler)

	dispatcher.Register("exception", wrapped)

	cmd := &CommandContext{
		GuildID: 1,
		UserID:  100,
		IsOwner: false,
		Raw:     "!iris exception add 999",
	}

	response, err := dispatcher.Dispatch(context.Background(), cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if response != "Mohon maaf, hanya admin server yang dapat mengubah konfigurasi I.R.I.S." {
		t.Errorf("expected denial, got: %s", response)
	}

	if len(store.addCalls) != 0 {
		t.Error("expected no store calls for non-admin")
	}
}

func TestDispatcherIntegrationAdminAllowed(t *testing.T) {
	dispatcher := NewDispatcher()
	store := newFakeExceptionStore()
	audit := newFakeAuditLogger()

	exceptionHandler := NewExceptionHandler(store, audit)
	wrapped := RequireAdmin(exceptionHandler)

	dispatcher.Register("exception", wrapped)

	cmd := &CommandContext{
		GuildID: 1,
		UserID:  100,
		IsOwner: true,
		Raw:     "!iris exception add 999",
	}

	response, err := dispatcher.Dispatch(context.Background(), cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if response != "Channel `999` telah ditambahkan ke daftar pengecualian." {
		t.Errorf("expected success, got: %s", response)
	}

	if len(store.addCalls) != 1 {
		t.Error("expected store call for admin")
	}
}
