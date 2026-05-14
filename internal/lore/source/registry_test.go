package source

import (
	"errors"
	"sort"
	"testing"
)

func TestDefaultRegistryFandomPolicy(t *testing.T) {
	r := DefaultRegistry()

	s, ok := r.Get("fandom_wutheringwaves")
	if !ok {
		t.Fatal("expected fandom_wutheringwaves source registered")
	}
	if s.Host != "wutheringwaves.fandom.com" {
		t.Errorf("host mismatch: %q", s.Host)
	}
	if s.Policy.License != "CC BY-SA 3.0" {
		t.Errorf("license mismatch: %q", s.Policy.License)
	}
	if !s.Policy.RequiresAttribution {
		t.Error("fandom must require attribution")
	}
	if s.Policy.UserAgent == "" {
		t.Error("fandom must declare a user-agent")
	}
	if s.Policy.AttributionURL == "" {
		t.Error("fandom must declare an attribution URL")
	}
	if s.Policy.RateLimitRPS <= 0 {
		t.Error("fandom rate limit must be positive")
	}

	wantAllowed := []AccessMethod{MethodMediaWikiAPI, MethodXMLDump, MethodBrowser}
	for _, m := range wantAllowed {
		if !s.Policy.Allows(m) {
			t.Errorf("fandom must allow %s", m)
		}
	}
	if s.Policy.Allows(MethodHTMLScrape) {
		t.Error("fandom must NOT allow html_scrape")
	}
}

func TestRegisterValidatesRequiredFields(t *testing.T) {
	base := func() *Source {
		return &Source{
			ID:   "x",
			Host: "x.invalid",
			Policy: Policy{
				Name:           "X",
				License:        "CC0",
				AttributionURL: "https://x.invalid/{page}",
				UserAgent:      "IrisBot/1.0",
				AllowedMethods: []AccessMethod{MethodMediaWikiAPI},
			},
		}
	}

	cases := []struct {
		name    string
		mutate  func(s *Source)
		wantErr error
	}{
		{"missing license", func(s *Source) { s.Policy.License = "" }, ErrMissingLicense},
		{"missing attribution", func(s *Source) { s.Policy.AttributionURL = "" }, ErrMissingAttribution},
		{"missing user agent", func(s *Source) { s.Policy.UserAgent = "" }, ErrMissingUserAgent},
		{"no methods", func(s *Source) { s.Policy.AllowedMethods = nil }, ErrNoAllowedMethods},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := NewRegistry()
			s := base()
			tc.mutate(s)
			err := r.Register(s)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("want %v, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestRegisterDuplicateRejected(t *testing.T) {
	r := DefaultRegistry()
	dup := &Source{
		ID:   "fandom_wutheringwaves",
		Host: "other.invalid",
		Policy: Policy{
			Name:           "dup",
			License:        "CC0",
			AttributionURL: "https://other.invalid/{page}",
			UserAgent:      "IrisBot/1.0",
			AllowedMethods: []AccessMethod{MethodMediaWikiAPI},
		},
	}
	err := r.Register(dup)
	if !errors.Is(err, ErrDuplicateSource) {
		t.Fatalf("want ErrDuplicateSource, got %v", err)
	}
}

func TestByHostFindsFandom(t *testing.T) {
	r := DefaultRegistry()
	s, ok := r.ByHost("wutheringwaves.fandom.com")
	if !ok {
		t.Fatal("expected to find fandom by host")
	}
	if s.ID != "fandom_wutheringwaves" {
		t.Errorf("unexpected ID %q", s.ID)
	}

	if _, ok := r.ByHost("WUTHERINGWAVES.FANDOM.COM"); !ok {
		t.Error("ByHost must be case-insensitive")
	}
}

func TestValidateAccessAllowed(t *testing.T) {
	r := DefaultRegistry()
	if err := r.ValidateAccess("wutheringwaves.fandom.com", MethodMediaWikiAPI); err != nil {
		t.Fatalf("expected API allowed, got %v", err)
	}
	if err := r.ValidateAccess("wutheringwaves.fandom.com", MethodXMLDump); err != nil {
		t.Fatalf("expected dump allowed, got %v", err)
	}
	if err := r.ValidateAccess("wutheringwaves.fandom.com", MethodBrowser); err != nil {
		t.Fatalf("expected browser allowed, got %v", err)
	}
}

func TestValidateAccessMethodNotAllowed(t *testing.T) {
	r := DefaultRegistry()
	err := r.ValidateAccess("wutheringwaves.fandom.com", MethodHTMLScrape)
	if !errors.Is(err, ErrMethodNotAllowed) {
		t.Fatalf("want ErrMethodNotAllowed, got %v", err)
	}
}

func TestValidateAccessUnregistered(t *testing.T) {
	r := DefaultRegistry()
	err := r.ValidateAccess("evil.example.invalid", MethodMediaWikiAPI)
	if !errors.Is(err, ErrSourceNotRegistered) {
		t.Fatalf("want ErrSourceNotRegistered, got %v", err)
	}
}

func TestListSortedByID(t *testing.T) {
	r := NewRegistry()
	ids := []string{"zeta", "alpha", "mike"}
	for _, id := range ids {
		s := &Source{
			ID:   id,
			Host: id + ".invalid",
			Policy: Policy{
				Name:           id,
				License:        "CC0",
				AttributionURL: "https://" + id + ".invalid/{page}",
				UserAgent:      "IrisBot/1.0",
				AllowedMethods: []AccessMethod{MethodMediaWikiAPI},
			},
		}
		if err := r.Register(s); err != nil {
			t.Fatalf("register %s: %v", id, err)
		}
	}

	got := r.List()
	gotIDs := make([]string, len(got))
	for i, s := range got {
		gotIDs[i] = s.ID
	}
	want := []string{"alpha", "mike", "zeta"}
	if !sort.StringsAreSorted(gotIDs) {
		t.Errorf("List not sorted: %v", gotIDs)
	}
	for i := range want {
		if gotIDs[i] != want[i] {
			t.Errorf("position %d: want %q, got %q", i, want[i], gotIDs[i])
		}
	}
}
