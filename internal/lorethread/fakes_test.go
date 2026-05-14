package lorethread

import (
	"context"
	"sync"
	"time"
)

// FakeSessionStore implements SessionStore for testing.
type FakeSessionStore struct {
	sessions map[int64]*Session
	mu       sync.RWMutex
}

func NewFakeSessionStore() *FakeSessionStore {
	return &FakeSessionStore{
		sessions: make(map[int64]*Session),
	}
}

func (f *FakeSessionStore) Create(ctx context.Context, session *Session) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if session.ID == 0 {
		session.ID = int64(len(f.sessions) + 1)
	}
	f.sessions[session.ID] = session
	return nil
}

func (f *FakeSessionStore) GetByID(ctx context.Context, id int64) (*Session, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if s, ok := f.sessions[id]; ok {
		return s, nil
	}
	return nil, nil
}

func (f *FakeSessionStore) GetActive(ctx context.Context, guildID, channelID int64) (*Session, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	for _, s := range f.sessions {
		if s.GuildID == guildID && s.ChannelID == channelID && s.Status == "open" {
			return s, nil
		}
	}
	return nil, nil
}

func (f *FakeSessionStore) Update(ctx context.Context, session *Session) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sessions[session.ID] = session
	return nil
}

func (f *FakeSessionStore) ListByGuild(ctx context.Context, guildID int64) ([]*Session, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	var result []*Session
	for _, s := range f.sessions {
		if s.GuildID == guildID {
			result = append(result, s)
		}
	}
	return result, nil
}

// ClaimDueForSummary claims a session due for summary processing (extended interface for Worker).
func (f *FakeSessionStore) ClaimDueForSummary(ctx context.Context, now time.Time) (*Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, s := range f.sessions {
		if s.Status == "open" && s.IdleDeadline.Before(now) {
			s.Status = "summarizing"
			return s, nil
		}
	}
	return nil, ErrNoSessionsDue
}

// MarkStatus updates session status (extended interface for Worker).
func (f *FakeSessionStore) MarkStatus(ctx context.Context, id int64, status string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if s, ok := f.sessions[id]; ok {
		s.Status = status
		return nil
	}
	return nil
}

// SetThreadResult sets thread creation result (extended interface for Worker).
func (f *FakeSessionStore) SetThreadResult(ctx context.Context, id int64, threadID int64, summaryMsgID int64, title string, summary string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if s, ok := f.sessions[id]; ok {
		s.ThreadID = &threadID
		s.SummaryMessageID = &summaryMsgID
		s.Title = &title
		s.Summary = &summary
		s.Status = "thread_created"
		return nil
	}
	return nil
}

// IncrementRetry increments retry count (extended interface for Worker).
func (f *FakeSessionStore) IncrementRetry(ctx context.Context, id int64, lastErr string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if s, ok := f.sessions[id]; ok {
		s.RetryCount++
		s.LastError = &lastErr
		return nil
	}
	return nil
}

// FakeThreadAnchorStore implements ThreadAnchorStore for testing.
type FakeThreadAnchorStore struct {
	anchors map[int64]*ThreadAnchor
	mu      sync.RWMutex
}

func NewFakeThreadAnchorStore() *FakeThreadAnchorStore {
	return &FakeThreadAnchorStore{
		anchors: make(map[int64]*ThreadAnchor),
	}
}

func (f *FakeThreadAnchorStore) Create(ctx context.Context, sessionID, threadID, messageID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.anchors[sessionID] = &ThreadAnchor{
		SessionID: sessionID,
		ThreadID:  threadID,
		MessageID: messageID,
	}
	return nil
}

func (f *FakeThreadAnchorStore) GetBySessionID(ctx context.Context, sessionID int64) (threadID, messageID int64, err error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if a, ok := f.anchors[sessionID]; ok {
		return a.ThreadID, a.MessageID, nil
	}
	return 0, 0, nil
}

func (f *FakeThreadAnchorStore) GetByThreadID(ctx context.Context, threadID int64) (sessionID int64, err error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	for _, a := range f.anchors {
		if a.ThreadID == threadID {
			return a.SessionID, nil
		}
	}
	return 0, nil
}

// FakeGuildSettingsStore implements GuildSettingsStore for testing.
type FakeGuildSettingsStore struct {
	settings map[int64]bool
	mu       sync.RWMutex
}

func NewFakeGuildSettingsStore() *FakeGuildSettingsStore {
	return &FakeGuildSettingsStore{
		settings: make(map[int64]bool),
	}
}

func (f *FakeGuildSettingsStore) GetLoreThreadEnabled(ctx context.Context, guildID int64) (bool, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.settings[guildID], nil
}

func (f *FakeGuildSettingsStore) SetLoreThreadEnabled(ctx context.Context, guildID int64, enabled bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.settings[guildID] = enabled
	return nil
}

// FakeLLMCaller implements LLMCaller for testing.
type FakeLLMCaller struct {
	responses map[string]string
	callCount int
	mu        sync.RWMutex
}

func NewFakeLLMCaller() *FakeLLMCaller {
	return &FakeLLMCaller{
		responses: make(map[string]string),
	}
}

func (f *FakeLLMCaller) Call(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.callCount++
	// Return a scripted response based on userPrompt content
	if resp, ok := f.responses[userPrompt]; ok {
		return resp, nil
	}
	return "default response", nil
}

func (f *FakeLLMCaller) SetResponse(userPrompt, response string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.responses[userPrompt] = response
}

func (f *FakeLLMCaller) CallCount() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.callCount
}

// FakeLoreClassifier implements LoreClassifier for testing.
type FakeLoreClassifier struct {
	classifyFn func(ctx context.Context, guildID int64, message *Message) (*ClassifyResult, error)
}

func NewFakeLoreClassifier(classifyFn func(ctx context.Context, guildID int64, message *Message) (*ClassifyResult, error)) *FakeLoreClassifier {
	return &FakeLoreClassifier{classifyFn: classifyFn}
}

func (f *FakeLoreClassifier) Classify(ctx context.Context, guildID int64, message *Message) (*ClassifyResult, error) {
	if f.classifyFn != nil {
		return f.classifyFn(ctx, guildID, message)
	}
	return &ClassifyResult{IsLore: true, Reason: "test"}, nil
}

// FakeLoreSummarizer implements LoreSummarizer for testing.
type FakeLoreSummarizer struct {
	summarizeFn func(ctx context.Context, req *SummaryRequest) (*SummaryResult, error)
}

func NewFakeLoreSummarizer(summarizeFn func(ctx context.Context, req *SummaryRequest) (*SummaryResult, error)) *FakeLoreSummarizer {
	return &FakeLoreSummarizer{summarizeFn: summarizeFn}
}

func (f *FakeLoreSummarizer) Summarize(ctx context.Context, req *SummaryRequest) (*SummaryResult, error) {
	if f.summarizeFn != nil {
		return f.summarizeFn(ctx, req)
	}
	return &SummaryResult{
		Title:   "Test Lore Summary",
		Summary: "This is a test summary of lore messages.",
	}, nil
}

// FakeTitleGenerator implements TitleGenerator for testing.
type FakeTitleGenerator struct {
	generateFn func(ctx context.Context, guildID int64, messages []*Message) (string, error)
}

func NewFakeTitleGenerator(generateFn func(ctx context.Context, guildID int64, messages []*Message) (string, error)) *FakeTitleGenerator {
	return &FakeTitleGenerator{generateFn: generateFn}
}

func (f *FakeTitleGenerator) Generate(ctx context.Context, guildID int64, messages []*Message) (string, error) {
	if f.generateFn != nil {
		return f.generateFn(ctx, guildID, messages)
	}
	return "Test Lore Title", nil
}

// FakeThreadCreator implements ThreadCreator for testing.
type FakeThreadCreator struct {
	createFn func(ctx context.Context, req *ThreadCreateRequest) (*ThreadCreateResult, error)
	created  []*ThreadCreateRequest
	mu       sync.RWMutex
}

func NewFakeThreadCreator(createFn func(ctx context.Context, req *ThreadCreateRequest) (*ThreadCreateResult, error)) *FakeThreadCreator {
	return &FakeThreadCreator{
		createFn: createFn,
		created:  []*ThreadCreateRequest{},
	}
}

func (f *FakeThreadCreator) Create(ctx context.Context, req *ThreadCreateRequest) (*ThreadCreateResult, error) {
	f.mu.Lock()
	f.created = append(f.created, req)
	f.mu.Unlock()

	if f.createFn != nil {
		return f.createFn(ctx, req)
	}
	return &ThreadCreateResult{
		ThreadID:  999999,
		MessageID: 888888,
	}, nil
}

func (f *FakeThreadCreator) CreatedCount() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.created)
}

func (f *FakeThreadCreator) GetCreated(index int) *ThreadCreateRequest {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if index < len(f.created) {
		return f.created[index]
	}
	return nil
}

// FakeMessageFetcher implements MessageFetcher for testing.
type FakeMessageFetcher struct {
	messages map[int64]*Message
	mu       sync.RWMutex
}

func NewFakeMessageFetcher() *FakeMessageFetcher {
	return &FakeMessageFetcher{
		messages: make(map[int64]*Message),
	}
}

func (f *FakeMessageFetcher) FetchRecent(ctx context.Context, guildID, channelID int64, limit int) ([]*Message, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	var result []*Message
	for _, msg := range f.messages {
		if msg.GuildID == guildID && msg.ChannelID == channelID {
			result = append(result, msg)
		}
	}
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (f *FakeMessageFetcher) FetchByID(ctx context.Context, guildID, messageID int64) (*Message, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if msg, ok := f.messages[messageID]; ok {
		return msg, nil
	}
	return nil, nil
}

func (f *FakeMessageFetcher) AddMessage(msg *Message) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.messages[msg.ID] = msg
}

// FakeLimiter implements Limiter for testing.
type FakeLimiter struct {
	allowedGuilds map[int64]int
	mu            sync.RWMutex
}

func NewFakeLimiter() *FakeLimiter {
	return &FakeLimiter{
		allowedGuilds: make(map[int64]int),
	}
}

func (f *FakeLimiter) Allow(ctx context.Context, guildID int64) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if count, ok := f.allowedGuilds[guildID]; ok && count > 0 {
		f.allowedGuilds[guildID]--
		return true
	}
	return false
}

func (f *FakeLimiter) Reset(ctx context.Context, guildID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.allowedGuilds[guildID] = 10 // arbitrary cap
	return nil
}

func (f *FakeLimiter) SetAllowed(guildID int64, count int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.allowedGuilds[guildID] = count
}
