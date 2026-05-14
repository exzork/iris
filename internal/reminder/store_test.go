package reminder

import (
	"context"
	"testing"
	"time"
)

func TestInMemoryStoreCreateGetListDelete(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStore()

	r := &Reminder{
		GuildID:   123,
		ChannelID: 456,
		CreatedBy: 789,
		Kind:      KindDaily,
		Message:   "Test reminder",
		Timezone:  "Asia/Jakarta",
		HourMin:   "10:00",
		NextRun:   time.Now().UTC(),
	}

	id, err := store.Create(ctx, r)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if id != 1 {
		t.Errorf("Create returned id %d, want 1", id)
	}

	got, err := store.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.ID != id || got.GuildID != 123 {
		t.Errorf("Get returned wrong reminder: %+v", got)
	}

	list, err := store.List(ctx, 123)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("List returned %d reminders, want 1", len(list))
	}

	err = store.Delete(ctx, 123, id)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = store.Get(ctx, id)
	if err != ErrNotFound {
		t.Errorf("Get after delete returned %v, want ErrNotFound", err)
	}
}

func TestInMemoryStoreGuildIsolation(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStore()

	r1 := &Reminder{GuildID: 111, ChannelID: 1, Kind: KindDaily, Message: "Guild 1", Timezone: "UTC", HourMin: "10:00", NextRun: time.Now().UTC()}
	r2 := &Reminder{GuildID: 222, ChannelID: 2, Kind: KindDaily, Message: "Guild 2", Timezone: "UTC", HourMin: "10:00", NextRun: time.Now().UTC()}

	id1, _ := store.Create(ctx, r1)
	id2, _ := store.Create(ctx, r2)

	list1, _ := store.List(ctx, 111)
	if len(list1) != 1 || list1[0].ID != id1 {
		t.Errorf("Guild 111 list = %v, want only reminder %d", list1, id1)
	}

	list2, _ := store.List(ctx, 222)
	if len(list2) != 1 || list2[0].ID != id2 {
		t.Errorf("Guild 222 list = %v, want only reminder %d", list2, id2)
	}

	err := store.Delete(ctx, 222, id1)
	if err != ErrNotFound {
		t.Errorf("Delete with wrong guild returned %v, want ErrNotFound", err)
	}
}

func TestInMemoryStoreDue(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStore()

	now := time.Date(2026, 5, 11, 10, 0, 0, 0, time.UTC)
	past := now.Add(-1 * time.Hour)
	future := now.Add(1 * time.Hour)

	r1 := &Reminder{GuildID: 1, ChannelID: 1, Kind: KindDaily, Message: "Past", Timezone: "UTC", HourMin: "09:00", NextRun: past}
	r2 := &Reminder{GuildID: 1, ChannelID: 1, Kind: KindDaily, Message: "Now", Timezone: "UTC", HourMin: "10:00", NextRun: now}
	r3 := &Reminder{GuildID: 1, ChannelID: 1, Kind: KindDaily, Message: "Future", Timezone: "UTC", HourMin: "11:00", NextRun: future}

	store.Create(ctx, r1)
	store.Create(ctx, r2)
	store.Create(ctx, r3)

	due, err := store.Due(ctx, now)
	if err != nil {
		t.Fatalf("Due failed: %v", err)
	}
	if len(due) != 2 {
		t.Errorf("Due returned %d reminders, want 2 (past and now)", len(due))
	}
}
