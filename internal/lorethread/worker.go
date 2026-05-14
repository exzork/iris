package lorethread

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	defaultWorkerPollInterval = 30 * time.Second
	defaultWorkerLLMTimeout   = 30 * time.Second
	defaultWorkerMaxRetries   = 3
	workerMessageFetchLimit   = 200
)

// dueSessionStore is the extended session-store contract needed by the idle worker.
type dueSessionStore interface {
	ClaimDueForSummary(ctx context.Context, now time.Time) (*Session, error)
	MarkStatus(ctx context.Context, id int64, status string) error
	SetThreadResult(ctx context.Context, id int64, threadID int64, summaryMsgID int64, title string, summary string) error
	IncrementRetry(ctx context.Context, id int64, lastErr string) error
}

// threadAnchorMetadataStore allows richer anchor persistence when available.
type threadAnchorMetadataStore interface {
	CreateAnchor(ctx context.Context, anchor *ThreadAnchor) error
}

// Worker processes idle lore sessions into summary threads.
type Worker struct {
	SessionStore       SessionStore
	ThreadAnchorStore  ThreadAnchorStore
	GuildSettingsStore GuildSettingsStore
	LoreSummarizer     LoreSummarizer
	TitleGenerator     TitleGenerator
	ThreadCreator      ThreadCreator
	MessageFetcher     MessageFetcher
	Clock              Clock
	Limiter            Limiter

	// Optional classifier. If set, fetched messages are filtered to lore messages before summary.
	LoreClassifier LoreClassifier

	PollInterval     time.Duration
	LLMTimeout       time.Duration
	MaxRetries       int
	ThreadCapPerHour int
	MetricsHooks     *MetricsHooks

	done      chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
	initOnce  sync.Once
	wg        sync.WaitGroup
}

func (w *Worker) initDefaults() {
	w.initOnce.Do(func() {
		if w.PollInterval <= 0 {
			w.PollInterval = defaultWorkerPollInterval
		}
		// LLMTimeout: 0 means no deadline (only parent ctx cancellation).
		// Do NOT substitute 0 with defaultWorkerLLMTimeout.
		if w.LLMTimeout < 0 {
			w.LLMTimeout = defaultWorkerLLMTimeout
		}
		if w.MaxRetries <= 0 {
			w.MaxRetries = defaultWorkerMaxRetries
		}
		if w.Clock == nil {
			w.Clock = &RealClock{}
		}
		if w.MetricsHooks == nil {
			w.MetricsHooks = NoOpMetricsHooks()
		}
		if w.done == nil {
			w.done = make(chan struct{})
		}
	})
}

func (w *Worker) validateDeps() error {
	if w.SessionStore == nil {
		return errors.New("lore worker: SessionStore is required")
	}
	if w.ThreadAnchorStore == nil {
		return errors.New("lore worker: ThreadAnchorStore is required")
	}
	if w.GuildSettingsStore == nil {
		return errors.New("lore worker: GuildSettingsStore is required")
	}
	if w.LoreSummarizer == nil {
		return errors.New("lore worker: LoreSummarizer is required")
	}
	if w.TitleGenerator == nil {
		return errors.New("lore worker: TitleGenerator is required")
	}
	if w.ThreadCreator == nil {
		return errors.New("lore worker: ThreadCreator is required")
	}
	if w.MessageFetcher == nil {
		return errors.New("lore worker: MessageFetcher is required")
	}
	if w.Limiter == nil {
		return errors.New("lore worker: Limiter is required")
	}
	if _, ok := w.SessionStore.(dueSessionStore); !ok {
		return errors.New("lore worker: SessionStore does not support due-claim operations")
	}
	return nil
}

// Start launches the polling goroutine and returns immediately.
func (w *Worker) Start(ctx context.Context) error {
	w.initDefaults()
	if err := w.validateDeps(); err != nil {
		return err
	}

	w.startOnce.Do(func() {
		w.wg.Add(1)
		go w.run(ctx)
	})

	return nil
}

func (w *Worker) run(ctx context.Context) {
	defer w.wg.Done()

	ticker := time.NewTicker(w.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.done:
			return
		case <-ticker.C:
			err := w.RunOnce(ctx)
			if err == nil || errors.Is(err, ErrNoSessionsDue) || errors.Is(err, context.Canceled) {
				continue
			}
		}
	}
}

// Stop closes the done channel and waits until the worker loop exits.
// It is safe to call multiple times.
func (w *Worker) Stop() {
	w.initDefaults()
	w.stopOnce.Do(func() {
		close(w.done)
	})
	w.wg.Wait()
}

// RunOnce claims and processes one due session.
func (w *Worker) RunOnce(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	w.initDefaults()
	if err := w.validateDeps(); err != nil {
		return err
	}

	store := w.SessionStore.(dueSessionStore)
	session, err := store.ClaimDueForSummary(ctx, w.Clock.Now())
	if err != nil {
		if errors.Is(err, ErrNoSessionsDue) {
			return ErrNoSessionsDue
		}
		return err
	}
	if session == nil {
		return ErrNoSessionsDue
	}

	if err := store.MarkStatus(ctx, session.ID, "summarizing"); err != nil {
		return err
	}
	session.Status = "summarizing"

	return w.processSession(ctx, session)
}

func (w *Worker) processSession(ctx context.Context, session *Session) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	store := w.SessionStore.(dueSessionStore)

	if session.GuildID == 0 {
		return store.MarkStatus(ctx, session.ID, "skipped")
	}

	enabled, err := w.GuildSettingsStore.GetLoreThreadEnabled(ctx, session.GuildID)
	if err != nil {
		return w.onProcessError(ctx, store, session, fmt.Errorf("guild settings check failed: %w", err))
	}
	if !enabled {
		w.MetricsHooks.OnSessionSkipped()
		return store.MarkStatus(ctx, session.ID, "skipped")
	}

	if !w.Limiter.Allow(ctx, session.GuildID) {
		w.MetricsHooks.OnRateCapHit()
		return store.MarkStatus(ctx, session.ID, "open")
	}

	parentMessageID := session.FirstLoreMessageID
	if parentMessageID == 0 && session.FirstMessage != nil {
		parentMessageID = session.FirstMessage.ID
	}
	if parentMessageID == 0 {
		return w.onProcessError(ctx, store, session, errors.New("session has no parent message ID"))
	}

	parentMsg, err := w.MessageFetcher.FetchByID(ctx, session.GuildID, parentMessageID)
	if err != nil {
		return w.onProcessError(ctx, store, session, fmt.Errorf("fetch parent message failed: %w", err))
	}
	if parentMsg == nil {
		return w.onProcessError(ctx, store, session, errors.New("parent message not found"))
	}

	recentMessages, err := w.MessageFetcher.FetchRecent(ctx, session.GuildID, session.ChannelID, workerMessageFetchLimit)
	if err != nil {
		return w.onProcessError(ctx, store, session, fmt.Errorf("fetch recent messages failed: %w", err))
	}

	candidates := selectCandidateMessages(parentMsg, recentMessages, session)
	loreMessages, err := w.filterLoreMessages(ctx, session.GuildID, candidates)
	if err != nil {
		return w.onProcessError(ctx, store, session, err)
	}

	title, summary, err := w.generateTitleAndSummary(ctx, session.GuildID, loreMessages)
	if err != nil {
		return w.onProcessError(ctx, store, session, err)
	}

	threadResult, err := w.ThreadCreator.Create(ctx, &ThreadCreateRequest{
		GuildID:         session.GuildID,
		ChannelID:       session.ChannelID,
		ParentMessageID: parentMessageID,
		Title:           title,
		FirstMessage:    summary,
	})
	if err != nil {
		if errors.Is(err, ErrDMNotSupported) {
			w.MetricsHooks.OnSessionSkipped()
			return store.MarkStatus(ctx, session.ID, "skipped")
		}
		return w.onProcessError(ctx, store, session, fmt.Errorf("thread creation failed: %w", err))
	}
	if threadResult == nil {
		return w.onProcessError(ctx, store, session, errors.New("thread creation returned nil result"))
	}

	w.MetricsHooks.OnThreadCreated()

	if err := w.persistAnchor(ctx, session, threadResult, title, summary); err != nil {
		return w.onProcessError(ctx, store, session, fmt.Errorf("anchor persistence failed: %w", err))
	}

	if err := store.SetThreadResult(ctx, session.ID, threadResult.ThreadID, threadResult.MessageID, title, summary); err != nil {
		return w.onProcessError(ctx, store, session, fmt.Errorf("set thread result failed: %w", err))
	}

	return nil
}

func (w *Worker) filterLoreMessages(ctx context.Context, guildID int64, messages []*Message) ([]*Message, error) {
	if len(messages) == 0 {
		return nil, errors.New("no messages available for lore summary")
	}

	if w.LoreClassifier == nil {
		return messages, nil
	}

	loreMessages := make([]*Message, 0, len(messages))
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		result, err := w.LoreClassifier.Classify(ctx, guildID, msg)
		if err != nil {
			w.MetricsHooks.OnClassifierFailure()
			return nil, fmt.Errorf("classify message %d failed: %w", msg.ID, err)
		}
		if result != nil && result.IsLore {
			loreMessages = append(loreMessages, msg)
		}
	}

	if len(loreMessages) == 0 {
		return nil, errors.New("no lore messages after classification")
	}

	return loreMessages, nil
}

func (w *Worker) generateTitleAndSummary(ctx context.Context, guildID int64, messages []*Message) (string, string, error) {
	llmCtx := ctx
	var cancel context.CancelFunc
	if w.LLMTimeout > 0 {
		llmCtx, cancel = context.WithTimeout(ctx, w.LLMTimeout)
		defer cancel()
	}

	var summaryResult *SummaryResult
	var generatedTitle string
	var summaryErr, titleErr error

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		res, err := w.LoreSummarizer.Summarize(llmCtx, &SummaryRequest{GuildID: guildID, Messages: messages})
		if err != nil {
			summaryErr = fmt.Errorf("summarize failed: %w", err)
			return
		}
		summaryResult = res
	}()
	go func() {
		defer wg.Done()
		title, err := w.TitleGenerator.Generate(llmCtx, guildID, messages)
		if err != nil {
			titleErr = fmt.Errorf("title generation failed: %w", err)
			return
		}
		generatedTitle = strings.TrimSpace(title)
	}()
	wg.Wait()

	if summaryErr != nil {
		w.MetricsHooks.OnSummaryFailure()
		return "", "", summaryErr
	}
	if titleErr != nil {
		w.MetricsHooks.OnSummaryFailure()
		return "", "", titleErr
	}

	if summaryResult == nil {
		w.MetricsHooks.OnSummaryFailure()
		return "", "", errors.New("summarizer returned nil result")
	}

	summary := strings.TrimSpace(summaryResult.Summary)
	if summary == "" {
		return "", "", errors.New("summary is empty")
	}

	title := strings.TrimSpace(summaryResult.Title)
	if title == "" {
		title = generatedTitle
	}
	if title == "" {
		title = fmt.Sprintf("Ringkasan Lore — %s", w.Clock.Now().UTC().Format("2006-01-02"))
	}

	return title, summary, nil
}

func (w *Worker) persistAnchor(ctx context.Context, session *Session, threadResult *ThreadCreateResult, title, summary string) error {
	if richStore, ok := w.ThreadAnchorStore.(threadAnchorMetadataStore); ok {
		return richStore.CreateAnchor(ctx, &ThreadAnchor{
			SessionID: session.ID,
			GuildID:   session.GuildID,
			ChannelID: session.ChannelID,
			ThreadID:  threadResult.ThreadID,
			MessageID: threadResult.MessageID,
			Title:     title,
			Summary:   summary,
		})
	}

	return w.ThreadAnchorStore.Create(ctx, session.ID, threadResult.ThreadID, threadResult.MessageID)
}

func (w *Worker) onProcessError(ctx context.Context, store dueSessionStore, session *Session, processErr error) error {
	if processErr == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	errMsg := processErr.Error()
	if len(errMsg) > 1000 {
		errMsg = errMsg[:1000]
	}

	if err := store.IncrementRetry(ctx, session.ID, errMsg); err != nil {
		return errors.Join(processErr, err)
	}

	nextRetry := session.RetryCount + 1
	session.RetryCount = nextRetry
	if nextRetry >= w.MaxRetries {
		if err := store.MarkStatus(ctx, session.ID, "failed"); err != nil {
			return errors.Join(processErr, err)
		}
	}

	return processErr
}

func selectCandidateMessages(parent *Message, recent []*Message, session *Session) []*Message {
	selected := make([]*Message, 0, len(recent)+1)
	seen := map[int64]struct{}{}

	for _, msg := range recent {
		if msg == nil {
			continue
		}
		if !msg.CreatedAt.IsZero() && msg.CreatedAt.Before(parent.CreatedAt) {
			continue
		}
		if !session.LastLoreMessageAt.IsZero() && !msg.CreatedAt.IsZero() && msg.CreatedAt.After(session.LastLoreMessageAt) {
			continue
		}
		if _, ok := seen[msg.ID]; ok {
			continue
		}
		seen[msg.ID] = struct{}{}
		selected = append(selected, msg)
	}

	if _, ok := seen[parent.ID]; !ok {
		selected = append([]*Message{parent}, selected...)
	}

	if len(selected) == 0 {
		return []*Message{parent}
	}

	return selected
}
