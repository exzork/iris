package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"

	"github.com/eko/iris-bot/internal/tools"
)

// Supervisor owns the current MCP Manager and the persisted mcps.json file.
// It serializes add/remove/reload so config changes atomically swap the
// running manager without racing with in-flight tool calls.
type Supervisor struct {
	configPath string
	ownerID    int64
	registry   *tools.Registry

	mu      sync.Mutex
	current *Manager
	cfg     Config
}

// NewSupervisor loads cfg from configPath (missing file is fine), starts the
// initial manager, and returns the supervisor ready for hot-reload.
func NewSupervisor(ctx context.Context, configPath string, ownerID int64, registry *tools.Registry) (*Supervisor, error) {
	cfg, err := LoadConfig(configPath)
	if err != nil {
		return nil, err
	}
	s := &Supervisor{
		configPath: configPath,
		ownerID:    ownerID,
		registry:   registry,
		cfg:        cfg,
	}
	if len(cfg.MCPServers) > 0 {
		mgr, count, err := NewManager(ctx, cfg, registry)
		if err != nil {
			return nil, err
		}
		s.current = mgr
		slog.Info("mcp_supervisor_loaded", "servers", len(cfg.MCPServers), "tools_registered", count)
	}
	return s, nil
}

// OwnerID is the Discord user ID authorized to mutate MCP configuration.
// Returns 0 when no owner is configured.
func (s *Supervisor) OwnerID() int64 { return s.ownerID }

// List returns a snapshot of currently configured server names.
func (s *Supervisor) List() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	names := make([]string, 0, len(s.cfg.MCPServers))
	for name := range s.cfg.MCPServers {
		names = append(names, name)
	}
	return names
}

// Add persists a new MCP server to disk and hot-reloads the running manager.
func (s *Supervisor) Add(ctx context.Context, name string, server ServerConfig) error {
	if name == "" {
		return errors.New("mcp: server name required")
	}
	if server.Command == "" {
		return errors.New("mcp: server command required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cfg.MCPServers == nil {
		s.cfg.MCPServers = make(map[string]ServerConfig)
	}
	if _, exists := s.cfg.MCPServers[name]; exists {
		return fmt.Errorf("mcp: server %q already exists", name)
	}
	s.cfg.MCPServers[name] = server
	if err := s.persistLocked(); err != nil {
		delete(s.cfg.MCPServers, name)
		return err
	}
	return s.reloadLocked(ctx, s.currentPrefixes())
}

// Remove uninstalls an MCP server by name, persists, and hot-reloads.
func (s *Supervisor) Remove(ctx context.Context, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.cfg.MCPServers[name]; !exists {
		return fmt.Errorf("mcp: server %q not found", name)
	}
	backup := s.cfg.MCPServers[name]
	oldPrefixes := s.currentPrefixes()
	delete(s.cfg.MCPServers, name)
	if err := s.persistLocked(); err != nil {
		s.cfg.MCPServers[name] = backup
		return err
	}
	return s.reloadLocked(ctx, oldPrefixes)
}

// Close stops the currently running manager. Safe to call multiple times.
func (s *Supervisor) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current != nil {
		err := s.current.Close()
		s.current = nil
		return err
	}
	return nil
}

func (s *Supervisor) persistLocked() error {
	raw, err := json.MarshalIndent(s.cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("mcp: marshal config: %w", err)
	}
	raw = append(raw, '\n')
	tmp := s.configPath + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return fmt.Errorf("mcp: write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, s.configPath); err != nil {
		return fmt.Errorf("mcp: rename %s -> %s: %w", tmp, s.configPath, err)
	}
	return nil
}

// reloadLocked stops the current manager, evicts tools whose prefixes appear
// in either oldPrefixes or s.cfg, and starts a fresh manager. Callers must
// hold s.mu.
func (s *Supervisor) reloadLocked(ctx context.Context, oldPrefixes []string) error {
	if s.current != nil {
		_ = s.current.Close()
		s.current = nil
	}
	purge := make(map[string]bool, len(oldPrefixes)+len(s.cfg.MCPServers))
	for _, p := range oldPrefixes {
		purge[p] = true
	}
	for name := range s.cfg.MCPServers {
		purge[sanitize(name)+"_"] = true
	}
	prefixes := make([]string, 0, len(purge))
	for p := range purge {
		prefixes = append(prefixes, p)
	}
	s.registry.UnregisterPrefix(prefixes)

	if len(s.cfg.MCPServers) == 0 {
		return nil
	}
	mgr, count, err := NewManager(ctx, s.cfg, s.registry)
	if err != nil {
		return err
	}
	s.current = mgr
	slog.Info("mcp_supervisor_reloaded", "servers", len(s.cfg.MCPServers), "tools_registered", count)
	return nil
}

func (s *Supervisor) currentPrefixes() []string {
	prefixes := make([]string, 0, len(s.cfg.MCPServers))
	for name := range s.cfg.MCPServers {
		prefixes = append(prefixes, sanitize(name)+"_")
	}
	return prefixes
}
