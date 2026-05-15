package wire

import (
	"context"
	"log/slog"

	"github.com/eko/iris-bot/internal/repository"
	"github.com/eko/iris-bot/internal/tools"
)

// ToolLogAuditAdapter persists every tool execution to tool_logs. It implements
// tools.AuditLogger so the registry can pass through to the repository without
// pulling repository deps into the tools package.
type ToolLogAuditAdapter struct {
	Repo *repository.ToolLogRepo
}

func (a *ToolLogAuditAdapter) Record(ctx context.Context, evt tools.AuditEvent) error {
	if a.Repo == nil {
		return nil
	}
	args := evt.Args
	if args == nil {
		args = map[string]interface{}{}
	}
	output := map[string]interface{}{}
	if evt.Output != "" {
		output["raw"] = evt.Output
	}
	if evt.Duration > 0 {
		output["duration_ms"] = evt.Duration.Milliseconds()
	}
	if err := a.Repo.Save(ctx, evt.GuildID, evt.UserID, evt.Tool, args, output, evt.Status, evt.Error); err != nil {
		slog.Default().Warn("tool_log_audit_save_failed", "tool", evt.Tool, "guild", evt.GuildID, "user", evt.UserID, "err", err.Error())
		return err
	}
	return nil
}
