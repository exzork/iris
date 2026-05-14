package wire

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/eko/iris-bot/internal/domain"
	"github.com/jackc/pgx/v5"
)

type MockLoreSessionRepo struct {
	getOpenByChannelErr error
	getOpenByChannelRes *domain.LoreSession
}

func (m *MockLoreSessionRepo) GetOpenByChannel(ctx context.Context, guildID, channelID int64) (*domain.LoreSession, error) {
	if m.getOpenByChannelErr != nil {
		return nil, m.getOpenByChannelErr
	}
	return m.getOpenByChannelRes, nil
}

func (m *MockLoreSessionRepo) OpenOrRefresh(ctx context.Context, guildID int64, channelID int64, msgID int64, msgTime time.Time, idleDeadline time.Time) (int64, error) {
	return 0, nil
}

func (m *MockLoreSessionRepo) GetByID(ctx context.Context, id int64) (*domain.LoreSession, error) {
	return nil, nil
}

func (m *MockLoreSessionRepo) ClaimDueForSummary(ctx context.Context, now time.Time) (*domain.LoreSession, error) {
	return nil, nil
}

func (m *MockLoreSessionRepo) MarkStatus(ctx context.Context, id int64, status string) error {
	return nil
}

func (m *MockLoreSessionRepo) SetThreadResult(ctx context.Context, id int64, threadID int64, summaryMsgID int64, title string, summary string) error {
	return nil
}

func (m *MockLoreSessionRepo) IncrementRetry(ctx context.Context, id int64, lastErr string) error {
	return nil
}

func TestGetActive_NoSessionReturnsNilNil(t *testing.T) {
	mockRepo := &MockLoreSessionRepo{
		getOpenByChannelErr: fmt.Errorf("failed to get open lore session: %w", pgx.ErrNoRows),
	}

	adapter := NewLoreSessionStoreAdapter(mockRepo)

	session, err := adapter.GetActive(context.Background(), 123, 456)

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if session != nil {
		t.Fatalf("expected nil session, got %v", session)
	}
}

func TestGetActive_OtherErrorPropagates(t *testing.T) {
	expectedErr := fmt.Errorf("database connection failed")
	mockRepo := &MockLoreSessionRepo{
		getOpenByChannelErr: expectedErr,
	}

	adapter := NewLoreSessionStoreAdapter(mockRepo)

	session, err := adapter.GetActive(context.Background(), 123, 456)

	if err != expectedErr {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}

	if session != nil {
		t.Fatalf("expected nil session, got %v", session)
	}
}

func TestGetActive_SessionFound(t *testing.T) {
	domainSession := &domain.LoreSession{
		ID:        1,
		GuildID:   123,
		ChannelID: 456,
		Status:    "open",
	}

	mockRepo := &MockLoreSessionRepo{
		getOpenByChannelRes: domainSession,
	}

	adapter := NewLoreSessionStoreAdapter(mockRepo)

	session, err := adapter.GetActive(context.Background(), 123, 456)

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if session == nil {
		t.Fatal("expected session to be returned")
	}

	if session.ID != 1 {
		t.Fatalf("expected session ID 1, got %d", session.ID)
	}
}
