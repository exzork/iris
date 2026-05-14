package lorethread

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type workerSessionStoreMock struct {
	mu sync.Mutex

	dueQueue []*Session
	sessions map[int64]*Session

	claims int

	statusHistory map[int64][]string
	retryErrors   map[int64][]string
}

func newWorkerSessionStoreMock(due ...*Session) *workerSessionStoreMock {
	s := &workerSessionStoreMock{
		dueQueue:      make([]*Session, 0, len(due)),
		sessions:      make(map[int64]*Session),
		statusHistory: make(map[int64][]string),
		retryErrors:   make(map[int64][]string),
	}
	for _, sess := range due {
		if sess == nil {
			continue
		}
		cloned := cloneSession(sess)
		s.dueQueue = append(s.dueQueue, cloned)
		s.sessions[cloned.ID] = cloneSession(cloned)
	}
	return s
}

func (m *workerSessionStoreMock) Create(ctx context.Context, session *Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[session.ID] = cloneSession(session)
	return nil
}

func (m *workerSessionStoreMock) GetByID(ctx context.Context, id int64) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[id]; ok {
		return cloneSession(s), nil
	}
	return nil, nil
}

func (m *workerSessionStoreMock) GetActive(ctx context.Context, guildID, channelID int64) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.sessions {
		if s.GuildID == guildID && s.ChannelID == channelID && s.Status == "open" {
			return cloneSession(s), nil
		}
	}
	return nil, nil
}

func (m *workerSessionStoreMock) Update(ctx context.Context, session *Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[session.ID] = cloneSession(session)
	return nil
}

func (m *workerSessionStoreMock) ListByGuild(ctx context.Context, guildID int64) ([]*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*Session, 0)
	for _, s := range m.sessions {
		if s.GuildID == guildID {
			out = append(out, cloneSession(s))
		}
	}
	return out, nil
}

func (m *workerSessionStoreMock) ClaimDueForSummary(ctx context.Context, now time.Time) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.claims++
	if len(m.dueQueue) == 0 {
		return nil, ErrNoSessionsDue
	}
	s := m.dueQueue[0]
	m.dueQueue = m.dueQueue[1:]
	return cloneSession(s), nil
}

func (m *workerSessionStoreMock) MarkStatus(ctx context.Context, id int64, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return nil
	}
	s.Status = status
	m.statusHistory[id] = append(m.statusHistory[id], status)
	return nil
}

func (m *workerSessionStoreMock) SetThreadResult(ctx context.Context, id int64, threadID int64, summaryMsgID int64, title string, summary string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return nil
	}
	s.Status = "thread_created"
	s.ThreadID = &threadID
	s.SummaryMessageID = &summaryMsgID
	s.Title = &title
	s.Summary = &summary
	m.statusHistory[id] = append(m.statusHistory[id], "thread_created")
	return nil
}

func (m *workerSessionStoreMock) IncrementRetry(ctx context.Context, id int64, lastErr string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if ok {
		s.RetryCount++
	}
	m.retryErrors[id] = append(m.retryErrors[id], lastErr)
	return nil
}

type workerAnchorStoreMock struct {
	mu          sync.Mutex
	createCount int
	anchors     []*ThreadAnchor
}

func (m *workerAnchorStoreMock) Create(ctx context.Context, sessionID, threadID, messageID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createCount++
	m.anchors = append(m.anchors, &ThreadAnchor{SessionID: sessionID, ThreadID: threadID, MessageID: messageID})
	return nil
}

func (m *workerAnchorStoreMock) GetBySessionID(ctx context.Context, sessionID int64) (threadID, messageID int64, err error) {
	return 0, 0, nil
}

func (m *workerAnchorStoreMock) GetByThreadID(ctx context.Context, threadID int64) (sessionID int64, err error) {
	return 0, nil
}

func (m *workerAnchorStoreMock) CreateAnchor(ctx context.Context, anchor *ThreadAnchor) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createCount++
	m.anchors = append(m.anchors, &ThreadAnchor{
		SessionID: anchor.SessionID,
		GuildID:   anchor.GuildID,
		ChannelID: anchor.ChannelID,
		ThreadID:  anchor.ThreadID,
		MessageID: anchor.MessageID,
		Title:     anchor.Title,
		Summary:   anchor.Summary,
	})
	return nil
}

type workerGuildSettingsStoreMock struct {
	enabled bool
	err     error
}

func (m *workerGuildSettingsStoreMock) GetLoreThreadEnabled(ctx context.Context, guildID int64) (bool, error) {
	if m.err != nil {
		return false, m.err
	}
	return m.enabled, nil
}

func (m *workerGuildSettingsStoreMock) SetLoreThreadEnabled(ctx context.Context, guildID int64, enabled bool) error {
	m.enabled = enabled
	return nil
}

type workerSummarizerMock struct {
	result *SummaryResult
	err    error
}

func (m *workerSummarizerMock) Summarize(ctx context.Context, req *SummaryRequest) (*SummaryResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.result == nil {
		return &SummaryResult{Summary: "summary"}, nil
	}
	return m.result, nil
}

type workerTitleGeneratorMock struct {
	title string
	err   error
}

func (m *workerTitleGeneratorMock) Generate(ctx context.Context, guildID int64, messages []*Message) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	if m.title == "" {
		return "Generated title", nil
	}
	return m.title, nil
}

type workerThreadCreatorMock struct {
	mu     sync.Mutex
	err    error
	result *ThreadCreateResult
	calls  int
}

func (m *workerThreadCreatorMock) Create(ctx context.Context, req *ThreadCreateRequest) (*ThreadCreateResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	if m.err != nil {
		return nil, m.err
	}
	if m.result == nil {
		return &ThreadCreateResult{ThreadID: 7001, MessageID: 8001}, nil
	}
	return m.result, nil
}

func (m *workerThreadCreatorMock) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

type workerMessageFetcherMock struct {
	recent  []*Message
	byID    map[int64]*Message
	err     error
	byIDErr error
}

func (m *workerMessageFetcherMock) FetchRecent(ctx context.Context, guildID, channelID int64, limit int) ([]*Message, error) {
	if m.err != nil {
		return nil, m.err
	}
	out := make([]*Message, 0, len(m.recent))
	for _, msg := range m.recent {
		out = append(out, cloneMessage(msg))
	}
	return out, nil
}

func (m *workerMessageFetcherMock) FetchByID(ctx context.Context, guildID, messageID int64) (*Message, error) {
	if m.byIDErr != nil {
		return nil, m.byIDErr
	}
	if m.byID == nil {
		return nil, nil
	}
	msg := m.byID[messageID]
	if msg == nil {
		return nil, nil
	}
	return cloneMessage(msg), nil
}

type workerLimiterMock struct {
	allow bool
}

func (m *workerLimiterMock) Allow(ctx context.Context, guildID int64) bool {
	return m.allow
}

func (m *workerLimiterMock) Reset(ctx context.Context, guildID int64) error {
	return nil
}

type workerClassifierMock struct {
	isLore bool
	err    error
}

func (m *workerClassifierMock) Classify(ctx context.Context, guildID int64, message *Message) (*ClassifyResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &ClassifyResult{IsLore: m.isLore}, nil
}

func TestWorkerRunOnce_HappyPath(t *testing.T) {
	now := time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC)
	parent := &Message{ID: 101, GuildID: 123, ChannelID: 456, AuthorID: 1, Content: "lore start", CreatedAt: now.Add(-4 * time.Minute)}
	second := &Message{ID: 102, GuildID: 123, ChannelID: 456, AuthorID: 2, Content: "detail lore", CreatedAt: now.Add(-3 * time.Minute)}

	session := &Session{
		ID:                 1,
		GuildID:            123,
		ChannelID:          456,
		FirstLoreMessageID: parent.ID,
		LastLoreMessageAt:  now,
		RetryCount:         0,
		Status:             "open",
	}

	store := newWorkerSessionStoreMock(session)
	anchorStore := &workerAnchorStoreMock{}
	creator := &workerThreadCreatorMock{result: &ThreadCreateResult{ThreadID: 9001, MessageID: 9002}}

	worker := &Worker{
		SessionStore:       store,
		ThreadAnchorStore:  anchorStore,
		GuildSettingsStore: &workerGuildSettingsStoreMock{enabled: true},
		LoreSummarizer:     &workerSummarizerMock{result: &SummaryResult{Summary: "ringkasan lore", Title: ""}},
		TitleGenerator:     &workerTitleGeneratorMock{title: "Judul Lore"},
		ThreadCreator:      creator,
		MessageFetcher:     &workerMessageFetcherMock{recent: []*Message{parent, second}, byID: map[int64]*Message{parent.ID: parent}},
		Clock:              NewFakeClock(now),
		Limiter:            &workerLimiterMock{allow: true},
		LLMTimeout:         2 * time.Second,
		MaxRetries:         3,
	}

	err := worker.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce failed: %v", err)
	}

	if creator.callCount() != 1 {
		t.Fatalf("expected exactly one thread creation call, got %d", creator.callCount())
	}

	if anchorStore.createCount != 1 {
		t.Fatalf("expected one anchor to be created, got %d", anchorStore.createCount)
	}

	stored, _ := store.GetByID(context.Background(), session.ID)
	if stored == nil {
		t.Fatal("expected stored session")
	}
	if stored.Status != "thread_created" {
		t.Fatalf("expected status thread_created, got %q", stored.Status)
	}
	if stored.ThreadID == nil || *stored.ThreadID != 9001 {
		t.Fatalf("expected thread_id 9001, got %+v", stored.ThreadID)
	}
}

func TestWorkerRunOnce_ConcurrencyOneClaim(t *testing.T) {
	now := time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC)
	parent := &Message{ID: 301, GuildID: 123, ChannelID: 456, AuthorID: 1, Content: "lore", CreatedAt: now.Add(-4 * time.Minute)}
	session := &Session{ID: 99, GuildID: 123, ChannelID: 456, FirstLoreMessageID: parent.ID, LastLoreMessageAt: now, Status: "open"}

	store := newWorkerSessionStoreMock(session)
	anchorStore := &workerAnchorStoreMock{}
	creator := &workerThreadCreatorMock{}
	fetcher := &workerMessageFetcherMock{recent: []*Message{parent}, byID: map[int64]*Message{parent.ID: parent}}

	newWorker := func() *Worker {
		return &Worker{
			SessionStore:       store,
			ThreadAnchorStore:  anchorStore,
			GuildSettingsStore: &workerGuildSettingsStoreMock{enabled: true},
			LoreSummarizer:     &workerSummarizerMock{result: &SummaryResult{Summary: "summary"}},
			TitleGenerator:     &workerTitleGeneratorMock{title: "title"},
			ThreadCreator:      creator,
			MessageFetcher:     fetcher,
			Clock:              NewFakeClock(now),
			Limiter:            &workerLimiterMock{allow: true},
			LLMTimeout:         2 * time.Second,
			MaxRetries:         3,
		}
	}

	w1 := newWorker()
	w2 := newWorker()

	errCh := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		errCh <- w1.RunOnce(context.Background())
	}()
	go func() {
		defer wg.Done()
		errCh <- w2.RunOnce(context.Background())
	}()
	wg.Wait()
	close(errCh)

	var noDueCount int
	for err := range errCh {
		if err == nil {
			continue
		}
		if errors.Is(err, ErrNoSessionsDue) {
			noDueCount++
			continue
		}
		t.Fatalf("unexpected error: %v", err)
	}

	if noDueCount != 1 {
		t.Fatalf("expected one ErrNoSessionsDue, got %d", noDueCount)
	}
	if creator.callCount() != 1 {
		t.Fatalf("expected one thread creation, got %d", creator.callCount())
	}
}

func TestWorkerRunOnce_RateCapRequeuesOpen(t *testing.T) {
	now := time.Now().UTC()
	parent := &Message{ID: 401, GuildID: 123, ChannelID: 456, CreatedAt: now.Add(-2 * time.Minute)}
	session := &Session{ID: 11, GuildID: 123, ChannelID: 456, FirstLoreMessageID: parent.ID, LastLoreMessageAt: now, Status: "open"}

	store := newWorkerSessionStoreMock(session)
	creator := &workerThreadCreatorMock{}

	worker := &Worker{
		SessionStore:       store,
		ThreadAnchorStore:  &workerAnchorStoreMock{},
		GuildSettingsStore: &workerGuildSettingsStoreMock{enabled: true},
		LoreSummarizer:     &workerSummarizerMock{result: &SummaryResult{Summary: "summary"}},
		TitleGenerator:     &workerTitleGeneratorMock{title: "title"},
		ThreadCreator:      creator,
		MessageFetcher:     &workerMessageFetcherMock{recent: []*Message{parent}, byID: map[int64]*Message{parent.ID: parent}},
		Clock:              NewFakeClock(now),
		Limiter:            &workerLimiterMock{allow: false},
		MaxRetries:         3,
	}

	if err := worker.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce should not fail on cap, got %v", err)
	}
	if creator.callCount() != 0 {
		t.Fatalf("expected no thread creation when capped, got %d", creator.callCount())
	}

	stored, _ := store.GetByID(context.Background(), session.ID)
	if stored.Status != "open" {
		t.Fatalf("expected status open after cap, got %q", stored.Status)
	}
}

func TestWorkerRunOnce_DisabledGuildMarksSkipped(t *testing.T) {
	now := time.Now().UTC()
	parent := &Message{ID: 501, GuildID: 123, ChannelID: 456, CreatedAt: now.Add(-2 * time.Minute)}
	session := &Session{ID: 12, GuildID: 123, ChannelID: 456, FirstLoreMessageID: parent.ID, LastLoreMessageAt: now, Status: "open"}

	store := newWorkerSessionStoreMock(session)
	creator := &workerThreadCreatorMock{}

	worker := &Worker{
		SessionStore:       store,
		ThreadAnchorStore:  &workerAnchorStoreMock{},
		GuildSettingsStore: &workerGuildSettingsStoreMock{enabled: false},
		LoreSummarizer:     &workerSummarizerMock{result: &SummaryResult{Summary: "summary"}},
		TitleGenerator:     &workerTitleGeneratorMock{title: "title"},
		ThreadCreator:      creator,
		MessageFetcher:     &workerMessageFetcherMock{recent: []*Message{parent}, byID: map[int64]*Message{parent.ID: parent}},
		Clock:              NewFakeClock(now),
		Limiter:            &workerLimiterMock{allow: true},
		MaxRetries:         3,
	}

	if err := worker.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce should not fail when guild disabled, got %v", err)
	}
	if creator.callCount() != 0 {
		t.Fatalf("expected no thread creation when disabled, got %d", creator.callCount())
	}

	stored, _ := store.GetByID(context.Background(), session.ID)
	if stored.Status != "skipped" {
		t.Fatalf("expected status skipped, got %q", stored.Status)
	}
}

func TestWorkerRunOnce_TransientFailuresIncrementRetryAndFailAtLimit(t *testing.T) {
	now := time.Now().UTC()
	parent := &Message{ID: 601, GuildID: 123, ChannelID: 456, CreatedAt: now.Add(-3 * time.Minute)}

	tests := []struct {
		name            string
		classifierErr   error
		summarizerErr   error
		threadCreateErr error
	}{
		{name: "classifier error", classifierErr: errors.New("classifier timeout")},
		{name: "summarizer error", summarizerErr: errors.New("summary provider down")},
		{name: "thread creator error", threadCreateErr: errors.New("discord failure")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			session := &Session{ID: 13, GuildID: 123, ChannelID: 456, FirstLoreMessageID: parent.ID, LastLoreMessageAt: now, RetryCount: 2, Status: "open"}
			store := newWorkerSessionStoreMock(session)

			worker := &Worker{
				SessionStore:       store,
				ThreadAnchorStore:  &workerAnchorStoreMock{},
				GuildSettingsStore: &workerGuildSettingsStoreMock{enabled: true},
				LoreSummarizer:     &workerSummarizerMock{result: &SummaryResult{Summary: "summary"}, err: tc.summarizerErr},
				TitleGenerator:     &workerTitleGeneratorMock{title: "title"},
				ThreadCreator:      &workerThreadCreatorMock{err: tc.threadCreateErr},
				MessageFetcher:     &workerMessageFetcherMock{recent: []*Message{parent}, byID: map[int64]*Message{parent.ID: parent}},
				Clock:              NewFakeClock(now),
				Limiter:            &workerLimiterMock{allow: true},
				MaxRetries:         3,
				LoreClassifier:     &workerClassifierMock{isLore: true, err: tc.classifierErr},
			}

			err := worker.RunOnce(context.Background())
			if err == nil {
				t.Fatal("expected error")
			}

			stored, _ := store.GetByID(context.Background(), session.ID)
			if stored.RetryCount != 3 {
				t.Fatalf("expected retry_count 3, got %d", stored.RetryCount)
			}
			if stored.Status != "failed" {
				t.Fatalf("expected failed status at retry limit, got %q", stored.Status)
			}
			if len(store.retryErrors[session.ID]) != 1 {
				t.Fatalf("expected one retry increment, got %d", len(store.retryErrors[session.ID]))
			}
		})
	}
}

func TestWorkerRunOnce_ContextCancellation(t *testing.T) {
	store := newWorkerSessionStoreMock()
	worker := &Worker{
		SessionStore:       store,
		ThreadAnchorStore:  &workerAnchorStoreMock{},
		GuildSettingsStore: &workerGuildSettingsStoreMock{enabled: true},
		LoreSummarizer:     &workerSummarizerMock{result: &SummaryResult{Summary: "summary"}},
		TitleGenerator:     &workerTitleGeneratorMock{title: "title"},
		ThreadCreator:      &workerThreadCreatorMock{},
		MessageFetcher:     &workerMessageFetcherMock{},
		Clock:              NewFakeClock(time.Now().UTC()),
		Limiter:            &workerLimiterMock{allow: true},
		MaxRetries:         3,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := worker.RunOnce(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	if store.claims != 0 {
		t.Fatalf("expected no claim attempt after cancellation, got %d", store.claims)
	}
}

func cloneSession(s *Session) *Session {
	if s == nil {
		return nil
	}
	cloned := *s
	if s.FirstMessage != nil {
		cloned.FirstMessage = cloneMessage(s.FirstMessage)
	}
	if len(s.Messages) > 0 {
		cloned.Messages = make([]*Message, 0, len(s.Messages))
		for _, msg := range s.Messages {
			cloned.Messages = append(cloned.Messages, cloneMessage(msg))
		}
	}
	return &cloned
}

func cloneMessage(m *Message) *Message {
	if m == nil {
		return nil
	}
	cloned := *m
	return &cloned
}
