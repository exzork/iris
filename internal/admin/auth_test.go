package admin

import (
	"context"
	"testing"
)

func TestIsAdminOwner(t *testing.T) {
	cmd := &CommandContext{
		GuildID:     1,
		UserID:      100,
		IsOwner:     true,
		Permissions: 0,
		RoleIDs:     []int64{},
	}

	if !IsAdmin(cmd) {
		t.Error("expected owner to be admin")
	}
}

func TestIsAdminWithAdministratorPermission(t *testing.T) {
	cmd := &CommandContext{
		GuildID:     1,
		UserID:      100,
		IsOwner:     false,
		Permissions: 0x8, // Administrator bit
		RoleIDs:     []int64{},
	}

	if !IsAdmin(cmd) {
		t.Error("expected user with Administrator permission to be admin")
	}
}

func TestIsAdminWithWhitelistedRole(t *testing.T) {
	cmd := &CommandContext{
		GuildID:     1,
		UserID:      100,
		IsOwner:     false,
		Permissions: 0,
		RoleIDs:     []int64{999, 1000, 1001},
	}

	adminRoles := map[int64]bool{1000: true}

	if !IsAdminWithRoles(cmd, adminRoles) {
		t.Error("expected user with whitelisted role to be admin")
	}
}

func TestIsAdminNonAdmin(t *testing.T) {
	cmd := &CommandContext{
		GuildID:     1,
		UserID:      100,
		IsOwner:     false,
		Permissions: 0,
		RoleIDs:     []int64{999},
	}

	adminRoles := map[int64]bool{1000: true}

	if IsAdminWithRoles(cmd, adminRoles) {
		t.Error("expected non-admin user to not be admin")
	}
}

func TestRequireAdminDeniesNonAdmin(t *testing.T) {
	called := false
	handler := HandlerFunc(func(ctx context.Context, cmd *CommandContext, args []string) (string, error) {
		called = true
		return "success", nil
	})

	wrapped := RequireAdmin(handler)

	cmd := &CommandContext{
		GuildID:     1,
		UserID:      100,
		IsOwner:     false,
		Permissions: 0,
		RoleIDs:     []int64{},
	}

	response, err := wrapped.Handle(context.Background(), cmd, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if response != "Mohon maaf, hanya admin server yang dapat mengubah konfigurasi I.R.I.S." {
		t.Errorf("expected Indonesian denial, got: %s", response)
	}

	if called {
		t.Error("expected handler not to be called for non-admin")
	}
}

func TestRequireAdminAllowsAdmin(t *testing.T) {
	called := false
	handler := HandlerFunc(func(ctx context.Context, cmd *CommandContext, args []string) (string, error) {
		called = true
		return "success", nil
	})

	wrapped := RequireAdmin(handler)

	cmd := &CommandContext{
		GuildID:     1,
		UserID:      100,
		IsOwner:     true,
		Permissions: 0,
		RoleIDs:     []int64{},
	}

	response, err := wrapped.Handle(context.Background(), cmd, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if response != "success" {
		t.Errorf("expected success, got: %s", response)
	}

	if !called {
		t.Error("expected handler to be called for admin")
	}
}
