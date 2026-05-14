package reminder

import (
	"testing"
	"time"
)

func TestComputeNextRunDaily(t *testing.T) {
	tests := []struct {
		name     string
		tz       string
		hourMin  string
		after    time.Time
		wantHour int
		wantMin  int
	}{
		{
			name:     "daily at 10:00 Asia/Jakarta, after 09:59:59",
			tz:       "Asia/Jakarta",
			hourMin:  "10:00",
			after:    time.Date(2026, 5, 11, 2, 59, 59, 0, time.UTC),
			wantHour: 10,
			wantMin:  0,
		},
		{
			name:     "daily at 10:00 Asia/Jakarta, after 10:00:00",
			tz:       "Asia/Jakarta",
			hourMin:  "10:00",
			after:    time.Date(2026, 5, 11, 3, 0, 0, 0, time.UTC),
			wantHour: 10,
			wantMin:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Reminder{
				Kind:     KindDaily,
				Timezone: tt.tz,
				HourMin:  tt.hourMin,
			}

			got, err := ComputeNextRun(r, tt.after)
			if err != nil {
				t.Fatalf("ComputeNextRun failed: %v", err)
			}

			loc, _ := time.LoadLocation(tt.tz)
			gotLocal := got.In(loc)
			if gotLocal.Hour() != tt.wantHour || gotLocal.Minute() != tt.wantMin {
				t.Errorf("ComputeNextRun = %v (local: %v), want hour=%d min=%d", got, gotLocal, tt.wantHour, tt.wantMin)
			}
		})
	}
}

func TestComputeNextRunWeekly(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Jakarta")
	mondayUTC := time.Date(2026, 5, 11, 3, 0, 0, 0, time.UTC)

	r := &Reminder{
		Kind:     KindWeekly,
		Timezone: "Asia/Jakarta",
		HourMin:  "10:00",
		Weekday:  time.Wednesday,
	}

	got, err := ComputeNextRun(r, mondayUTC)
	if err != nil {
		t.Fatalf("ComputeNextRun failed: %v", err)
	}

	gotLocal := got.In(loc)
	if gotLocal.Weekday() != time.Wednesday {
		t.Errorf("ComputeNextRun weekday = %v, want Wednesday", gotLocal.Weekday())
	}
	if gotLocal.Hour() != 10 || gotLocal.Minute() != 0 {
		t.Errorf("ComputeNextRun time = %02d:%02d, want 10:00", gotLocal.Hour(), gotLocal.Minute())
	}

	if !got.After(mondayUTC) {
		t.Errorf("ComputeNextRun = %v, should be after %v", got, mondayUTC)
	}
}

func TestComputeNextRunOnce(t *testing.T) {
	r := &Reminder{
		Kind:    KindOnce,
		NextRun: time.Date(2026, 5, 12, 10, 0, 0, 0, time.UTC),
	}

	after := time.Date(2026, 5, 11, 10, 0, 0, 0, time.UTC)
	got, err := ComputeNextRun(r, after)
	if err != nil {
		t.Fatalf("ComputeNextRun failed: %v", err)
	}
	if got != r.NextRun {
		t.Errorf("ComputeNextRun = %v, want %v", got, r.NextRun)
	}

	afterPassed := time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC)
	_, err = ComputeNextRun(r, afterPassed)
	if err == nil {
		t.Errorf("ComputeNextRun should error when once reminder already passed")
	}
}

func TestComputeNextRunInvalidTimezone(t *testing.T) {
	r := &Reminder{
		Kind:     KindDaily,
		Timezone: "Invalid/Timezone",
		HourMin:  "10:00",
	}

	_, err := ComputeNextRun(r, time.Now())
	if err != ErrInvalidTimezone {
		t.Errorf("ComputeNextRun returned %v, want ErrInvalidTimezone", err)
	}
}

func TestComputeNextRunInvalidHourMin(t *testing.T) {
	tests := []string{"25:00", "10:60", "10", "invalid"}

	for _, hourMin := range tests {
		r := &Reminder{
			Kind:     KindDaily,
			Timezone: "UTC",
			HourMin:  hourMin,
		}

		_, err := ComputeNextRun(r, time.Now())
		if err != ErrInvalidHourMin {
			t.Errorf("ComputeNextRun with %q returned %v, want ErrInvalidHourMin", hourMin, err)
		}
	}
}
