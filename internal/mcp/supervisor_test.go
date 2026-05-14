package mcp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eko/iris-bot/internal/tools"
)

func TestSupervisor_AddPersistsAndReloads(t *testing.T) {
	bin := buildFakeServer(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "mcps.json")

	reg := tools.NewRegistry(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	sup, err := NewSupervisor(ctx, path, 999, reg)
	if err != nil {
		t.Fatalf("NewSupervisor: %v", err)
	}
	defer sup.Close()

	if err := sup.Add(ctx, "fake", ServerConfig{Command: bin}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	names := reg.List()
	found := false
	for _, n := range names {
		if n == "fake_echo" {
			found = true
		}
	}
	if !found {
		t.Fatalf("fake_echo must be registered after Add; got %v", names)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read persisted config: %v", err)
	}
	if len(raw) == 0 {
		t.Fatalf("persisted config must not be empty")
	}

	sup2, err := NewSupervisor(ctx, path, 999, tools.NewRegistry(nil))
	if err != nil {
		t.Fatalf("second supervisor on persisted file: %v", err)
	}
	defer sup2.Close()
	if len(sup2.List()) != 1 {
		t.Fatalf("expected persisted config to be loaded on restart, got %v", sup2.List())
	}
}

func TestSupervisor_RemoveRemovesToolFromRegistry(t *testing.T) {
	bin := buildFakeServer(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "mcps.json")

	reg := tools.NewRegistry(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	sup, err := NewSupervisor(ctx, path, 999, reg)
	if err != nil {
		t.Fatalf("NewSupervisor: %v", err)
	}
	defer sup.Close()

	if err := sup.Add(ctx, "fake", ServerConfig{Command: bin}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := sup.Remove(ctx, "fake"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	for _, n := range reg.List() {
		if n == "fake_echo" {
			t.Fatalf("fake_echo should be gone after Remove, got %v", reg.List())
		}
	}
	if len(sup.List()) != 0 {
		t.Fatalf("supervisor.List should be empty after Remove")
	}
}

func TestAddTool_RequiresOwner(t *testing.T) {
	bin := buildFakeServer(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "mcps.json")

	reg := tools.NewRegistry(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	sup, err := NewSupervisor(ctx, path, 12345, reg)
	if err != nil {
		t.Fatalf("NewSupervisor: %v", err)
	}
	defer sup.Close()

	tool := NewAddTool(sup)
	args := map[string]interface{}{
		"name":    "fake",
		"command": bin,
	}

	if _, err := tool.Run(ctx, args); err == nil {
		t.Fatalf("expected error when caller user ID not in ctx")
	}

	wrongCtx := WithCallerUserID(ctx, 99999)
	if _, err := tool.Run(wrongCtx, args); err == nil {
		t.Fatalf("expected error for wrong caller user ID")
	}

	ownerCtx := WithCallerUserID(ctx, 12345)
	if _, err := tool.Run(ownerCtx, args); err != nil {
		t.Fatalf("owner should succeed, got %v", err)
	}
}

func TestAddTool_RefusesWhenOwnerUnset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcps.json")
	reg := tools.NewRegistry(nil)
	ctx := context.Background()

	sup, err := NewSupervisor(ctx, path, 0, reg)
	if err != nil {
		t.Fatalf("NewSupervisor: %v", err)
	}
	defer sup.Close()

	tool := NewAddTool(sup)
	ownerCtx := WithCallerUserID(ctx, 12345)
	if _, err := tool.Run(ownerCtx, map[string]interface{}{"name": "x", "command": "/bin/true"}); err == nil {
		t.Fatalf("must refuse when owner ID is 0, even if caller ID matches")
	}
}

func TestListTool_OwnerOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcps.json")
	reg := tools.NewRegistry(nil)
	ctx := context.Background()

	sup, err := NewSupervisor(ctx, path, 12345, reg)
	if err != nil {
		t.Fatalf("NewSupervisor: %v", err)
	}
	defer sup.Close()

	tool := NewListTool(sup)
	if _, err := tool.Run(ctx, nil); err == nil {
		t.Fatalf("list must refuse without caller ID")
	}
	if _, err := tool.Run(WithCallerUserID(ctx, 99), nil); err == nil {
		t.Fatalf("list must refuse for non-owner")
	}
	if _, err := tool.Run(WithCallerUserID(ctx, 12345), nil); err != nil {
		t.Fatalf("list must succeed for owner, got %v", err)
	}
}
