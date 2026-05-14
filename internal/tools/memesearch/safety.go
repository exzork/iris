package memesearch

import (
	"strings"
)

type SafetyClassifier interface {
	Classify(item MediaItem) Safety
}

type DefaultSafetyClassifier struct {
	nsfwKeywords []string
	safeHosts    []string
}

func NewDefaultSafetyClassifier() *DefaultSafetyClassifier {
	return &DefaultSafetyClassifier{
		nsfwKeywords: []string{"nsfw", "porn", "nude", "xxx", "explicit", "18+", "adult"},
		safeHosts:    []string{"tenor.com", "giphy.com", "media.discordapp.net"},
	}
}

func (c *DefaultSafetyClassifier) Classify(item MediaItem) Safety {
	urlLower := strings.ToLower(item.URL)
	captionLower := strings.ToLower(item.Caption)

	for _, keyword := range c.nsfwKeywords {
		if strings.Contains(urlLower, keyword) || strings.Contains(captionLower, keyword) {
			return SafetyNSFW
		}
	}

	for _, host := range c.safeHosts {
		if strings.Contains(urlLower, host) {
			return SafetySafe
		}
	}

	return SafetyUnknown
}
