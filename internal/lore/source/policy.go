// Package source defines the lore source registry and compliance policies.
//
// The registry is the single source of truth for which external lore hosts
// the bot is allowed to access, which access methods are permitted per host,
// and what attribution / user-agent / rate-limit rules apply.
//
// This package deliberately contains NO fetcher, crawler, or HTTP client.
// Ingestion components must consult Registry.ValidateAccess before making
// any outbound request. See docs/wiki-compliance.md for the human-readable
// decision document.
package source

import "errors"

// AccessMethod identifies how a source may be queried.
type AccessMethod string

const (
	// MethodMediaWikiAPI uses the MediaWiki web API (action=query, action=parse, etc.).
	MethodMediaWikiAPI AccessMethod = "mediawiki_api"
	// MethodXMLDump consumes an offline XML dump (pages-articles export).
	MethodXMLDump AccessMethod = "xml_dump"
	// MethodBrowser uses a headless browser (Camoufox) for JS-heavy pages.
	MethodBrowser AccessMethod = "browser"
	// MethodHTMLScrape fetches rendered HTML directly. Forbidden unless a
	// source explicitly registers it; prefer API or dump instead.
	MethodHTMLScrape AccessMethod = "html_scrape"
)

// Policy describes the legal and operational rules that govern a source.
type Policy struct {
	// Name is the human-readable source name, e.g. "Wuthering Waves Wiki".
	Name string
	// License is the content license, e.g. "CC BY-SA 3.0".
	License string
	// AttributionURL is the URL template or exact URL used in citations.
	// Templates may contain a {page} placeholder.
	AttributionURL string
	// UserAgent is the required UA string. Must identify the bot and a
	// contact address.
	UserAgent string
	// RateLimitRPS is the max requests-per-second allowed against this source.
	RateLimitRPS float64
	// AllowedMethods enumerates which AccessMethod values are permitted.
	AllowedMethods []AccessMethod
	// RequiresAttribution is true when user-visible output must cite the source.
	RequiresAttribution bool
	// NotesURL links to the source's ToU or policy page.
	NotesURL string
}

// Policy-level validation errors.
var (
	ErrMissingLicense     = errors.New("source policy missing license")
	ErrMissingAttribution = errors.New("source policy missing attribution URL")
	ErrMissingUserAgent   = errors.New("source policy missing user-agent")
	ErrNoAllowedMethods   = errors.New("source policy must allow at least one access method")
	ErrMissingName        = errors.New("source policy missing name")
)

// Validate checks that a Policy has every field required for compliant access.
func (p *Policy) Validate() error {
	if p == nil {
		return ErrMissingName
	}
	if p.Name == "" {
		return ErrMissingName
	}
	if p.License == "" {
		return ErrMissingLicense
	}
	if p.AttributionURL == "" {
		return ErrMissingAttribution
	}
	if p.UserAgent == "" {
		return ErrMissingUserAgent
	}
	if len(p.AllowedMethods) == 0 {
		return ErrNoAllowedMethods
	}
	return nil
}

// Allows reports whether the policy permits the given access method.
func (p *Policy) Allows(m AccessMethod) bool {
	for _, allowed := range p.AllowedMethods {
		if allowed == m {
			return true
		}
	}
	return false
}
