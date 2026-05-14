package lorethread

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestE2E_FakesIntegration(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC)

	sessionStore := NewFakeSessionStore()
	anchorStore := NewFakeThreadAnchorStore()
	settingsStore := NewFakeGuildSettingsStore()
	classifier := NewFakeLoreClassifier(func(ctx context.Context, guildID int64, msg *Message) (*ClassifyResult, error) {
		return &ClassifyResult{IsLore: true, Reason: "test"}, nil
	})
	summarizer := NewFakeLoreSummarizer(func(ctx context.Context, req *SummaryRequest) (*SummaryResult, error) {
		return &SummaryResult{Title: "Test", Summary: "Summary"}, nil
	})
	titleGen := NewFakeTitleGenerator(func(ctx context.Context, guildID int64, messages []*Message) (string, error) {
		return "Test Title", nil
	})
	threadCreator := NewFakeThreadCreator(func(ctx context.Context, req *ThreadCreateRequest) (*ThreadCreateResult, error) {
		return &ThreadCreateResult{ThreadID: 999, MessageID: 888}, nil
	})
	messageFetcher := NewFakeMessageFetcher()
	limiter := NewFakeLimiter()

	msg := &Message{ID: 100, GuildID: 111, ChannelID: 222, AuthorID: 333, Content: "Test", CreatedAt: now}
	session := &Session{
		ID:        1,
		GuildID:   111,
		ChannelID: 222,
		Status:    "open",
		Messages:  []*Message{msg},
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := sessionStore.Create(ctx, session); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	retrieved, err := sessionStore.GetByID(ctx, 1)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("Expected session, got nil")
	}
	if retrieved.Status != "open" {
		t.Errorf("Expected status 'open', got %q", retrieved.Status)
	}

	if err := anchorStore.Create(ctx, 1, 999, 888); err != nil {
		t.Fatalf("Anchor create failed: %v", err)
	}

	threadID, msgID, err := anchorStore.GetBySessionID(ctx, 1)
	if err != nil {
		t.Fatalf("GetBySessionID failed: %v", err)
	}
	if threadID != 999 || msgID != 888 {
		t.Errorf("Expected threadID=999, msgID=888, got %d, %d", threadID, msgID)
	}

	settingsStore.SetLoreThreadEnabled(ctx, 111, true)
	enabled, err := settingsStore.GetLoreThreadEnabled(ctx, 111)
	if err != nil {
		t.Fatalf("GetLoreThreadEnabled failed: %v", err)
	}
	if !enabled {
		t.Error("Expected lore threads enabled")
	}

	result, err := classifier.Classify(ctx, 111, msg)
	if err != nil {
		t.Fatalf("Classify failed: %v", err)
	}
	if !result.IsLore {
		t.Error("Expected IsLore=true")
	}

	summary, err := summarizer.Summarize(ctx, &SummaryRequest{GuildID: 111, Messages: []*Message{msg}})
	if err != nil {
		t.Fatalf("Summarize failed: %v", err)
	}
	if summary.Title != "Test" {
		t.Errorf("Expected title 'Test', got %q", summary.Title)
	}

	title, err := titleGen.Generate(ctx, 111, []*Message{msg})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if title != "Test Title" {
		t.Errorf("Expected 'Test Title', got %q", title)
	}

	threadResult, err := threadCreator.Create(ctx, &ThreadCreateRequest{
		GuildID:         111,
		ChannelID:       222,
		ParentMessageID: 100,
		Title:           "Test",
		FirstMessage:    "Summary",
	})
	if err != nil {
		t.Fatalf("Create thread failed: %v", err)
	}
	if threadResult.ThreadID != 999 {
		t.Errorf("Expected threadID 999, got %d", threadResult.ThreadID)
	}
	if threadCreator.CreatedCount() != 1 {
		t.Errorf("Expected 1 thread created, got %d", threadCreator.CreatedCount())
	}

	messageFetcher.AddMessage(msg)
	fetched, err := messageFetcher.FetchByID(ctx, 111, 100)
	if err != nil {
		t.Fatalf("FetchByID failed: %v", err)
	}
	if fetched == nil || fetched.ID != 100 {
		t.Error("Expected message to be fetched")
	}

	limiter.SetAllowed(111, 5)
	if !limiter.Allow(ctx, 111) {
		t.Error("Expected limiter to allow")
	}
}

func TestE2E_SessionStoreExtendedInterface(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC)

	store := NewFakeSessionStore()

	session := &Session{
		ID:           1,
		GuildID:      111,
		ChannelID:    222,
		Status:       "open",
		IdleDeadline: now.Add(-1 * time.Minute),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	store.Create(ctx, session)

	claimed, err := store.ClaimDueForSummary(ctx, now)
	if err != nil {
		t.Fatalf("ClaimDueForSummary failed: %v", err)
	}
	if claimed == nil {
		t.Fatal("Expected claimed session")
	}
	if claimed.Status != "summarizing" {
		t.Errorf("Expected status 'summarizing', got %q", claimed.Status)
	}

	store.SetThreadResult(ctx, 1, 999, 888, "Title", "Summary")
	updated, _ := store.GetByID(ctx, 1)
	if updated.Status != "thread_created" {
		t.Errorf("Expected status 'thread_created', got %q", updated.Status)
	}
	if updated.ThreadID == nil || *updated.ThreadID != 999 {
		t.Errorf("Expected threadID 999, got %v", updated.ThreadID)
	}

	store.IncrementRetry(ctx, 1, "test error")
	updated, _ = store.GetByID(ctx, 1)
	if updated.RetryCount != 1 {
		t.Errorf("Expected retry count 1, got %d", updated.RetryCount)
	}
}

func TestE2E_FakeClockDeterminism(t *testing.T) {
	now := time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC)
	clock := NewFakeClock(now)

	if clock.Now() != now {
		t.Errorf("Expected %v, got %v", now, clock.Now())
	}

	ch := clock.After(5 * time.Minute)
	select {
	case <-ch:
		t.Error("Timer should not fire before advance")
	default:
	}

	clock.Advance(5 * time.Minute)
	select {
	case <-ch:
	default:
		t.Error("Timer should fire after advance")
	}

	if clock.Now() != now.Add(5*time.Minute) {
		t.Errorf("Expected %v, got %v", now.Add(5*time.Minute), clock.Now())
	}
}

func TestE2E_LimiterRateCapping(t *testing.T) {
	ctx := context.Background()
	limiter := NewFakeLimiter()

	limiter.SetAllowed(111, 3)

	if !limiter.Allow(ctx, 111) {
		t.Error("First allow should succeed")
	}
	if !limiter.Allow(ctx, 111) {
		t.Error("Second allow should succeed")
	}
	if !limiter.Allow(ctx, 111) {
		t.Error("Third allow should succeed")
	}
	if limiter.Allow(ctx, 111) {
		t.Error("Fourth allow should fail")
	}

	limiter.Reset(ctx, 111)
	if !limiter.Allow(ctx, 111) {
		t.Error("Allow should succeed after reset")
	}
}

func TestE2E_ClassifierWithCustomLogic(t *testing.T) {
	ctx := context.Background()

	callCount := 0
	classifier := NewFakeLoreClassifier(func(ctx context.Context, guildID int64, msg *Message) (*ClassifyResult, error) {
		callCount++
		if msg.Content == "error" {
			return nil, errors.New("test error")
		}
		return &ClassifyResult{IsLore: msg.Content == "lore"}, nil
	})

	msg1 := &Message{ID: 1, Content: "lore"}
	result1, _ := classifier.Classify(ctx, 111, msg1)
	if !result1.IsLore {
		t.Error("Expected lore classification")
	}

	msg2 := &Message{ID: 2, Content: "not lore"}
	result2, _ := classifier.Classify(ctx, 111, msg2)
	if result2.IsLore {
		t.Error("Expected non-lore classification")
	}

	msg3 := &Message{ID: 3, Content: "error"}
	_, err := classifier.Classify(ctx, 111, msg3)
	if err == nil {
		t.Error("Expected error")
	}

	if callCount != 3 {
		t.Errorf("Expected 3 calls, got %d", callCount)
	}
}

func TestE2E_SummarizerWithRedaction(t *testing.T) {
	ctx := context.Background()

	summarizer := NewFakeLoreSummarizer(func(ctx context.Context, req *SummaryRequest) (*SummaryResult, error) {
		if len(req.Messages) == 0 {
			return nil, errors.New("no messages")
		}
		return &SummaryResult{
			Title:   "Summary",
			Summary: "Processed messages",
		}, nil
	})

	result, err := summarizer.Summarize(ctx, &SummaryRequest{
		GuildID:  111,
		Messages: []*Message{{ID: 1, Content: "msg1"}, {ID: 2, Content: "msg2"}},
	})
	if err != nil {
		t.Fatalf("Summarize failed: %v", err)
	}
	if result.Title != "Summary" {
		t.Errorf("Expected title 'Summary', got %q", result.Title)
	}

	_, err = summarizer.Summarize(ctx, &SummaryRequest{GuildID: 111, Messages: []*Message{}})
	if err == nil {
		t.Error("Expected error for empty messages")
	}
}
