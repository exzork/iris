package source

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Registry-level errors.
var (
	ErrSourceNotRegistered = errors.New("source host not registered")
	ErrDuplicateSource     = errors.New("source ID already registered")
	ErrMethodNotAllowed    = errors.New("access method not allowed for source")
)

// Source is a registered lore source bound to a specific host.
type Source struct {
	ID     string
	Host   string
	Policy Policy
}

// Validate checks that a Source has an ID, a host, and a valid Policy.
func (s *Source) Validate() error {
	if s == nil {
		return errors.New("nil source")
	}
	if strings.TrimSpace(s.ID) == "" {
		return errors.New("source missing ID")
	}
	if strings.TrimSpace(s.Host) == "" {
		return errors.New("source missing host")
	}
	return s.Policy.Validate()
}

// Registry is the authoritative set of allowed lore sources. It maps stable
// IDs to Source entries and answers access-control questions for ingestion
// components.
type Registry struct {
	sources map[string]*Source
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{sources: map[string]*Source{}}
}

// Register validates s and stores it. Returns ErrDuplicateSource if the ID
// already exists.
func (r *Registry) Register(s *Source) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if _, exists := r.sources[s.ID]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicateSource, s.ID)
	}
	copy := *s
	r.sources[s.ID] = &copy
	return nil
}

// Get returns the source with the given ID.
func (r *Registry) Get(id string) (*Source, bool) {
	s, ok := r.sources[id]
	return s, ok
}

// ByHost returns the first source whose Host matches (case-insensitive).
func (r *Registry) ByHost(host string) (*Source, bool) {
	host = strings.ToLower(strings.TrimSpace(host))
	for _, s := range r.sources {
		if strings.EqualFold(s.Host, host) {
			return s, true
		}
	}
	return nil, false
}

// List returns all sources sorted by ID.
func (r *Registry) List() []*Source {
	out := make([]*Source, 0, len(r.sources))
	for _, s := range r.sources {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// ValidateAccess reports whether the given host is registered and whether the
// requested access method is permitted by that source's policy. Fails closed:
// unknown hosts return ErrSourceNotRegistered.
func (r *Registry) ValidateAccess(host string, method AccessMethod) error {
	s, ok := r.ByHost(host)
	if !ok {
		return fmt.Errorf("%w: %s", ErrSourceNotRegistered, host)
	}
	if !s.Policy.Allows(method) {
		return fmt.Errorf("%w: host=%s method=%s", ErrMethodNotAllowed, host, method)
	}
	return nil
}

// DefaultRegistry returns a Registry pre-populated with the sources the bot
// is allowed to use out of the box. Currently only the Fandom Wuthering Waves
// wiki is registered. HTML scraping is intentionally excluded; use the
// MediaWiki API, an XML dump, or browser-assisted access instead.
func DefaultRegistry() *Registry {
	r := NewRegistry()
	if err := r.Register(fandomWutheringWaves()); err != nil {
		panic(fmt.Sprintf("default registry: %v", err))
	}
	return r
}

func fandomWutheringWaves() *Source {
	return &Source{
		ID:   "fandom_wutheringwaves",
		Host: "wutheringwaves.fandom.com",
		Policy: Policy{
			Name:           "Wuthering Waves Wiki (Fandom)",
			License:        "CC BY-SA 3.0",
			AttributionURL: "https://wutheringwaves.fandom.com/wiki/{page}",
			UserAgent:      "IrisBot/1.0 (+https://github.com/eko/iris-bot; contact: ops@example.invalid)",
			RateLimitRPS:   1.0,
			AllowedMethods: []AccessMethod{
				MethodMediaWikiAPI,
				MethodXMLDump,
				MethodBrowser,
			},
			RequiresAttribution: true,
			NotesURL:            "https://www.fandom.com/terms-of-use",
		},
	}
}
