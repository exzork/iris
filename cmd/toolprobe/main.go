package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/eko/iris-bot/internal/app/wire"
	"github.com/eko/iris-bot/internal/config"
	"github.com/eko/iris-bot/internal/llm"
	"github.com/eko/iris-bot/internal/reminder"
	"github.com/eko/iris-bot/internal/repository"
	"github.com/eko/iris-bot/internal/tools"
	reminderTool "github.com/eko/iris-bot/internal/tools/reminder"
)

type fakeSender struct {
	mu       chan struct{}
	sent     []sentMsg
}

type sentMsg struct {
	guildID, channelID int64
	content            string
}

func newFakeSender() *fakeSender {
	return &fakeSender{mu: make(chan struct{}, 1)}
}

func (f *fakeSender) Send(ctx context.Context, guildID, channelID int64, content string) error {
	f.sent = append(f.sent, sentMsg{guildID, channelID, content})
	select {
	case f.mu <- struct{}{}:
	default:
	}
	return nil
}

func main() {
	guildID := flag.Int64("guild", 0, "guild id (required)")
	channelID := flag.Int64("channel", 0, "channel id (required)")
	userID := flag.Int64("user", 0, "user id (required)")
	mode := flag.String("mode", "all", "tool_log | reminder | all")
	flag.Parse()

	if *guildID == 0 || *channelID == 0 || *userID == 0 {
		fmt.Fprintln(os.Stderr, "guild, channel, user are required")
		os.Exit(2)
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})))

	ctx := context.Background()
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config load", "err", err)
		os.Exit(1)
	}

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("pgxpool", "err", err)
		os.Exit(1)
	}
	defer pool.Close()
	db := repository.NewDB(pool)

	if *mode == "tool_log" || *mode == "all" {
		if err := exerciseToolLog(ctx, db, *guildID, *userID); err != nil {
			slog.Error("tool_log probe failed", "err", err.Error())
			os.Exit(1)
		}
	}

	if *mode == "reminder" || *mode == "all" {
		if err := exerciseReminder(ctx, db, *guildID, *channelID, *userID); err != nil {
			slog.Error("reminder probe failed", "err", err.Error())
			os.Exit(1)
		}
	}

	slog.Info("toolprobe success")
}

func exerciseToolLog(ctx context.Context, db *repository.DB, guildID, userID int64) error {
	beforeRow := db.QueryRow(ctx, "SELECT COUNT(*) FROM tool_logs WHERE guild_id = $1 AND tool_name = 'echo_probe'", guildID)
	var before int
	if err := beforeRow.Scan(&before); err != nil {
		return err
	}
	slog.Info("tool_log_probe_before", "rows", before)

	repo := repository.NewToolLogRepo(db)
	registry := tools.NewRegistry(&wire.ToolLogAuditAdapter{Repo: repo})
	if err := registry.Register(&tools.ToolDefinition{Tool: &echoTool{}, Timeout: time.Second, MaxOutput: 256}); err != nil {
		return err
	}
	res := registry.Execute(ctx, tools.ExecuteRequest{
		GuildID: guildID,
		UserID:  userID,
		Tool:    "echo_probe",
		Args:    map[string]interface{}{"text": "halo dunia"},
	})
	if res.Err != nil {
		return res.Err
	}

	afterRow := db.QueryRow(ctx, "SELECT COUNT(*) FROM tool_logs WHERE guild_id = $1 AND tool_name = 'echo_probe'", guildID)
	var after int
	if err := afterRow.Scan(&after); err != nil {
		return err
	}
	slog.Info("tool_log_probe_after", "rows", after, "delta", after-before, "output", res.Output)
	if after <= before {
		return errors.New("expected tool_logs row count to increase")
	}
	return nil
}

func exerciseReminder(ctx context.Context, db *repository.DB, guildID, channelID, userID int64) error {
	store := wire.NewPostgresReminderStore(db)
	sender := newFakeSender()
	svc := reminder.NewService(store, reminder.RealClock{}, sender)
	svc.Start(ctx)
	defer svc.Stop()
	svc.Scheduler.Tick = 1 * time.Second

	ll := llm.WithMeta(ctx, &llm.ContextMeta{
		GuildID:   guildID,
		ChannelID: channelID,
		UserID:    userID,
	})

	tool := reminderTool.New(svc)
	beforeRow := db.QueryRow(ctx, "SELECT COUNT(*) FROM reminders WHERE guild_id = $1", guildID)
	var before int
	if err := beforeRow.Scan(&before); err != nil {
		return err
	}
	slog.Info("reminder_probe_before", "rows", before)

	out, err := tool.Run(ll, map[string]interface{}{
		"message":    "Tes reminder dari probe",
		"in_minutes": float64(1),
	})
	if err != nil {
		return fmt.Errorf("create: %w", err)
	}
	slog.Info("reminder_created", "output", out)

	deadline := time.After(90 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-deadline:
			return errors.New("reminder did not fire within 60s")
		case <-sender.mu:
			if len(sender.sent) > 0 && sender.sent[len(sender.sent)-1].guildID == guildID {
				slog.Info("reminder_fired", "messages", len(sender.sent), "last", sender.sent[len(sender.sent)-1].content)
				time.Sleep(1 * time.Second)
				return nil
			}
		case <-ticker.C:
		}
	}
}

type echoTool struct{}

func (e *echoTool) Schema() *tools.Schema {
	return &tools.Schema{
		Name:        "echo_probe",
		Description: "ephemeral probe tool",
		Fields: []tools.FieldSpec{
			{Name: "text", Kind: tools.KindString, Required: true, Description: "echo text"},
		},
	}
}

func (e *echoTool) Run(ctx context.Context, args map[string]interface{}) (string, error) {
	t, _ := args["text"].(string)
	return fmt.Sprintf("echo: %s", t), nil
}

var _ tools.Tool = (*echoTool)(nil)
