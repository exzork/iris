package reminder

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestCreateValidatesInputs(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStore()
	clock := NewFakeClock(time.Date(2026, 5, 11, 10, 0, 0, 0, time.UTC))
	sender := &FakeSender{}
	service := NewService(store, clock, sender)

	tests := []struct {
		name    string
		input   CreateInput
		wantErr error
	}{
		{
			name: "invalid channel",
			input: CreateInput{
				GuildID:   123,
				ChannelID: 0,
				CreatedBy: 789,
				Kind:      KindDaily,
				Message:   "Test",
				Timezone:  "UTC",
				HourMin:   "10:00",
			},
			wantErr: ErrInvalidChannel,
		},
		{
			name: "empty message",
			input: CreateInput{
				GuildID:   123,
				ChannelID: 456,
				CreatedBy: 789,
				Kind:      KindDaily,
				Message:   "",
				Timezone:  "UTC",
				HourMin:   "10:00",
			},
			wantErr: ErrInvalidMessage,
		},
		{
			name: "invalid timezone",
			input: CreateInput{
				GuildID:   123,
				ChannelID: 456,
				CreatedBy: 789,
				Kind:      KindDaily,
				Message:   "Test",
				Timezone:  "Invalid/Zone",
				HourMin:   "10:00",
			},
			wantErr: ErrInvalidTimezone,
		},
		{
			name: "invalid hour:min",
			input: CreateInput{
				GuildID:   123,
				ChannelID: 456,
				CreatedBy: 789,
				Kind:      KindDaily,
				Message:   "Test",
				Timezone:  "UTC",
				HourMin:   "25:00",
			},
			wantErr: ErrInvalidHourMin,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := service.Create(ctx, tt.input)
			if err != tt.wantErr {
				t.Errorf("Create returned %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestReminderFiresInConfiguredChannel(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStore()

	jakartaLoc, _ := time.LoadLocation("Asia/Jakarta")
	baseTime := time.Date(2026, 5, 11, 9, 59, 59, 0, jakartaLoc)
	baseTimeUTC := baseTime.UTC()

	clock := NewFakeClock(baseTimeUTC)
	sender := &FakeSender{}
	service := NewService(store, clock, sender)

	input := CreateInput{
		GuildID:   123,
		ChannelID: 456,
		CreatedBy: 789,
		Kind:      KindDaily,
		Message:   "Pengingat harian",
		Timezone:  "Asia/Jakarta",
		HourMin:   "10:00",
	}

	reminder, err := service.Create(ctx, input)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	clock.Advance(2 * time.Second)

	err = service.Scheduler.TickOnce(ctx)
	if err != nil {
		t.Fatalf("TickOnce failed: %v", err)
	}

	messages := sender.GetMessages()
	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}

	msg := messages[0]
	if msg.GuildID != 123 {
		t.Errorf("Message GuildID = %d, want 123", msg.GuildID)
	}
	if msg.ChannelID != 456 {
		t.Errorf("Message ChannelID = %d, want 456", msg.ChannelID)
	}
	if msg.Content != "Pengingat harian" {
		t.Errorf("Message Content = %q, want %q", msg.Content, "Pengingat harian")
	}

	output := fmt.Sprintf("PASS: Reminder fired in configured channel\n")
	output += fmt.Sprintf("  Guild: %d, Channel: %d\n", msg.GuildID, msg.ChannelID)
	output += fmt.Sprintf("  Message: %s\n", msg.Content)
	output += fmt.Sprintf("  Reminder ID: %d\n", reminder.ID)

	fmt.Println(output)
}

func TestDeletedReminderDoesNotFire(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStore()

	jakartaLoc, _ := time.LoadLocation("Asia/Jakarta")
	baseTime := time.Date(2026, 5, 11, 9, 59, 59, 0, jakartaLoc)
	baseTimeUTC := baseTime.UTC()

	clock := NewFakeClock(baseTimeUTC)
	sender := &FakeSender{}
	service := NewService(store, clock, sender)

	input := CreateInput{
		GuildID:   123,
		ChannelID: 456,
		CreatedBy: 789,
		Kind:      KindDaily,
		Message:   "Pengingat harian",
		Timezone:  "Asia/Jakarta",
		HourMin:   "10:00",
	}

	reminder, err := service.Create(ctx, input)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	err = service.Delete(ctx, 123, reminder.ID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	clock.Advance(2 * time.Second)

	err = service.Scheduler.TickOnce(ctx)
	if err != nil {
		t.Fatalf("TickOnce failed: %v", err)
	}

	messages := sender.GetMessages()
	if len(messages) != 0 {
		t.Fatalf("Expected 0 messages after delete, got %d", len(messages))
	}

	output := fmt.Sprintf("PASS: Deleted reminder did not fire\n")
	output += fmt.Sprintf("  Reminder ID: %d was deleted\n", reminder.ID)
	output += fmt.Sprintf("  Messages sent: %d (expected 0)\n", len(messages))

	fmt.Println(output)
}

func TestGuildScopingOnList(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStore()
	clock := NewFakeClock(time.Date(2026, 5, 11, 10, 0, 0, 0, time.UTC))
	sender := &FakeSender{}
	service := NewService(store, clock, sender)

	input1 := CreateInput{
		GuildID:   111,
		ChannelID: 1,
		CreatedBy: 1,
		Kind:      KindDaily,
		Message:   "Guild 1 reminder",
		Timezone:  "UTC",
		HourMin:   "10:00",
	}

	input2 := CreateInput{
		GuildID:   222,
		ChannelID: 2,
		CreatedBy: 2,
		Kind:      KindDaily,
		Message:   "Guild 2 reminder",
		Timezone:  "UTC",
		HourMin:   "10:00",
	}

	service.Create(ctx, input1)
	service.Create(ctx, input2)

	list1, err := service.List(ctx, 111)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list1) != 1 {
		t.Errorf("Guild 111 list = %d reminders, want 1", len(list1))
	}

	list2, err := service.List(ctx, 222)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list2) != 1 {
		t.Errorf("Guild 222 list = %d reminders, want 1", len(list2))
	}
}
