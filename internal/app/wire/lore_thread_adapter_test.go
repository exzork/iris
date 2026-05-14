package wire

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eko/iris-bot/internal/lorethread"
)

// ThreadCreatorGateway is a minimal interface for testing thread creation.
type ThreadCreatorGateway interface {
	CreateThreadFromMessage(ctx context.Context, guildID, channelID, parentMessageID int64, name string, archiveAfter time.Duration) (int64, error)
	SendMessageToThread(ctx context.Context, threadID int64, content string) (int64, error)
}

// MockGateway implements ThreadCreatorGateway for testing.
type MockGateway struct {
	createThreadErr    error
	createThreadID     int64
	sendMessageErr     error
	sendMessageID      int64
	createThreadCalled bool
	sendMessageCalled  bool
	lastThreadName     string
}

func (m *MockGateway) CreateThreadFromMessage(ctx context.Context, guildID, channelID, parentMessageID int64, name string, archiveAfter time.Duration) (int64, error) {
	m.createThreadCalled = true
	m.lastThreadName = name
	if m.createThreadErr != nil {
		return 0, m.createThreadErr
	}
	return m.createThreadID, nil
}

func (m *MockGateway) SendMessageToThread(ctx context.Context, threadID int64, content string) (int64, error) {
	m.sendMessageCalled = true
	if m.sendMessageErr != nil {
		return 0, m.sendMessageErr
	}
	return m.sendMessageID, nil
}

func TestDiscordThreadCreatorCreate_Success(t *testing.T) {
	mock := &MockGateway{
		createThreadID: 999,
		sendMessageID:  888,
	}

	adapter := NewDiscordThreadCreator(mock)

	req := &lorethread.ThreadCreateRequest{
		GuildID:         123,
		ChannelID:       456,
		ParentMessageID: 789,
		Title:           "Test Thread",
		FirstMessage:    "This is a test summary",
	}

	result, err := adapter.Create(context.Background(), req)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if result.ThreadID != 999 {
		t.Errorf("expected ThreadID 999, got %d", result.ThreadID)
	}
	if result.MessageID != 888 {
		t.Errorf("expected MessageID 888, got %d", result.MessageID)
	}
	if !mock.createThreadCalled {
		t.Error("expected CreateThreadFromMessage to be called")
	}
	if !mock.sendMessageCalled {
		t.Error("expected SendMessageToThread to be called")
	}
}

func TestDiscordThreadCreatorCreate_DMRejection(t *testing.T) {
	mock := &MockGateway{}

	adapter := NewDiscordThreadCreator(mock)

	req := &lorethread.ThreadCreateRequest{
		GuildID:         0, // DM marker
		ChannelID:       456,
		ParentMessageID: 789,
		Title:           "Test Thread",
		FirstMessage:    "This is a test summary",
	}

	result, err := adapter.Create(context.Background(), req)

	if !errors.Is(err, lorethread.ErrDMNotSupported) {
		t.Errorf("expected ErrDMNotSupported, got %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result, got %v", result)
	}
	if mock.createThreadCalled {
		t.Error("expected CreateThreadFromMessage NOT to be called")
	}
}

func TestDiscordThreadCreatorCreate_LongMessageRejection(t *testing.T) {
	mock := &MockGateway{}

	adapter := NewDiscordThreadCreator(mock)

	// Create a message longer than 2000 characters
	longMessage := ""
	for i := 0; i < 2001; i++ {
		longMessage += "x"
	}

	req := &lorethread.ThreadCreateRequest{
		GuildID:         123,
		ChannelID:       456,
		ParentMessageID: 789,
		Title:           "Test Thread",
		FirstMessage:    longMessage,
	}

	result, err := adapter.Create(context.Background(), req)

	if !errors.Is(err, lorethread.ErrFirstMessageTooLong) {
		t.Errorf("expected ErrFirstMessageTooLong, got %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result, got %v", result)
	}
	if mock.createThreadCalled {
		t.Error("expected CreateThreadFromMessage NOT to be called")
	}
}

func TestDiscordThreadCreatorCreate_LongNameTruncation(t *testing.T) {
	mock := &MockGateway{
		createThreadID: 999,
		sendMessageID:  888,
	}

	adapter := NewDiscordThreadCreator(mock)

	// Create a title longer than 100 characters
	longTitle := ""
	for i := 0; i < 110; i++ {
		longTitle += "a"
	}

	req := &lorethread.ThreadCreateRequest{
		GuildID:         123,
		ChannelID:       456,
		ParentMessageID: 789,
		Title:           longTitle,
		FirstMessage:    "This is a test summary",
	}

	result, err := adapter.Create(context.Background(), req)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}

	// Verify the thread name was truncated to 100 characters
	if len(mock.lastThreadName) != 100 {
		t.Errorf("expected thread name length 100, got %d", len(mock.lastThreadName))
	}
	if mock.lastThreadName[97:] != "..." {
		t.Errorf("expected thread name to end with '...', got %q", mock.lastThreadName[97:])
	}
}

func TestDiscordThreadCreatorCreate_CreateThreadError(t *testing.T) {
	mock := &MockGateway{
		createThreadErr: errors.New("permission denied"),
	}

	adapter := NewDiscordThreadCreator(mock)

	req := &lorethread.ThreadCreateRequest{
		GuildID:         123,
		ChannelID:       456,
		ParentMessageID: 789,
		Title:           "Test Thread",
		FirstMessage:    "This is a test summary",
	}

	result, err := adapter.Create(context.Background(), req)

	if err == nil {
		t.Error("expected error, got nil")
	}
	if result != nil {
		t.Errorf("expected nil result, got %v", result)
	}
	if !mock.createThreadCalled {
		t.Error("expected CreateThreadFromMessage to be called")
	}
	if mock.sendMessageCalled {
		t.Error("expected SendMessageToThread NOT to be called after thread creation error")
	}
}

func TestDiscordThreadCreatorCreate_SendMessageError(t *testing.T) {
	mock := &MockGateway{
		createThreadID: 999,
		sendMessageErr: errors.New("rate limited"),
	}

	adapter := NewDiscordThreadCreator(mock)

	req := &lorethread.ThreadCreateRequest{
		GuildID:         123,
		ChannelID:       456,
		ParentMessageID: 789,
		Title:           "Test Thread",
		FirstMessage:    "This is a test summary",
	}

	result, err := adapter.Create(context.Background(), req)

	if err == nil {
		t.Error("expected error, got nil")
	}
	if result != nil {
		t.Errorf("expected nil result, got %v", result)
	}
	if !mock.createThreadCalled {
		t.Error("expected CreateThreadFromMessage to be called")
	}
	if !mock.sendMessageCalled {
		t.Error("expected SendMessageToThread to be called")
	}
}
