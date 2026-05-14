package modelswitch

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/eko/iris-bot/internal/llm"
	"github.com/eko/iris-bot/internal/mcp"
	"github.com/eko/iris-bot/internal/tools"
)

type AuditLogger interface {
	Log(ctx context.Context, guildID, userID int64, eventType, entityType, entityID string, changes map[string]interface{}) error
}

// SetTool exposes `iris_set_model` to the LLM. Owner-gated via
// mcp.CallerUserID. Persists the override to the global_settings store so
// subsequent Iris replies use the new model on the fly.
type SetTool struct {
	resolver *llm.ModelResolver
	ownerID  int64
	audit    AuditLogger
}

func NewSetTool(resolver *llm.ModelResolver, ownerID int64, audit AuditLogger) *SetTool {
	return &SetTool{resolver: resolver, ownerID: ownerID, audit: audit}
}

func (t *SetTool) Schema() *tools.Schema {
	return &tools.Schema{
		Name: "iris_set_model",
		Description: "Switch the active LLM model for a given tier (default | strong | router) at runtime. " +
			"Owner-only. The new model persists across restarts via global_settings. " +
			"Use when the bot owner asks Iris to change models (e.g. 'switch default to kr/claude-sonnet-4.5').",
		Fields: []tools.FieldSpec{
			{Name: "tier", Kind: tools.KindString, Required: true, Description: "Which tier to change: 'default', 'strong', or 'router'."},
			{Name: "model", Kind: tools.KindString, Required: true, Description: "Fully-qualified model name, e.g. 'kr/claude-sonnet-4.5' or 'kr/claude-opus-4.7'."},
		},
	}
}

func (t *SetTool) Run(ctx context.Context, args map[string]interface{}) (string, error) {
	if err := assertOwner(ctx, t.ownerID); err != nil {
		return "", err
	}
	tierStr, _ := args["tier"].(string)
	model, _ := args["model"].(string)

	tierStr = strings.ToLower(strings.TrimSpace(tierStr))
	model = strings.TrimSpace(model)
	if tierStr == "" || model == "" {
		return "", fmt.Errorf("both 'tier' and 'model' are required")
	}

	tier, err := llm.ParseTier(tierStr)
	if err != nil {
		return "", err
	}

	actor := mcp.CallerUserID(ctx)
	previous := currentFor(t.resolver, tier)
	if err := t.resolver.SetOverride(ctx, tier, model, actor); err != nil {
		return "", err
	}

	if t.audit != nil {
		_ = t.audit.Log(ctx, 0, actor, "model_switch", "global_settings", string(tier), map[string]interface{}{
			"tier":     string(tier),
			"model":    model,
			"previous": previous,
		})
	}

	return fmt.Sprintf("Model tier %q diperbarui: %s → %s.", tier, previous, model), nil
}

// GetTool exposes `iris_get_models` to the LLM. Read-only, not owner-gated,
// so the owner can inspect the active models without a separate path.
type GetTool struct {
	resolver *llm.ModelResolver
}

func NewGetTool(resolver *llm.ModelResolver) *GetTool {
	return &GetTool{resolver: resolver}
}

func (t *GetTool) Schema() *tools.Schema {
	return &tools.Schema{
		Name:        "iris_get_models",
		Description: "Return the currently active LLM models for each tier (default, strong, router) together with the env-var fallbacks.",
	}
}

func (t *GetTool) Run(ctx context.Context, args map[string]interface{}) (string, error) {
	if t.resolver == nil {
		return "", fmt.Errorf("model resolver not configured")
	}
	type row struct {
		Tier     string `json:"tier"`
		Active   string `json:"active"`
		Fallback string `json:"fallback"`
	}
	rows := []row{
		{Tier: "default", Active: t.resolver.Default(), Fallback: t.resolver.Fallback(llm.ModelTierDefault)},
		{Tier: "strong", Active: t.resolver.Strong(), Fallback: t.resolver.Fallback(llm.ModelTierStrong)},
		{Tier: "router", Active: t.resolver.Router(), Fallback: t.resolver.Fallback(llm.ModelTierRouter)},
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Tier < rows[j].Tier })

	var b strings.Builder
	b.WriteString("Model aktif:\n")
	for _, r := range rows {
		marker := ""
		if r.Active != r.Fallback {
			marker = " (override)"
		}
		fmt.Fprintf(&b, "- %s: %s%s (fallback: %s)\n", r.Tier, r.Active, marker, r.Fallback)
	}
	return b.String(), nil
}

// ResetTool exposes `iris_reset_model` to the LLM. Owner-gated. Removes the
// persisted override for a tier so subsequent reads return the env-var
// fallback captured at boot.
type ResetTool struct {
	resolver *llm.ModelResolver
	ownerID  int64
	audit    AuditLogger
}

func NewResetTool(resolver *llm.ModelResolver, ownerID int64, audit AuditLogger) *ResetTool {
	return &ResetTool{resolver: resolver, ownerID: ownerID, audit: audit}
}

func (t *ResetTool) Schema() *tools.Schema {
	return &tools.Schema{
		Name:        "iris_reset_model",
		Description: "Clear the runtime model override for a tier (default | strong | router) so the env-var fallback takes over again. Owner-only.",
		Fields: []tools.FieldSpec{
			{Name: "tier", Kind: tools.KindString, Required: true, Description: "Which tier to reset: 'default', 'strong', or 'router'."},
		},
	}
}

func (t *ResetTool) Run(ctx context.Context, args map[string]interface{}) (string, error) {
	if err := assertOwner(ctx, t.ownerID); err != nil {
		return "", err
	}
	tierStr, _ := args["tier"].(string)
	tierStr = strings.ToLower(strings.TrimSpace(tierStr))
	if tierStr == "" {
		return "", fmt.Errorf("'tier' is required")
	}

	tier, err := llm.ParseTier(tierStr)
	if err != nil {
		return "", err
	}

	actor := mcp.CallerUserID(ctx)
	previous := currentFor(t.resolver, tier)
	if err := t.resolver.ResetOverride(ctx, tier); err != nil {
		return "", err
	}
	fallback := t.resolver.Fallback(tier)

	if t.audit != nil {
		_ = t.audit.Log(ctx, 0, actor, "model_reset", "global_settings", string(tier), map[string]interface{}{
			"tier":     string(tier),
			"previous": previous,
			"fallback": fallback,
		})
	}

	return fmt.Sprintf("Model tier %q direset ke fallback: %s → %s.", tier, previous, fallback), nil
}

func currentFor(r *llm.ModelResolver, tier llm.ModelTier) string {
	if r == nil {
		return ""
	}
	switch tier {
	case llm.ModelTierStrong:
		return r.Strong()
	case llm.ModelTierRouter:
		return r.Router()
	default:
		return r.Default()
	}
}

func assertOwner(ctx context.Context, ownerID int64) error {
	if ownerID == 0 {
		return fmt.Errorf("owner is not configured; refusing model-switch mutation")
	}
	if mcp.CallerUserID(ctx) != ownerID {
		return fmt.Errorf("only the bot owner may switch models")
	}
	return nil
}
