package patchnotes

import (
	"net/url"
	"strings"
)

// ClassifySource returns the SourceLevel based on the URL host.
// Official: kurogames.com, wutheringwaves.com
// Wiki: wutheringwaves.fandom.com
// Community: reddit.com, x.com, twitter.com, youtube.com, and anything else
func ClassifySource(rawURL string) SourceLevel {
	u, err := url.Parse(rawURL)
	if err != nil {
		return LevelCommunity
	}

	host := strings.ToLower(u.Host)
	host = strings.TrimPrefix(host, "www.")

	switch {
	case strings.Contains(host, "kurogames.com"):
		return LevelOfficial
	case strings.Contains(host, "wutheringwaves.com"):
		return LevelOfficial
	case strings.Contains(host, "wutheringwaves.fandom.com"):
		return LevelWiki
	case strings.Contains(host, "reddit.com"):
		return LevelCommunity
	case strings.Contains(host, "x.com"):
		return LevelCommunity
	case strings.Contains(host, "twitter.com"):
		return LevelCommunity
	case strings.Contains(host, "youtube.com"):
		return LevelCommunity
	default:
		return LevelCommunity
	}
}
