package wire

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/eko/iris-bot/internal/lorethread"
)

type ThreadCreatorGateway interface {
	CreateThreadFromMessage(ctx context.Context, guildID, channelID, parentMessageID int64, name string, archiveAfter time.Duration) (int64, error)
	CreateThread(ctx context.Context, guildID, channelID int64, name string, archiveAfter time.Duration) (int64, error)
	SendMessageToThread(ctx context.Context, threadID int64, content string) (int64, error)
}

type MockGateway struct {
	createFromMessageErr   error
	createFromMessageID    int64
	createStandaloneErr    error
	createStandaloneID     int64
	sendMessageErr         error
	sendMessageIDs         []int64
	createFromMessageCalls int
	createStandaloneCalls  int
	sendMessageCalls       int
	lastThreadName         string
	sentChunks             []string
}

func (m *MockGateway) CreateThreadFromMessage(ctx context.Context, guildID, channelID, parentMessageID int64, name string, archiveAfter time.Duration) (int64, error) {
	m.createFromMessageCalls++
	m.lastThreadName = name
	if m.createFromMessageErr != nil {
		return 0, m.createFromMessageErr
	}
	return m.createFromMessageID, nil
}

func (m *MockGateway) CreateThread(ctx context.Context, guildID, channelID int64, name string, archiveAfter time.Duration) (int64, error) {
	m.createStandaloneCalls++
	m.lastThreadName = name
	if m.createStandaloneErr != nil {
		return 0, m.createStandaloneErr
	}
	return m.createStandaloneID, nil
}

func (m *MockGateway) SendMessageToThread(ctx context.Context, threadID int64, content string) (int64, error) {
	m.sendMessageCalls++
	m.sentChunks = append(m.sentChunks, content)
	if m.sendMessageErr != nil {
		return 0, m.sendMessageErr
	}
	if len(m.sendMessageIDs) > 0 {
		idx := m.sendMessageCalls - 1
		if idx < len(m.sendMessageIDs) {
			return m.sendMessageIDs[idx], nil
		}
		return m.sendMessageIDs[len(m.sendMessageIDs)-1], nil
	}
	return 0, nil
}

func TestDiscordThreadCreatorCreate_Success(t *testing.T) {
	mock := &MockGateway{
		createFromMessageID: 999,
		sendMessageIDs:      []int64{888},
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
	if mock.createFromMessageCalls != 1 {
		t.Errorf("expected CreateThreadFromMessage call count 1, got %d", mock.createFromMessageCalls)
	}
	if mock.createStandaloneCalls != 0 {
		t.Errorf("expected CreateThread NOT to be called, got %d calls", mock.createStandaloneCalls)
	}
	if mock.sendMessageCalls != 1 {
		t.Errorf("expected 1 send call, got %d", mock.sendMessageCalls)
	}
}

func TestDiscordThreadCreatorCreate_DMRejection(t *testing.T) {
	mock := &MockGateway{}

	adapter := NewDiscordThreadCreator(mock)

	req := &lorethread.ThreadCreateRequest{
		GuildID:         0,
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
	if mock.createFromMessageCalls != 0 {
		t.Errorf("expected CreateThreadFromMessage NOT to be called, got %d calls", mock.createFromMessageCalls)
	}
}

func TestDiscordThreadCreatorCreate_LongMessageChunked(t *testing.T) {
	mock := &MockGateway{
		createFromMessageID: 999,
		sendMessageIDs:      []int64{8881, 8882, 8883},
	}

	adapter := NewDiscordThreadCreator(mock)

	longMessage := strings.Repeat("x", 5500)

	req := &lorethread.ThreadCreateRequest{
		GuildID:         123,
		ChannelID:       456,
		ParentMessageID: 789,
		Title:           "Test Thread",
		FirstMessage:    longMessage,
	}

	result, err := adapter.Create(context.Background(), req)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if result.MessageID != 8881 {
		t.Errorf("expected anchor MessageID to be first chunk id 8881, got %d", result.MessageID)
	}
	if mock.sendMessageCalls != 3 {
		t.Errorf("expected 3 chunk sends, got %d", mock.sendMessageCalls)
	}
	for i, chunk := range mock.sentChunks {
		if len(chunk) > 2000 {
			t.Errorf("chunk %d exceeds 2000 chars: %d", i, len(chunk))
		}
	}
}

func TestDiscordThreadCreatorCreate_LongNameTruncation(t *testing.T) {
	mock := &MockGateway{
		createFromMessageID: 999,
		sendMessageIDs:      []int64{888},
	}

	adapter := NewDiscordThreadCreator(mock)

	longTitle := strings.Repeat("a", 110)

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

	if len(mock.lastThreadName) != 100 {
		t.Errorf("expected thread name length 100, got %d", len(mock.lastThreadName))
	}
	if mock.lastThreadName[97:] != "..." {
		t.Errorf("expected thread name to end with '...', got %q", mock.lastThreadName[97:])
	}
}

func TestDiscordThreadCreatorCreate_CreateThreadError(t *testing.T) {
	mock := &MockGateway{
		createFromMessageErr: errors.New("permission denied"),
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
	if mock.createFromMessageCalls != 1 {
		t.Errorf("expected CreateThreadFromMessage to be called once, got %d", mock.createFromMessageCalls)
	}
	if mock.sendMessageCalls != 0 {
		t.Error("expected SendMessageToThread NOT to be called after thread creation error")
	}
}

func TestDiscordThreadCreatorCreate_SendMessageError(t *testing.T) {
	mock := &MockGateway{
		createFromMessageID: 999,
		sendMessageErr:      errors.New("rate limited"),
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
	if mock.createFromMessageCalls != 1 {
		t.Errorf("expected CreateThreadFromMessage to be called once, got %d", mock.createFromMessageCalls)
	}
	if mock.sendMessageCalls != 1 {
		t.Errorf("expected one send attempt, got %d", mock.sendMessageCalls)
	}
}

func TestDiscordThreadCreatorCreate_ThreadAlreadyExists_FallsBackToStandalone(t *testing.T) {
	mock := &MockGateway{
		createFromMessageErr: lorethread.ErrThreadAlreadyExists,
		createStandaloneID:   1234,
		sendMessageIDs:       []int64{5678},
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
		t.Fatalf("expected no error after fallback, got %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if result.ThreadID != 1234 {
		t.Errorf("expected fallback ThreadID 1234, got %d", result.ThreadID)
	}
	if result.MessageID != 5678 {
		t.Errorf("expected MessageID 5678, got %d", result.MessageID)
	}
	if mock.createFromMessageCalls != 1 {
		t.Errorf("expected CreateThreadFromMessage call count 1, got %d", mock.createFromMessageCalls)
	}
	if mock.createStandaloneCalls != 1 {
		t.Errorf("expected CreateThread fallback call count 1, got %d", mock.createStandaloneCalls)
	}
	if mock.sendMessageCalls != 1 {
		t.Errorf("expected 1 send call, got %d", mock.sendMessageCalls)
	}
}

func TestDiscordThreadCreatorCreate_StandaloneFallbackError(t *testing.T) {
	mock := &MockGateway{
		createFromMessageErr: lorethread.ErrThreadAlreadyExists,
		createStandaloneErr:  errors.New("permission denied"),
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
	if mock.createStandaloneCalls != 1 {
		t.Errorf("expected CreateThread fallback to be attempted once, got %d", mock.createStandaloneCalls)
	}
	if mock.sendMessageCalls != 0 {
		t.Error("expected SendMessageToThread NOT to be called after fallback failure")
	}
}
