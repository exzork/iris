package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/eko/iris-bot/internal/tools"
)

// AddTool exposes `mcp_add` to the LLM. It is owner-gated: the tool refuses
// to execute unless ctx carries the owner's Discord user ID via
// WithCallerUserID. This is the primary guard against prompt-injection
// exploiting MCP to install attacker-controlled subprocesses.
type AddTool struct {
	sup *Supervisor
}

func NewAddTool(sup *Supervisor) *AddTool {
	return &AddTool{sup: sup}
}

func (t *AddTool) Schema() *tools.Schema {
	return &tools.Schema{
		Name:        "mcp_add",
		Description: "Install a new Model Context Protocol (MCP) server by name, command, args, and optional env. Owner-only. Persists to mcps.json and hot-reloads tools.",
		Fields: []tools.FieldSpec{
			{Name: "name", Kind: tools.KindString, Required: true, Description: "Short identifier for the server (letters, numbers, underscore)."},
			{Name: "command", Kind: tools.KindString, Required: true, Description: "Executable path or npm/uvx/pipx wrapper, e.g. 'npx' or 'uvx'."},
			{Name: "args", Kind: tools.KindArray, Required: false, Description: "Arguments as an array of strings, e.g. ['-y', '@modelcontextprotocol/server-filesystem', '/data']."},
			{Name: "env", Kind: tools.KindObject, Required: false, Description: "Environment variables as key-value pairs (strings)."},
		},
	}
}

func (t *AddTool) Run(ctx context.Context, args map[string]interface{}) (string, error) {
	if err := assertOwner(ctx, t.sup); err != nil {
		return "", err
	}
	name, _ := args["name"].(string)
	cmdStr, _ := args["command"].(string)

	var cmdArgs []string
	if raw, ok := args["args"]; ok && raw != nil {
		if arr, ok := raw.([]interface{}); ok {
			for _, v := range arr {
				if s, ok := v.(string); ok {
					cmdArgs = append(cmdArgs, s)
				}
			}
		}
	}

	env := make(map[string]string)
	if raw, ok := args["env"]; ok && raw != nil {
		if obj, ok := raw.(map[string]interface{}); ok {
			for k, v := range obj {
				if s, ok := v.(string); ok {
					env[k] = s
				}
			}
		}
	}

	if err := t.sup.Add(ctx, name, ServerConfig{Command: cmdStr, Args: cmdArgs, Env: env}); err != nil {
		return "", err
	}
	return fmt.Sprintf("MCP server %q installed. Tools available as %s_<tool>.", name, sanitize(name)), nil
}

// RemoveTool exposes `mcp_remove` to the LLM (owner-gated).
type RemoveTool struct {
	sup *Supervisor
}

func NewRemoveTool(sup *Supervisor) *RemoveTool {
	return &RemoveTool{sup: sup}
}

func (t *RemoveTool) Schema() *tools.Schema {
	return &tools.Schema{
		Name:        "mcp_remove",
		Description: "Uninstall an MCP server by name. Owner-only. Persists to mcps.json and hot-reloads tools.",
		Fields: []tools.FieldSpec{
			{Name: "name", Kind: tools.KindString, Required: true, Description: "Name of the installed server to remove."},
		},
	}
}

func (t *RemoveTool) Run(ctx context.Context, args map[string]interface{}) (string, error) {
	if err := assertOwner(ctx, t.sup); err != nil {
		return "", err
	}
	name, _ := args["name"].(string)
	if err := t.sup.Remove(ctx, name); err != nil {
		return "", err
	}
	return fmt.Sprintf("MCP server %q removed.", name), nil
}

// ListTool exposes `mcp_list` to the LLM (owner-gated).
type ListTool struct {
	sup *Supervisor
}

func NewListTool(sup *Supervisor) *ListTool {
	return &ListTool{sup: sup}
}

func (t *ListTool) Schema() *tools.Schema {
	return &tools.Schema{
		Name:        "mcp_list",
		Description: "List installed MCP servers. Owner-only.",
	}
}

func (t *ListTool) Run(ctx context.Context, args map[string]interface{}) (string, error) {
	if err := assertOwner(ctx, t.sup); err != nil {
		return "", err
	}
	names := t.sup.List()
	out, _ := json.Marshal(names)
	return string(out), nil
}

// assertOwner is the single authorization check shared by every mcp_* tool.
// Rejecting here closes the prompt-injection path: even if the LLM is
// convinced by user input to call mcp_add/remove, the tool refuses unless
// the originating Discord user is the configured owner.
func assertOwner(ctx context.Context, sup *Supervisor) error {
	if sup.OwnerID() == 0 {
		return fmt.Errorf("mcp: owner is not configured; refusing MCP mutation")
	}
	caller := CallerUserID(ctx)
	if caller == 0 || caller != sup.OwnerID() {
		return fmt.Errorf("mcp: only the bot owner may modify MCP configuration")
	}
	return nil
}
