package reminder

import "time"

type ReminderKind string

const (
	KindDaily  ReminderKind = "daily"
	KindWeekly ReminderKind = "weekly"
	KindOnce   ReminderKind = "once"
)

type Reminder struct {
	ID        int64
	GuildID   int64
	ChannelID int64
	CreatedBy int64
	Kind      ReminderKind
	Message   string     // Indonesian text to send
	Timezone  string     // e.g. "Asia/Jakarta"
	HourMin   string     // e.g. "10:00" (24h)
	Weekday   time.Weekday // for weekly
	NextRun   time.Time  // next fire time in UTC
	CreatedAt time.Time
}
