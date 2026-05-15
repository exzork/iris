package reminder

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/eko/iris-bot/internal/llm"
	reminderpkg "github.com/eko/iris-bot/internal/reminder"
	"github.com/eko/iris-bot/internal/tools"
)

type Tool struct {
	Service *reminderpkg.Service
}

func New(svc *reminderpkg.Service) *Tool {
	return &Tool{Service: svc}
}

func (t *Tool) Schema() *tools.Schema {
	return &tools.Schema{
		Name:        "reminder_create",
		Description: "Schedule a reminder that I.R.I.S will deliver in this Discord channel at the requested time. Use for explicit user requests like 'iris ingetin gw 5 menit lagi' or 'iris reminder besok jam 9'. Times are interpreted in Asia/Jakarta unless the user specifies another tz. Once-shot only (kind=once); recurring reminders are not supported via this tool.",
		Fields: []tools.FieldSpec{
			{
				Name:        "message",
				Kind:        tools.KindString,
				Required:    true,
				Description: "The reminder text to send back to the user when the reminder fires. Bahasa Indonesia, single line, <500 chars.",
			},
			{
				Name:        "in_minutes",
				Kind:        tools.KindNumber,
				Required:    false,
				Description: "Fire the reminder this many minutes from now. Mutually exclusive with run_at_iso.",
			},
			{
				Name:        "run_at_iso",
				Kind:        tools.KindString,
				Required:    false,
				Description: "Absolute fire time in RFC3339 (e.g. 2026-05-16T09:00:00+07:00). Mutually exclusive with in_minutes.",
			},
		},
	}
}

func (t *Tool) Run(ctx context.Context, args map[string]interface{}) (string, error) {
	if t.Service == nil {
		return "", fmt.Errorf("reminder service unavailable")
	}

	meta := llm.MetaFromContext(ctx)
	if meta == nil || meta.GuildID == 0 || meta.ChannelID == 0 {
		return "", fmt.Errorf("reminder_create requires guild and channel context")
	}

	message, _ := args["message"].(string)
	message = strings.TrimSpace(message)
	if message == "" {
		return "", fmt.Errorf("message is required")
	}

	now := time.Now().UTC()
	var runAt time.Time
	if v, ok := args["in_minutes"]; ok {
		minutes, err := numberArg(v)
		if err != nil {
			return "", fmt.Errorf("in_minutes: %w", err)
		}
		if minutes <= 0 {
			return "", fmt.Errorf("in_minutes must be > 0")
		}
		if minutes > 60*24*30 {
			return "", fmt.Errorf("in_minutes too large; max 30 days")
		}
		runAt = now.Add(time.Duration(minutes) * time.Minute)
	}
	if v, ok := args["run_at_iso"].(string); ok && strings.TrimSpace(v) != "" {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(v))
		if err != nil {
			return "", fmt.Errorf("run_at_iso must be RFC3339: %w", err)
		}
		runAt = parsed.UTC()
	}
	if runAt.IsZero() {
		return "", fmt.Errorf("either in_minutes or run_at_iso is required")
	}
	if runAt.Before(now.Add(15 * time.Second)) {
		return "", fmt.Errorf("reminder must be at least 15 seconds in the future")
	}

	r, err := t.Service.Create(ctx, reminderpkg.CreateInput{
		GuildID:   meta.GuildID,
		ChannelID: meta.ChannelID,
		CreatedBy: meta.UserID,
		Kind:      reminderpkg.KindOnce,
		Message:   message,
		Timezone:  "Asia/Jakarta",
		RunAt:     runAt,
	})
	if err != nil {
		return "", fmt.Errorf("create reminder: %w", err)
	}

	out := map[string]interface{}{
		"id":            r.ID,
		"scheduled_for": r.NextRun.Format(time.RFC3339),
		"channel_id":    r.ChannelID,
		"message":       r.Message,
	}
	b, _ := json.Marshal(out)
	return string(b), nil
}

func numberArg(v interface{}) (int64, error) {
	switch n := v.(type) {
	case float64:
		return int64(n), nil
	case int:
		return int64(n), nil
	case int64:
		return n, nil
	case string:
		var i int64
		_, err := fmt.Sscanf(n, "%d", &i)
		return i, err
	}
	return 0, fmt.Errorf("not a number")
}
