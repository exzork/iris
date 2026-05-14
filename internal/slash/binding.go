package slash

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"
)

type OptionType string

const (
	OptionTypeString  OptionType = "string"
	OptionTypeInteger OptionType = "integer"
	OptionTypeBoolean OptionType = "boolean"
	OptionTypeUser    OptionType = "user"
	OptionTypeChannel OptionType = "channel"
	OptionTypeRole    OptionType = "role"
)

type Option struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Type        OptionType `json:"type"`
	Required    bool       `json:"required,omitempty"`
	SchemaPath  string     `json:"schemaPath"`
}

type Binding struct {
	Tool        string   `json:"tool"`
	Description string   `json:"description"`
	Options     []Option `json:"options,omitempty"`
	Ephemeral   bool     `json:"ephemeral,omitempty"`
	AdminOnly   bool     `json:"adminOnly,omitempty"`
	OwnerOnly   bool     `json:"ownerOnly,omitempty"`
}

type Config struct {
	MCPServers    map[string]json.RawMessage `json:"mcpServers,omitempty"`
	SlashCommands map[string]Binding         `json:"slashCommands,omitempty"`
}

type BindingProvider interface {
	SlashBindings() map[string]Binding
}

type StaticProvider struct {
	mu       sync.RWMutex
	bindings map[string]Binding
}

func NewStaticProvider(bindings map[string]Binding) *StaticProvider {
	return &StaticProvider{bindings: cloneBindings(bindings)}
}

func (p *StaticProvider) SlashBindings() map[string]Binding {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return cloneBindings(p.bindings)
}

func (p *StaticProvider) Set(bindings map[string]Binding) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.bindings = cloneBindings(bindings)
}

func DefaultBindings() map[string]Binding {
	return map[string]Binding{
		"search": {
			Tool:        "web_search",
			Description: "Cari informasi di web.",
			Options: []Option{
				{
					Name:        "query",
					Description: "Apa yang mau dicari",
					Type:        OptionTypeString,
					Required:    true,
					SchemaPath:  "query",
				},
			},
			Ephemeral: false,
		},
	}
}

func MergeWithDefaults(bindings map[string]Binding) map[string]Binding {
	out := cloneBindings(DefaultBindings())
	for name, b := range bindings {
		out[name] = cloneBinding(b)
	}
	return out
}

func ParseOptionType(raw string) (OptionType, error) {
	v := OptionType(strings.ToLower(strings.TrimSpace(raw)))
	switch v {
	case OptionTypeString, OptionTypeInteger, OptionTypeBoolean, OptionTypeUser, OptionTypeChannel, OptionTypeRole:
		return v, nil
	default:
		return "", fmt.Errorf("slash: unsupported option type %q", raw)
	}
}

func OptionTypeToDiscord(t OptionType) (discordgo.ApplicationCommandOptionType, bool) {
	switch t {
	case OptionTypeString:
		return discordgo.ApplicationCommandOptionString, true
	case OptionTypeInteger:
		return discordgo.ApplicationCommandOptionInteger, true
	case OptionTypeBoolean:
		return discordgo.ApplicationCommandOptionBoolean, true
	case OptionTypeUser:
		return discordgo.ApplicationCommandOptionUser, true
	case OptionTypeChannel:
		return discordgo.ApplicationCommandOptionChannel, true
	case OptionTypeRole:
		return discordgo.ApplicationCommandOptionRole, true
	default:
		return 0, false
	}
}

func ValidateBinding(name string, b Binding) error {
	if !isValidCommandName(name) {
		return fmt.Errorf("slash: invalid command name %q", name)
	}
	if strings.TrimSpace(b.Tool) == "" {
		return fmt.Errorf("slash: binding %q tool is required", name)
	}
	if strings.TrimSpace(b.Description) == "" {
		return fmt.Errorf("slash: binding %q description is required", name)
	}
	seen := make(map[string]bool, len(b.Options))
	for i, opt := range b.Options {
		if !isValidCommandName(opt.Name) {
			return fmt.Errorf("slash: binding %q option[%d] has invalid name %q", name, i, opt.Name)
		}
		if strings.TrimSpace(opt.Description) == "" {
			return fmt.Errorf("slash: binding %q option %q description is required", name, opt.Name)
		}
		if _, ok := OptionTypeToDiscord(opt.Type); !ok {
			return fmt.Errorf("slash: binding %q option %q has invalid type %q", name, opt.Name, opt.Type)
		}
		if strings.TrimSpace(opt.SchemaPath) == "" {
			return fmt.Errorf("slash: binding %q option %q schemaPath is required", name, opt.Name)
		}
		if seen[opt.Name] {
			return fmt.Errorf("slash: binding %q has duplicate option name %q", name, opt.Name)
		}
		seen[opt.Name] = true
	}
	return nil
}

func SanitizeCommandName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return ""
	}
	b := make([]byte, 0, len(name))
	prevDash := false
	for i := 0; i < len(name); i++ {
		c := name[i]
		valid := (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' || c == '-'
		if !valid {
			c = '-'
		}
		if c == '-' {
			if prevDash {
				continue
			}
			prevDash = true
		} else {
			prevDash = false
		}
		b = append(b, c)
		if len(b) == 32 {
			break
		}
	}
	out := strings.Trim(string(b), "-")
	if len(out) > 32 {
		out = out[:32]
	}
	return out
}

func isValidCommandName(name string) bool {
	if len(name) < 1 || len(name) > 32 {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' || c == '-' {
			continue
		}
		return false
	}
	return true
}

func cloneBinding(in Binding) Binding {
	out := in
	if len(in.Options) > 0 {
		out.Options = append([]Option(nil), in.Options...)
	}
	return out
}

func cloneBindings(in map[string]Binding) map[string]Binding {
	out := make(map[string]Binding, len(in))
	for k, v := range in {
		out[k] = cloneBinding(v)
	}
	return out
}

type Store struct {
	path    string
	mu      sync.Mutex
	cfg     Config
	onMutate func()
}

func NewStore(path string) (*Store, error) {
	cfg, err := LoadConfig(path)
	if err != nil {
		return nil, err
	}
	return &Store{path: path, cfg: cfg}, nil
}

func LoadConfig(path string) (Config, error) {
	var cfg Config
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg.MCPServers = make(map[string]json.RawMessage)
			cfg.SlashCommands = make(map[string]Binding)
			return cfg, nil
		}
		return cfg, fmt.Errorf("slash: read %s: %w", path, err)
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return cfg, fmt.Errorf("slash: parse %s: %w", path, err)
	}
	if cfg.MCPServers == nil {
		cfg.MCPServers = make(map[string]json.RawMessage)
	}
	if cfg.SlashCommands == nil {
		cfg.SlashCommands = make(map[string]Binding)
	}
	for name, b := range cfg.SlashCommands {
		if err := ValidateBinding(name, b); err != nil {
			return cfg, err
		}
	}
	return cfg, nil
}

func (s *Store) SlashBindings() map[string]Binding {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneBindings(s.cfg.SlashCommands)
}

func (s *Store) SortedSlashNames() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	names := make([]string, 0, len(s.cfg.SlashCommands))
	for name := range s.cfg.SlashCommands {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (s *Store) Set(name string, binding Binding) error {
	if err := ValidateBinding(name, binding); err != nil {
		return err
	}
	s.mu.Lock()
	if s.cfg.SlashCommands == nil {
		s.cfg.SlashCommands = make(map[string]Binding)
	}
	prev, hadPrev := s.cfg.SlashCommands[name]
	s.cfg.SlashCommands[name] = cloneBinding(binding)
	if err := s.persistLocked(); err != nil {
		if hadPrev {
			s.cfg.SlashCommands[name] = prev
		} else {
			delete(s.cfg.SlashCommands, name)
		}
		s.mu.Unlock()
		return err
	}
	hook := s.onMutate
	s.mu.Unlock()
	if hook != nil {
		hook()
	}
	return nil
}

func (s *Store) Delete(name string) error {
	s.mu.Lock()
	if s.cfg.SlashCommands == nil {
		s.cfg.SlashCommands = make(map[string]Binding)
		s.mu.Unlock()
		return nil
	}
	prev, hadPrev := s.cfg.SlashCommands[name]
	if !hadPrev {
		s.mu.Unlock()
		return fmt.Errorf("slash: binding %q not found", name)
	}
	delete(s.cfg.SlashCommands, name)
	if err := s.persistLocked(); err != nil {
		s.cfg.SlashCommands[name] = prev
		s.mu.Unlock()
		return err
	}
	hook := s.onMutate
	s.mu.Unlock()
	if hook != nil {
		hook()
	}
	return nil
}

// OnMutate registers a callback fired after any successful Set/Delete. Used
// by the registrar to trigger a per-guild BulkOverwrite reload so new slash
// commands appear in Discord without a bot restart.
func (s *Store) OnMutate(hook func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onMutate = hook
}

func (s *Store) Reload() error {
	cfg, err := LoadConfig(s.path)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg = cfg
	return nil
}

func (s *Store) persistLocked() error {
	raw, err := json.MarshalIndent(s.cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("slash: marshal config: %w", err)
	}
	raw = append(raw, '\n')
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return fmt.Errorf("slash: write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("slash: rename %s -> %s: %w", tmp, s.path, err)
	}
	return nil
}
