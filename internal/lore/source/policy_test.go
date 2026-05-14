package source

import (
	"errors"
	"testing"
)

func TestPolicyValidateOK(t *testing.T) {
	p := Policy{
		Name:           "Test",
		License:        "CC BY-SA 3.0",
		AttributionURL: "https://example.invalid/{page}",
		UserAgent:      "IrisBot/1.0",
		AllowedMethods: []AccessMethod{MethodMediaWikiAPI},
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("expected valid policy, got %v", err)
	}
}

func TestPolicyValidateMissingEach(t *testing.T) {
	base := Policy{
		Name:           "Test",
		License:        "CC BY-SA 3.0",
		AttributionURL: "https://example.invalid/{page}",
		UserAgent:      "IrisBot/1.0",
		AllowedMethods: []AccessMethod{MethodMediaWikiAPI},
	}

	cases := []struct {
		name    string
		mutate  func(p *Policy)
		wantErr error
	}{
		{"missing name", func(p *Policy) { p.Name = "" }, ErrMissingName},
		{"missing license", func(p *Policy) { p.License = "" }, ErrMissingLicense},
		{"missing attribution", func(p *Policy) { p.AttributionURL = "" }, ErrMissingAttribution},
		{"missing user agent", func(p *Policy) { p.UserAgent = "" }, ErrMissingUserAgent},
		{"no methods", func(p *Policy) { p.AllowedMethods = nil }, ErrNoAllowedMethods},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := base
			tc.mutate(&p)
			err := p.Validate()
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("want %v, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestPolicyAllows(t *testing.T) {
	p := Policy{AllowedMethods: []AccessMethod{MethodMediaWikiAPI, MethodXMLDump}}
	if !p.Allows(MethodMediaWikiAPI) {
		t.Error("expected API allowed")
	}
	if p.Allows(MethodHTMLScrape) {
		t.Error("expected html_scrape not allowed")
	}
}
