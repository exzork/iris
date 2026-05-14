package slash

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/eko/iris-bot/internal/tools"
)

// ToolExecutor is the subset of tools.Registry the router uses.
type ToolExecutor interface {
	Execute(ctx context.Context, req tools.ExecuteRequest) tools.ExecuteResult
}

// Synthesizer turns a tool's raw output into a user-facing reply via the
// LLM. It is invoked after a slash binding executes so the user receives a
// natural-language response (in persona) rather than raw JSON.
type Synthesizer interface {
	Synthesize(ctx context.Context, guildID int64, toolName, userQuery, toolOutput string) (string, error)
}

// Router handles InteractionCreate events, routes the invocation to either a
// native command or a tool binding, and produces the Discord response. It
// plugs into the gateway via the InteractionHandler interface.
type Router struct {
	reg         *Registrar
	executor    ToolExecutor
	ownerID     int64
	callerCtx   func(ctx context.Context, userID int64) context.Context
	synthesizer Synthesizer
}

// NewRouter wires the dependencies. callerCtxFn attaches the invoking user
// ID to ctx so owner-gated tools (like mcp_add) can authenticate; pass
// mcp.WithCallerUserID through here.
func NewRouter(reg *Registrar, executor ToolExecutor, ownerID int64, callerCtxFn func(context.Context, int64) context.Context) *Router {
	return &Router{
		reg:       reg,
		executor:  executor,
		ownerID:   ownerID,
		callerCtx: callerCtxFn,
	}
}

// SetSynthesizer enables LLM-driven post-processing of tool outputs. When
// nil (default) the router falls back to the deterministic Markdown
// formatter, which keeps tests independent of the LLM client.
func (r *Router) SetSynthesizer(s Synthesizer) {
	r.synthesizer = s
}

// HandleInteraction satisfies discord.InteractionHandler.
func (r *Router) HandleInteraction(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i == nil || i.Interaction == nil {
		return
	}
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}

	data := i.ApplicationCommandData()
	name := data.Name
	userID := extractUserID(i)
	guildID, _ := strconv.ParseInt(i.GuildID, 10, 64)
	channelID, _ := strconv.ParseInt(i.ChannelID, 10, 64)

	if r.callerCtx != nil && userID != 0 {
		ctx = r.callerCtx(ctx, userID)
	}

	if native := r.reg.NativeByName(name); native != nil {
		r.handleNative(ctx, s, i, native, guildID, channelID, userID)
		return
	}

	binding, ok := r.reg.BindingByName(name)
	if !ok {
		respondError(s, i, "Perintah tidak dikenali.")
		return
	}
	r.handleBinding(ctx, s, i, name, binding, guildID, channelID, userID)
}

func (r *Router) handleNative(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate, native *NativeCommand, guildID, channelID, userID int64) {
	isAdmin := interactionIsAdmin(i)
	if native.AdminOnly && !isAdmin {
		respondError(s, i, "Mohon maaf, perintah ini hanya untuk admin server.")
		return
	}

	if err := defer_(s, i, false); err != nil {
		slog.Warn("slash_defer_failed", "command", native.Name, "err", err)
		return
	}

	data := i.ApplicationCommandData()
	inv := &NativeInvocation{
		GuildID:   guildID,
		ChannelID: channelID,
		UserID:    userID,
		IsAdmin:   isAdmin,
		Options:   data.Options,
	}
	reply, err := native.Execute(ctx, inv)
	if err != nil {
		slog.Warn("slash_native_error", "command", native.Name, "err", err)
		editResponse(s, i, "Terjadi error saat memproses perintah.")
		return
	}
	if strings.TrimSpace(reply) == "" {
		reply = "Perintah selesai."
	}
	editResponse(s, i, reply)
}

func (r *Router) handleBinding(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate, name string, binding Binding, guildID, channelID, userID int64) {
	if binding.OwnerOnly {
		if r.ownerID == 0 || userID != r.ownerID {
			respondError(s, i, "Perintah ini hanya untuk owner bot.")
			return
		}
	}
	if binding.AdminOnly && !interactionIsAdmin(i) {
		respondError(s, i, "Perintah ini hanya untuk admin server.")
		return
	}

	if err := defer_(s, i, binding.Ephemeral); err != nil {
		slog.Warn("slash_defer_failed", "command", name, "err", err)
		return
	}

	data := i.ApplicationCommandData()
	args, err := buildToolArgs(binding, data.Options)
	if err != nil {
		editResponse(s, i, fmt.Sprintf("Argumen tidak valid: %s", err.Error()))
		return
	}

	execCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	result := r.executor.Execute(execCtx, tools.ExecuteRequest{
		GuildID: guildID,
		UserID:  userID,
		Tool:    binding.Tool,
		Args:    args,
		Caller: tools.CallerContext{
			IsAdmin: interactionIsAdmin(i),
		},
	})
	if result.Err != nil {
		slog.Warn("slash_tool_error", "command", name, "tool", binding.Tool, "err", result.Err)
		editResponse(s, i, fmt.Sprintf("Tool gagal: %s", shortenErr(result.Err)))
		return
	}

	out := r.renderToolOutput(ctx, guildID, binding.Tool, args, result.Output)
	if len(out) > 1900 {
		out = out[:1900] + "…"
	}
	editResponse(s, i, out)
}

// renderToolOutput converts a tool's raw stdout into the message the user
// sees. When a Synthesizer is wired the LLM summarizes the output in
// persona (Indonesian, no JSON dump); on any synthesizer failure or empty
// reply the deterministic Markdown formatter is the fallback so slash
// commands never silently break.
func (r *Router) renderToolOutput(ctx context.Context, guildID int64, toolName string, args map[string]interface{}, raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "(tool kembali kosong)"
	}
	if r.synthesizer != nil {
		userQuery := queryFromArgs(args)
		synthCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		reply, err := r.synthesizer.Synthesize(synthCtx, guildID, toolName, userQuery, trimmed)
		if err != nil {
			slog.Warn("slash_synthesizer_error", "tool", toolName, "err", err)
		} else if strings.TrimSpace(reply) != "" {
			return reply
		}
	}
	return formatToolOutput(toolName, trimmed)
}

// queryFromArgs pulls a short representative string from the tool args so
// the synthesizer can ground its reply in what the user asked. It favors
// the "query" field (web_search) and falls back to the first string value.
func queryFromArgs(args map[string]interface{}) string {
	if args == nil {
		return ""
	}
	if q, ok := args["query"].(string); ok && strings.TrimSpace(q) != "" {
		return q
	}
	for _, v := range args {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

// buildToolArgs maps interaction options into the tool's argument object
// using each OptionBinding.SchemaPath. A schemaPath of "" or equal to the
// option Name produces a flat top-level field; dot-separated paths create
// nested maps so MCP tools that expect structured input work out of the box.
func buildToolArgs(binding Binding, opts []*discordgo.ApplicationCommandInteractionDataOption) (map[string]interface{}, error) {
	lookup := make(map[string]Option, len(binding.Options))
	for _, ob := range binding.Options {
		lookup[ob.Name] = ob
	}
	args := make(map[string]interface{})
	for _, opt := range opts {
		bind, ok := lookup[opt.Name]
		if !ok {
			continue
		}
		val, err := coerceOption(bind.Type, opt)
		if err != nil {
			return nil, fmt.Errorf("option %q: %w", opt.Name, err)
		}
		path := bind.SchemaPath
		if path == "" {
			path = bind.Name
		}
		setByPath(args, path, val)
	}
	for _, ob := range binding.Options {
		if !ob.Required {
			continue
		}
		path := ob.SchemaPath
		if path == "" {
			path = ob.Name
		}
		if _, ok := getByPath(args, path); !ok {
			return nil, fmt.Errorf("missing required option %q", ob.Name)
		}
	}
	return args, nil
}

func coerceOption(t OptionType, opt *discordgo.ApplicationCommandInteractionDataOption) (interface{}, error) {
	switch t {
	case OptionTypeString:
		return opt.StringValue(), nil
	case OptionTypeInteger:
		return opt.IntValue(), nil
	case OptionTypeBoolean:
		return opt.BoolValue(), nil
	case OptionTypeUser:
		return opt.UserValue(nil).ID, nil
	case OptionTypeChannel:
		if c := opt.ChannelValue(nil); c != nil {
			return c.ID, nil
		}
		return nil, errors.New("channel value not resolvable")
	case OptionTypeRole:
		if r := opt.RoleValue(nil, ""); r != nil {
			return r.ID, nil
		}
		return nil, errors.New("role value not resolvable")
	default:
		return nil, fmt.Errorf("unsupported option type %q", t)
	}
}

func setByPath(dst map[string]interface{}, path string, val interface{}) {
	parts := strings.Split(path, ".")
	cur := dst
	for i, p := range parts {
		if i == len(parts)-1 {
			cur[p] = val
			return
		}
		next, ok := cur[p].(map[string]interface{})
		if !ok {
			next = make(map[string]interface{})
			cur[p] = next
		}
		cur = next
	}
}

func getByPath(src map[string]interface{}, path string) (interface{}, bool) {
	parts := strings.Split(path, ".")
	cur := src
	for i, p := range parts {
		if i == len(parts)-1 {
			v, ok := cur[p]
			return v, ok
		}
		next, ok := cur[p].(map[string]interface{})
		if !ok {
			return nil, false
		}
		cur = next
	}
	return nil, false
}

func extractUserID(i *discordgo.InteractionCreate) int64 {
	if i.Member != nil && i.Member.User != nil {
		id, _ := strconv.ParseInt(i.Member.User.ID, 10, 64)
		return id
	}
	if i.User != nil {
		id, _ := strconv.ParseInt(i.User.ID, 10, 64)
		return id
	}
	return 0
}

func interactionIsAdmin(i *discordgo.InteractionCreate) bool {
	if i == nil || i.Member == nil {
		return false
	}
	return int64(i.Member.Permissions)&int64(discordgo.PermissionAdministrator) != 0
}

func defer_(s *discordgo.Session, i *discordgo.InteractionCreate, ephemeral bool) error {
	resp := &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredChannelMessageWithSource}
	if ephemeral {
		resp.Data = &discordgo.InteractionResponseData{Flags: discordgo.MessageFlagsEphemeral}
	}
	return s.InteractionRespond(i.Interaction, resp)
}

func editResponse(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	_, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &content})
	if err != nil {
		slog.Warn("slash_edit_response_failed", "err", err)
	}
}

func respondError(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}

func shortenErr(err error) string {
	msg := err.Error()
	if len(msg) > 400 {
		return msg[:400] + "…"
	}
	return msg
}
