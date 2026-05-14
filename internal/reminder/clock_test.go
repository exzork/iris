package reminder

import (
	"testing"
	"time"
)

func TestFakeClockAdvance(t *testing.T) {
	start := time.Date(2026, 5, 11, 10, 0, 0, 0, time.UTC)
	fc := NewFakeClock(start)

	if got := fc.Now(); got != start {
		t.Errorf("Now() = %v, want %v", got, start)
	}

	fc.Advance(2 * time.Second)
	expected := start.Add(2 * time.Second)
	if got := fc.Now(); got != expected {
		t.Errorf("After Advance(2s), Now() = %v, want %v", got, expected)
	}

	fc.Advance(1 * time.Hour)
	expected = expected.Add(1 * time.Hour)
	if got := fc.Now(); got != expected {
		t.Errorf("After Advance(1h), Now() = %v, want %v", got, expected)
	}
}

func TestFakeClockSetTime(t *testing.T) {
	start := time.Date(2026, 5, 11, 10, 0, 0, 0, time.UTC)
	fc := NewFakeClock(start)

	newTime := time.Date(2026, 5, 12, 15, 30, 0, 0, time.UTC)
	fc.SetTime(newTime)

	if got := fc.Now(); got != newTime {
		t.Errorf("After SetTime, Now() = %v, want %v", got, newTime)
	}

	// Test that SetTime converts to UTC
	nonUTC := time.Date(2026, 5, 12, 15, 30, 0, 0, time.FixedZone("test", 3600))
	fc.SetTime(nonUTC)
	if got := fc.Now(); got.Location() != time.UTC {
		t.Errorf("After SetTime with non-UTC, location = %v, want UTC", got.Location())
	}
}
