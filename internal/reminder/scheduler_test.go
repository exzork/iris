package reminder

import (
	"context"
	"sync"
	"testing"
	"time"
)

type FakeSender struct {
	mu       sync.Mutex
	messages []SentMessage
}

type SentMessage struct {
	GuildID   int64
	ChannelID int64
	Content   string
}

func (fs *FakeSender) Send(ctx context.Context, guildID, channelID int64, content string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.messages = append(fs.messages, SentMessage{
		GuildID:   guildID,
		ChannelID: channelID,
		Content:   content,
	})
	return nil
}

func (fs *FakeSender) GetMessages() []SentMessage {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return append([]SentMessage{}, fs.messages...)
}

func TestTickOnceFiresDue(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStore()
	clock := NewFakeClock(time.Date(2026, 5, 11, 10, 0, 0, 0, time.UTC))
	sender := &FakeSender{}
	scheduler := NewScheduler(store, clock, sender)

	now := clock.Now()
	past := now.Add(-1 * time.Hour)

	r := &Reminder{
		GuildID:   123,
		ChannelID: 456,
		CreatedBy: 789,
		Kind:      KindDaily,
		Message:   "Test message",
		Timezone:  "UTC",
		HourMin:   "09:00",
		NextRun:   past,
	}

	id, _ := store.Create(ctx, r)

	err := scheduler.TickOnce(ctx)
	if err != nil {
		t.Fatalf("TickOnce failed: %v", err)
	}

	messages := sender.GetMessages()
	if len(messages) != 1 {
		t.Errorf("TickOnce sent %d messages, want 1", len(messages))
	}
	if messages[0].GuildID != 123 || messages[0].ChannelID != 456 {
		t.Errorf("TickOnce sent to wrong guild/channel: %+v", messages[0])
	}

	updated, _ := store.Get(ctx, id)
	if updated.NextRun.Before(now) {
		t.Errorf("TickOnce did not update NextRun for daily reminder")
	}
}

func TestTickOnceAdvancesDailyNextRun(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStore()
	clock := NewFakeClock(time.Date(2026, 5, 11, 10, 0, 0, 0, time.UTC))
	sender := &FakeSender{}
	scheduler := NewScheduler(store, clock, sender)

	now := clock.Now()
	past := now.Add(-1 * time.Hour)

	r := &Reminder{
		GuildID:   123,
		ChannelID: 456,
		CreatedBy: 789,
		Kind:      KindDaily,
		Message:   "Daily reminder",
		Timezone:  "UTC",
		HourMin:   "10:00",
		NextRun:   past,
	}

	id, _ := store.Create(ctx, r)

	scheduler.TickOnce(ctx)

	updated, _ := store.Get(ctx, id)
	if updated.NextRun.Day() != now.Day()+1 {
		t.Errorf("Daily reminder NextRun not advanced to next day: %v", updated.NextRun)
	}
}

func TestTickOnceDeletesOnceReminder(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStore()
	clock := NewFakeClock(time.Date(2026, 5, 11, 10, 0, 0, 0, time.UTC))
	sender := &FakeSender{}
	scheduler := NewScheduler(store, clock, sender)

	now := clock.Now()
	past := now.Add(-1 * time.Hour)

	r := &Reminder{
		GuildID:   123,
		ChannelID: 456,
		CreatedBy: 789,
		Kind:      KindOnce,
		Message:   "One-time reminder",
		Timezone:  "UTC",
		HourMin:   "09:00",
		NextRun:   past,
	}

	id, _ := store.Create(ctx, r)

	scheduler.TickOnce(ctx)

	_, err := store.Get(ctx, id)
	if err != ErrNotFound {
		t.Errorf("Once reminder not deleted after firing: %v", err)
	}
}
