package slash

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/bwmarrin/discordgo"
)

// SessionAPI is the subset of *discordgo.Session the registrar needs. Kept
// tight so tests can stub it.
type SessionAPI interface {
	ApplicationCommandBulkOverwrite(appID, guildID string, commands []*discordgo.ApplicationCommand, options ...discordgo.RequestOption) ([]*discordgo.ApplicationCommand, error)
}

// NativeCommand is a hand-written slash command that executes server-side
// Go code rather than proxying to a tool. Native commands never carry
// SchemaPath mappings; they access interaction options directly in Execute.
type NativeCommand struct {
	Name        string
	Description string
	Options     []*discordgo.ApplicationCommandOption
	AdminOnly   bool
	Execute     func(ctx context.Context, inv *NativeInvocation) (string, error)
}

// NativeInvocation is the context a native command receives.
type NativeInvocation struct {
	GuildID     int64
	ChannelID   int64
	UserID      int64
	IsAdmin     bool
	Options     []*discordgo.ApplicationCommandInteractionDataOption
}

// Registrar converts bindings + native commands into Discord application
// commands and performs per-guild BulkOverwrite registration.
type Registrar struct {
	appID          string
	session        SessionAPI
	bindingsOnce   sync.Mutex
	bindingsSrc    BindingProvider
	natives        []NativeCommand
	knownGuildsMu  sync.Mutex
	knownGuilds    map[int64]struct{}
}

func NewRegistrar(bindings BindingProvider, natives []NativeCommand) *Registrar {
	return &Registrar{
		bindingsSrc: bindings,
		natives:     natives,
		knownGuilds: make(map[int64]struct{}),
	}
}

// RegisterGuild performs BulkOverwrite for one guild. Called on Ready (for
// each guild the bot sees) and on GuildCreate (for runtime joins). If the
// registrar has not been Attach'd yet (because the bot ID isn't known until
// after Session.Open), the guild ID is buffered and flushed at Attach time.
func (r *Registrar) RegisterGuild(ctx context.Context, guildID int64) error {
	if r.session == nil || r.appID == "" {
		r.knownGuildsMu.Lock()
		r.knownGuilds[guildID] = struct{}{}
		r.knownGuildsMu.Unlock()
		slog.Debug("slash_register_deferred", "guild", guildID)
		return nil
	}
	cmds := r.Commands()
	gid := fmt.Sprintf("%d", guildID)
	_, err := r.session.ApplicationCommandBulkOverwrite(r.appID, gid, cmds)
	if err != nil {
		return fmt.Errorf("slash: bulk overwrite guild %d: %w", guildID, err)
	}
	r.knownGuildsMu.Lock()
	r.knownGuilds[guildID] = struct{}{}
	r.knownGuildsMu.Unlock()
	slog.Info("slash_commands_registered", "guild", guildID, "count", len(cmds))
	return nil
}

// Attach binds a discord session and application id, then flushes any guilds
// that came in before attachment so their commands register immediately.
func (r *Registrar) Attach(session SessionAPI, appID string) {
	r.session = session
	r.appID = appID
	r.ReloadAll(context.Background())
}

// Commands returns the current merged application-command slice: defaults +
// user bindings + native commands. Used by tests and by the registrar
// itself when calling BulkOverwrite.
func (r *Registrar) Commands() []*discordgo.ApplicationCommand {
	r.bindingsOnce.Lock()
	defer r.bindingsOnce.Unlock()

	user := r.bindingsSrc.SlashBindings()
	merged := MergeWithDefaults(user)

	out := make([]*discordgo.ApplicationCommand, 0, len(merged)+len(r.natives))
	for name, b := range merged {
		cmd := bindingToCommand(name, b)
		if cmd != nil {
			out = append(out, cmd)
		}
	}
	for _, n := range r.natives {
		out = append(out, nativeToCommand(n))
	}
	return out
}

// ReloadAll re-registers the current command set on every guild the
// registrar has previously seen. Used after a binding change so new slash
// commands appear without a bot restart.
func (r *Registrar) ReloadAll(ctx context.Context) {
	r.knownGuildsMu.Lock()
	guilds := make([]int64, 0, len(r.knownGuilds))
	for g := range r.knownGuilds {
		guilds = append(guilds, g)
	}
	r.knownGuildsMu.Unlock()

	for _, g := range guilds {
		if err := r.RegisterGuild(ctx, g); err != nil {
			slog.Warn("slash_reload_failed", "guild", g, "err", err)
		}
	}
}

// NativeByName returns the native command definition for routing, or nil if
// no native command owns that name.
func (r *Registrar) NativeByName(name string) *NativeCommand {
	for i := range r.natives {
		if r.natives[i].Name == name {
			return &r.natives[i]
		}
	}
	return nil
}

// BindingByName returns the user/default binding for routing, or nil + false
// if no binding owns that name.
func (r *Registrar) BindingByName(name string) (Binding, bool) {
	user := r.bindingsSrc.SlashBindings()
	merged := MergeWithDefaults(user)
	b, ok := merged[name]
	return b, ok
}

func bindingToCommand(name string, b Binding) *discordgo.ApplicationCommand {
	cmd := &discordgo.ApplicationCommand{
		Name:        name,
		Description: b.Description,
		Type:        discordgo.ChatApplicationCommand,
	}
	for _, opt := range b.Options {
		dt, ok := OptionTypeToDiscord(opt.Type)
		if !ok {
			continue
		}
		cmd.Options = append(cmd.Options, &discordgo.ApplicationCommandOption{
			Type:        dt,
			Name:        opt.Name,
			Description: opt.Description,
			Required:    opt.Required,
		})
	}
	return cmd
}

func nativeToCommand(n NativeCommand) *discordgo.ApplicationCommand {
	cmd := &discordgo.ApplicationCommand{
		Name:        n.Name,
		Description: n.Description,
		Type:        discordgo.ChatApplicationCommand,
		Options:     n.Options,
	}
	if n.AdminOnly {
		perms := int64(discordgo.PermissionAdministrator)
		cmd.DefaultMemberPermissions = &perms
	}
	return cmd
}
