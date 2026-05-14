package websearch

import (
	"net/url"
	"strings"
)

var canonicalHosts = map[string]bool{
	"wutheringwaves.fandom.com": true,
	"kurogames.com":             true,
}

func IsCanonAuthoritative(rawURL string) bool {
	if rawURL == "" {
		return false
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	host := strings.ToLower(u.Host)
	return canonicalHosts[host]
}
