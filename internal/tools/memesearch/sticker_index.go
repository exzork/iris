package memesearch

import (
	"context"
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
)

type GuildStickerIndex struct {
	Session *discordgo.Session
}

func NewGuildStickerIndex(s *discordgo.Session) *GuildStickerIndex {
	return &GuildStickerIndex{Session: s}
}

func (g *GuildStickerIndex) Search(ctx context.Context, guildID int64, query string, limit int) ([]MediaItem, error) {
	if g.Session == nil || guildID == 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 3
	}

	guildIDStr := fmt.Sprintf("%d", guildID)
	guild, err := g.Session.State.Guild(guildIDStr)
	if err != nil || guild == nil {
		return nil, nil
	}

	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil, nil
	}
	queryTokens := strings.Fields(q)

	results := make([]MediaItem, 0, limit)
	for _, sticker := range guild.Stickers {
		if sticker == nil || !sticker.Available {
			continue
		}
		haystack := strings.ToLower(sticker.Name + " " + sticker.Description + " " + sticker.Tags)
		if !matchesAllTokens(haystack, queryTokens) {
			continue
		}
		results = append(results, MediaItem{
			URL:       stickerCDNURL(sticker),
			Source:    SourceGuildSticker,
			MimeType:  stickerMIME(sticker.FormatType),
			Caption:   sticker.Name,
			Safety:    SafetySafe,
			StickerID: sticker.ID,
		})
		if len(results) >= limit {
			break
		}
	}
	return results, nil
}

func matchesAllTokens(haystack string, tokens []string) bool {
	for _, t := range tokens {
		if t == "" {
			continue
		}
		if !strings.Contains(haystack, t) {
			return false
		}
	}
	return true
}

func stickerCDNURL(s *discordgo.Sticker) string {
	if s == nil {
		return ""
	}
	switch s.FormatType {
	case discordgo.StickerFormatTypePNG, discordgo.StickerFormatTypeAPNG:
		return "https://cdn.discordapp.com/stickers/" + s.ID + ".png"
	case discordgo.StickerFormatTypeLottie:
		return "https://cdn.discordapp.com/stickers/" + s.ID + ".json"
	case discordgo.StickerFormatTypeGIF:
		return "https://cdn.discordapp.com/stickers/" + s.ID + ".gif"
	default:
		return "https://cdn.discordapp.com/stickers/" + s.ID + ".png"
	}
}

func stickerMIME(format discordgo.StickerFormat) string {
	switch format {
	case discordgo.StickerFormatTypeAPNG:
		return "image/apng"
	case discordgo.StickerFormatTypeLottie:
		return "application/json"
	case discordgo.StickerFormatTypeGIF:
		return "image/gif"
	default:
		return "image/png"
	}
}
