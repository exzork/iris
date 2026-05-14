package admin

import "context"

// CommandContext holds the context for a command execution.
type CommandContext struct {
	GuildID     int64
	UserID      int64
	ChannelID   int64
	Raw         string
	IsOwner     bool
	Permissions int64
	RoleIDs     []int64
}

// Handler processes a command and returns a response.
type Handler interface {
	Handle(ctx context.Context, cmd *CommandContext, args []string) (response string, err error)
}

// HandlerFunc is a function adapter for Handler.
type HandlerFunc func(ctx context.Context, cmd *CommandContext, args []string) (string, error)

func (f HandlerFunc) Handle(ctx context.Context, cmd *CommandContext, args []string) (string, error) {
	return f(ctx, cmd, args)
}

// Dispatcher routes commands to handlers.
type Dispatcher struct {
	handlers map[string]Handler
}

// NewDispatcher creates a new command dispatcher.
func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		handlers: make(map[string]Handler),
	}
}

// Register registers a handler for a command verb.
func (d *Dispatcher) Register(name string, h Handler) {
	d.handlers[name] = h
}

// ParsedCommand represents a parsed command.
type ParsedCommand struct {
	Verb string
	Sub  string
	Args []string
}

// Dispatch routes a command to its handler.
func (d *Dispatcher) Dispatch(ctx context.Context, cmd *CommandContext) (string, error) {
	parsed := parseCommand(cmd.Raw)
	if parsed == nil {
		return "Format perintah salah. Gunakan `!iris help` untuk melihat daftar perintah.", nil
	}

	handler, ok := d.handlers[parsed.Verb]
	if !ok {
		return "Perintah tidak dikenali. Gunakan `!iris help` untuk melihat daftar perintah.", nil
	}

	allArgs := []string{parsed.Sub}
	allArgs = append(allArgs, parsed.Args...)
	return handler.Handle(ctx, cmd, allArgs)
}

// parseCommand parses a raw command string into verb, sub, and args.
// Format: "!iris verb [sub] [args...]"
// Example: "!iris exception add 999" -> {verb: "exception", sub: "add", args: ["999"]}
func parseCommand(raw string) *ParsedCommand {
	if raw == "" {
		return nil
	}

	// Simple tokenizer that respects quoted strings
	tokens := tokenize(raw)
	if len(tokens) < 2 {
		return nil
	}

	// Skip "!iris" prefix
	if tokens[0] != "!iris" {
		return nil
	}

	verb := tokens[1]
	var sub string
	var args []string

	if len(tokens) > 2 {
		sub = tokens[2]
		args = tokens[3:]
	}

	return &ParsedCommand{
		Verb: verb,
		Sub:  sub,
		Args: args,
	}
}

func tokenize(s string) []string {
	var tokens []string
	var current string
	inQuotes := false

	for i := 0; i < len(s); i++ {
		ch := s[i]

		if ch == '"' {
			inQuotes = !inQuotes
			continue
		}

		if ch == ' ' && !inQuotes {
			if current != "" {
				tokens = append(tokens, current)
				current = ""
			}
			continue
		}

		current += string(ch)
	}

	if current != "" {
		tokens = append(tokens, current)
	}

	return tokens
}

// Store interfaces for dependency injection in tests.

// ExceptionChannelStore defines operations on exception channels.
type ExceptionChannelStore interface {
	Add(ctx context.Context, guildID, channelID int64) error
	Remove(ctx context.Context, guildID, channelID int64) error
	List(ctx context.Context, guildID int64) ([]int64, error)
}

// SettingsStore defines operations on guild settings.
type SettingsStore interface {
	Get(ctx context.Context, guildID int64, key string) (string, bool, error)
	Set(ctx context.Context, guildID int64, key, value string) error
	List(ctx context.Context, guildID int64) (map[string]string, error)
}

// AuditLogger defines audit logging operations.
type AuditLogger interface {
	Log(ctx context.Context, guildID, userID int64, eventType, entityType, entityID string, changes map[string]interface{}) error
}
