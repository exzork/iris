package reminder

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

func ComputeNextRun(r *Reminder, after time.Time) (time.Time, error) {
	loc, err := time.LoadLocation(r.Timezone)
	if err != nil {
		return time.Time{}, ErrInvalidTimezone
	}

	switch r.Kind {
	case KindDaily:
		return computeNextRunDaily(r, after, loc)
	case KindWeekly:
		return computeNextRunWeekly(r, after, loc)
	case KindOnce:
		if r.NextRun.After(after) {
			return r.NextRun, nil
		}
		return time.Time{}, fmt.Errorf("once reminder already passed")
	default:
		return time.Time{}, fmt.Errorf("unknown reminder kind: %s", r.Kind)
	}
}

func computeNextRunDaily(r *Reminder, after time.Time, loc *time.Location) (time.Time, error) {
	parts := strings.Split(r.HourMin, ":")
	if len(parts) != 2 {
		return time.Time{}, ErrInvalidHourMin
	}

	hour, err := strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return time.Time{}, ErrInvalidHourMin
	}

	min, err := strconv.Atoi(parts[1])
	if err != nil || min < 0 || min > 59 {
		return time.Time{}, ErrInvalidHourMin
	}

	afterLocal := after.In(loc)
	candidate := time.Date(afterLocal.Year(), afterLocal.Month(), afterLocal.Day(), hour, min, 0, 0, loc)

	if candidate.After(after) {
		return candidate.UTC(), nil
	}

	candidate = candidate.AddDate(0, 0, 1)
	return candidate.UTC(), nil
}

func computeNextRunWeekly(r *Reminder, after time.Time, loc *time.Location) (time.Time, error) {
	parts := strings.Split(r.HourMin, ":")
	if len(parts) != 2 {
		return time.Time{}, ErrInvalidHourMin
	}

	hour, err := strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return time.Time{}, ErrInvalidHourMin
	}

	min, err := strconv.Atoi(parts[1])
	if err != nil || min < 0 || min > 59 {
		return time.Time{}, ErrInvalidHourMin
	}

	afterLocal := after.In(loc)
	daysUntilTarget := int(r.Weekday - afterLocal.Weekday())
	if daysUntilTarget <= 0 {
		daysUntilTarget += 7
	}

	candidate := time.Date(afterLocal.Year(), afterLocal.Month(), afterLocal.Day(), hour, min, 0, 0, loc)
	candidate = candidate.AddDate(0, 0, daysUntilTarget)

	if candidate.After(after) {
		return candidate.UTC(), nil
	}

	candidate = candidate.AddDate(0, 0, 7)
	return candidate.UTC(), nil
}
