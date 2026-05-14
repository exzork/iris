package lorethread

import (
	"context"
	"errors"
)

// Config holds configuration for the Service.
type Config struct {
	IdleTimeout      int // seconds before marking a session idle
	MaxSessionAge    int // seconds before expiring a session
	CompactionTarget int // percentage of context to retain after compaction (e.g., 70)
}

// Deps holds all dependencies for the Service.
type Deps struct {
	SessionStore      SessionStore
	ThreadAnchorStore ThreadAnchorStore
	GuildSettings     GuildSettingsStore
	Classifier        LoreClassifier
	Summarizer        LoreSummarizer
	TitleGenerator    TitleGenerator
	ThreadCreator     ThreadCreator
	MessageFetcher    MessageFetcher
	Clock             Clock
	Limiter           Limiter
}

// Service orchestrates lore session lifecycle and thread creation.
type Service struct {
	cfg   Config
	deps  Deps
}

// NewService creates a new Service with the given dependencies.
func NewService(cfg Config, deps Deps) *Service {
	return &Service{
		cfg:  cfg,
		deps: deps,
	}
}

// ProcessMessage processes a message and updates the lore session.
func (s *Service) ProcessMessage(ctx context.Context, msg *Message) error {
	return errors.New("not implemented")
}

// GetSession retrieves an active lore session.
func (s *Service) GetSession(ctx context.Context, guildID, channelID int64) (*Session, error) {
	return nil, errors.New("not implemented")
}

// CreateThread creates a Discord thread for a lore session.
func (s *Service) CreateThread(ctx context.Context, sessionID int64) error {
	return errors.New("not implemented")
}

// Start begins the lore session worker (not implemented in this task).
func (s *Service) Start(ctx context.Context) error {
	return errors.New("not implemented")
}

// RunOnce processes one idle session check (not implemented in this task).
func (s *Service) RunOnce(ctx context.Context) error {
	return errors.New("not implemented")
}
