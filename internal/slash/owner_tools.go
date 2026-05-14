package slash

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/eko/iris-bot/internal/mcp"
	"github.com/eko/iris-bot/internal/tools"
)

// BindSlashTool exposes `mcp_bind_slash` to the LLM. Owner-gated: refuses
// unless ctx carries the configured owner's Discord user ID via
// mcp.WithCallerUserID. Creates or replaces a slash-command binding in
// mcps.json and triggers per-guild Discord re-registration so the command
// appears within seconds.
type BindSlashTool struct {
	store   *Store
	ownerID int64
}

func NewBindSlashTool(store *Store, ownerID int64) *BindSlashTool {
	return &BindSlashTool{store: store, ownerID: ownerID}
}

func (t *BindSlashTool) Schema() *tools.Schema {
	return &tools.Schema{
		Name:        "mcp_bind_slash",
		Description: "Create or replace a Discord slash command bound to a tool. Owner-only. Persists to mcps.json and hot-reloads slash commands.",
		Fields: []tools.FieldSpec{
			{Name: "name", Kind: tools.KindString, Required: true, Description: "Slash command name (1-32 chars, [a-z0-9_-])."},
			{Name: "tool", Kind: tools.KindString, Required: true, Description: "Target tool name registered in the tool registry, e.g. 'websearch' or 'pixiv_search_illust'."},
			{Name: "description", Kind: tools.KindString, Required: true, Description: "User-facing description shown in Discord's command picker."},
			{Name: "options", Kind: tools.KindArray, Required: false, Description: "Array of option bindings: [{name, description, type, required, schemaPath}]. type must be one of: string, integer, boolean, user, channel, role."},
			{Name: "ephemeral", Kind: tools.KindBool, Required: false, Description: "If true, the response is only visible to the invoking user."},
			{Name: "adminOnly", Kind: tools.KindBool, Required: false, Description: "If true, only guild admins can run the command."},
			{Name: "ownerOnly", Kind: tools.KindBool, Required: false, Description: "If true, only the bot owner can run the command."},
		},
	}
}

func (t *BindSlashTool) Run(ctx context.Context, args map[string]interface{}) (string, error) {
	if err := t.assertOwner(ctx); err != nil {
		return "", err
	}
	binding, name, err := bindingFromArgs(args)
	if err != nil {
		return "", err
	}
	if err := t.store.Set(name, binding); err != nil {
		return "", err
	}
	return fmt.Sprintf("Slash command /%s bound to tool %q.", name, binding.Tool), nil
}

// UnbindSlashTool exposes `mcp_unbind_slash` to the LLM (owner-gated).
type UnbindSlashTool struct {
	store   *Store
	ownerID int64
}

func NewUnbindSlashTool(store *Store, ownerID int64) *UnbindSlashTool {
	return &UnbindSlashTool{store: store, ownerID: ownerID}
}

func (t *UnbindSlashTool) Schema() *tools.Schema {
	return &tools.Schema{
		Name:        "mcp_unbind_slash",
		Description: "Remove a Discord slash command binding. Owner-only. Persists to mcps.json and hot-reloads slash commands.",
		Fields: []tools.FieldSpec{
			{Name: "name", Kind: tools.KindString, Required: true, Description: "Slash command name to unbind."},
		},
	}
}

func (t *UnbindSlashTool) Run(ctx context.Context, args map[string]interface{}) (string, error) {
	if err := t.assertOwner(ctx); err != nil {
		return "", err
	}
	name, _ := args["name"].(string)
	if err := t.store.Delete(name); err != nil {
		return "", err
	}
	return fmt.Sprintf("Slash command /%s unbound.", name), nil
}

// ListSlashBindingsTool exposes `mcp_list_slash` to the LLM (owner-gated).
type ListSlashBindingsTool struct {
	store   *Store
	ownerID int64
}

func NewListSlashBindingsTool(store *Store, ownerID int64) *ListSlashBindingsTool {
	return &ListSlashBindingsTool{store: store, ownerID: ownerID}
}

func (t *ListSlashBindingsTool) Schema() *tools.Schema {
	return &tools.Schema{
		Name:        "mcp_list_slash",
		Description: "List all current slash command bindings. Owner-only.",
	}
}

func (t *ListSlashBindingsTool) Run(ctx context.Context, args map[string]interface{}) (string, error) {
	if err := t.assertOwner(ctx); err != nil {
		return "", err
	}
	bindings := t.store.SlashBindings()
	out, _ := json.MarshalIndent(bindings, "", "  ")
	return string(out), nil
}

func (t *BindSlashTool) assertOwner(ctx context.Context) error {
	return assertOwnerCtx(ctx, t.ownerID)
}
func (t *UnbindSlashTool) assertOwner(ctx context.Context) error {
	return assertOwnerCtx(ctx, t.ownerID)
}
func (t *ListSlashBindingsTool) assertOwner(ctx context.Context) error {
	return assertOwnerCtx(ctx, t.ownerID)
}

func assertOwnerCtx(ctx context.Context, ownerID int64) error {
	if ownerID == 0 {
		return fmt.Errorf("slash: owner is not configured; refusing slash-binding mutation")
	}
	if mcp.CallerUserID(ctx) != ownerID {
		return fmt.Errorf("slash: only the bot owner may modify slash bindings")
	}
	return nil
}

// bindingFromArgs builds a Binding out of the loose tool-call JSON the LLM
// produces. Each option is extracted from the opaque args blob and checked
// against ValidateBinding so malformed input never hits the persist step.
func bindingFromArgs(args map[string]interface{}) (Binding, string, error) {
	name, _ := args["name"].(string)
	tool, _ := args["tool"].(string)
	desc, _ := args["description"].(string)

	b := Binding{
		Tool:        tool,
		Description: desc,
	}
	if v, ok := args["ephemeral"].(bool); ok {
		b.Ephemeral = v
	}
	if v, ok := args["adminOnly"].(bool); ok {
		b.AdminOnly = v
	}
	if v, ok := args["ownerOnly"].(bool); ok {
		b.OwnerOnly = v
	}

	if rawOpts, ok := args["options"]; ok && rawOpts != nil {
		arr, ok := rawOpts.([]interface{})
		if !ok {
			return Binding{}, name, fmt.Errorf("options must be an array")
		}
		for i, item := range arr {
			obj, ok := item.(map[string]interface{})
			if !ok {
				return Binding{}, name, fmt.Errorf("options[%d] must be an object", i)
			}
			opt := Option{}
			opt.Name, _ = obj["name"].(string)
			opt.Description, _ = obj["description"].(string)
			typStr, _ := obj["type"].(string)
			t, err := ParseOptionType(typStr)
			if err != nil {
				return Binding{}, name, fmt.Errorf("options[%d]: %w", i, err)
			}
			opt.Type = t
			if req, ok := obj["required"].(bool); ok {
				opt.Required = req
			}
			opt.SchemaPath, _ = obj["schemaPath"].(string)
			if opt.SchemaPath == "" {
				opt.SchemaPath = opt.Name
			}
			b.Options = append(b.Options, opt)
		}
	}

	if err := ValidateBinding(name, b); err != nil {
		return Binding{}, name, err
	}
	return b, name, nil
}
