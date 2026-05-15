package wire

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/eko/iris-bot/internal/reminder"
	"github.com/eko/iris-bot/internal/repository"
	"github.com/jackc/pgx/v5"
)

// PostgresReminderStore persists reminders to the reminders table. It maps
// reminder.Reminder onto the original schema (id, guild_id, user_id,
// channel_id, reminder_text, scheduled_for, created_at) by serializing the
// reminder kind/timezone/hour/weekday into a `kind|tz|hour|weekday` prefix
// of reminder_text so we don't have to migrate the schema.
type PostgresReminderStore struct {
	DB *repository.DB
}

func NewPostgresReminderStore(db *repository.DB) *PostgresReminderStore {
	return &PostgresReminderStore{DB: db}
}

const reminderMetaSep = "\x1f"

func encodeReminder(r *reminder.Reminder) string {
	meta := fmt.Sprintf("v1%s%s%s%s%s%d", reminderMetaSep, r.Kind, reminderMetaSep, r.Timezone, reminderMetaSep, r.Weekday)
	hour := r.HourMin
	return meta + reminderMetaSep + hour + reminderMetaSep + r.Message
}

func decodeReminder(text string) (kind reminder.ReminderKind, tz, hour string, weekday time.Weekday, message string) {
	parts := splitN(text, reminderMetaSep, 6)
	if len(parts) != 6 || parts[0] != "v1" {
		return reminder.KindOnce, "UTC", "", time.Sunday, text
	}
	kind = reminder.ReminderKind(parts[1])
	tz = parts[2]
	wd := time.Sunday
	if n := parseWeekday(parts[3]); n >= 0 {
		wd = time.Weekday(n)
	}
	hour = parts[4]
	message = parts[5]
	weekday = wd
	return
}

func parseWeekday(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return -1
		}
		n = n*10 + int(c-'0')
	}
	if n > 6 {
		return -1
	}
	return n
}

func splitN(s, sep string, n int) []string {
	out := make([]string, 0, n)
	for i := 0; i < n-1; i++ {
		idx := indexOf(s, sep)
		if idx < 0 {
			out = append(out, s)
			return out
		}
		out = append(out, s[:idx])
		s = s[idx+len(sep):]
	}
	out = append(out, s)
	return out
}

func indexOf(s, sep string) int {
	for i := 0; i+len(sep) <= len(s); i++ {
		if s[i:i+len(sep)] == sep {
			return i
		}
	}
	return -1
}

func (s *PostgresReminderStore) Create(ctx context.Context, r *reminder.Reminder) (int64, error) {
	const q = `
		INSERT INTO reminders (guild_id, user_id, channel_id, reminder_text, scheduled_for, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`
	encoded := encodeReminder(r)
	now := time.Now().UTC()
	var id int64
	err := s.DB.QueryRow(ctx, q, r.GuildID, r.CreatedBy, r.ChannelID, encoded, r.NextRun.UTC(), now).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("postgres reminder create: %w", err)
	}
	r.ID = id
	r.CreatedAt = now
	return id, nil
}

func (s *PostgresReminderStore) Get(ctx context.Context, id int64) (*reminder.Reminder, error) {
	const q = `
		SELECT id, guild_id, user_id, channel_id, reminder_text, scheduled_for, created_at
		FROM reminders
		WHERE id = $1
	`
	rec, err := s.scanOne(ctx, q, id)
	if err != nil {
		return nil, err
	}
	return rec, nil
}

func (s *PostgresReminderStore) List(ctx context.Context, guildID int64) ([]*reminder.Reminder, error) {
	const q = `
		SELECT id, guild_id, user_id, channel_id, reminder_text, scheduled_for, created_at
		FROM reminders
		WHERE guild_id = $1
		ORDER BY scheduled_for ASC
	`
	rows, err := s.DB.Query(ctx, q, guildID)
	if err != nil {
		return nil, fmt.Errorf("postgres reminder list: %w", err)
	}
	defer rows.Close()
	var out []*reminder.Reminder
	for rows.Next() {
		r, err := scanReminderRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

func (s *PostgresReminderStore) Delete(ctx context.Context, guildID, id int64) error {
	const q = `DELETE FROM reminders WHERE guild_id = $1 AND id = $2`
	tag, err := s.DB.Exec(ctx, q, guildID, id)
	if err != nil {
		return fmt.Errorf("postgres reminder delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return reminder.ErrNotFound
	}
	return nil
}

func (s *PostgresReminderStore) UpdateNextRun(ctx context.Context, id int64, next time.Time) error {
	const q = `UPDATE reminders SET scheduled_for = $2 WHERE id = $1`
	tag, err := s.DB.Exec(ctx, q, id, next.UTC())
	if err != nil {
		return fmt.Errorf("postgres reminder update next_run: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return reminder.ErrNotFound
	}
	return nil
}

func (s *PostgresReminderStore) Due(ctx context.Context, now time.Time) ([]*reminder.Reminder, error) {
	const q = `
		SELECT id, guild_id, user_id, channel_id, reminder_text, scheduled_for, created_at
		FROM reminders
		WHERE scheduled_for <= $1
		ORDER BY scheduled_for ASC
	`
	rows, err := s.DB.Query(ctx, q, now.UTC())
	if err != nil {
		return nil, fmt.Errorf("postgres reminder due: %w", err)
	}
	defer rows.Close()
	var out []*reminder.Reminder
	for rows.Next() {
		r, err := scanReminderRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

func (s *PostgresReminderStore) scanOne(ctx context.Context, query string, args ...interface{}) (*reminder.Reminder, error) {
	row := s.DB.QueryRow(ctx, query, args...)
	r, err := scanReminderSingle(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return nil, reminder.ErrNotFound
		}
		return nil, err
	}
	return r, nil
}

func scanReminderRow(rows pgx.Rows) (*reminder.Reminder, error) {
	var (
		id, guildID, channelID int64
		userID                 *int64
		text                   string
		scheduledFor, created  time.Time
	)
	if err := rows.Scan(&id, &guildID, &userID, &channelID, &text, &scheduledFor, &created); err != nil {
		return nil, fmt.Errorf("postgres reminder scan: %w", err)
	}
	kind, tz, hour, weekday, message := decodeReminder(text)
	createdBy := int64(0)
	if userID != nil {
		createdBy = *userID
	}
	return &reminder.Reminder{
		ID:        id,
		GuildID:   guildID,
		ChannelID: channelID,
		CreatedBy: createdBy,
		Kind:      kind,
		Message:   message,
		Timezone:  tz,
		HourMin:   hour,
		Weekday:   weekday,
		NextRun:   scheduledFor,
		CreatedAt: created,
	}, nil
}

func scanReminderSingle(row pgx.Row) (*reminder.Reminder, error) {
	var (
		id, guildID, channelID int64
		userID                 *int64
		text                   string
		scheduledFor, created  time.Time
	)
	if err := row.Scan(&id, &guildID, &userID, &channelID, &text, &scheduledFor, &created); err != nil {
		return nil, fmt.Errorf("postgres reminder scan: %w", err)
	}
	kind, tz, hour, weekday, message := decodeReminder(text)
	createdBy := int64(0)
	if userID != nil {
		createdBy = *userID
	}
	return &reminder.Reminder{
		ID:        id,
		GuildID:   guildID,
		ChannelID: channelID,
		CreatedBy: createdBy,
		Kind:      kind,
		Message:   message,
		Timezone:  tz,
		HourMin:   hour,
		Weekday:   weekday,
		NextRun:   scheduledFor,
		CreatedAt: created,
	}, nil
}

var _ reminder.Store = (*PostgresReminderStore)(nil)
