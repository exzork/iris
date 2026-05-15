package memesearch

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/eko/iris-bot/internal/tools"
)

type Tool struct {
	DiscordIndex DiscordMediaIndex
	StickerIndex DiscordMediaIndex
	Social       []SocialAdapter
	Safety       SafetyClassifier
}

func New(idx DiscordMediaIndex, stickers DiscordMediaIndex, social []SocialAdapter, safety SafetyClassifier) *Tool {
	return &Tool{
		DiscordIndex: idx,
		StickerIndex: stickers,
		Social:       social,
		Safety:       safety,
	}
}

func (t *Tool) Schema() *tools.Schema {
	return &tools.Schema{
		Name:        "meme_search",
		Description: "Search for a reaction GIF or sticker to attach to your reply. Sources: GIPHY (general reaction GIFs), guild stickers (from this server), Discord history media. Use this tool when a reaction GIF or sticker would amplify the reply tone (excitement, dismissal, agreement, comedic timing). The chat reply must still carry the actual answer; the GIF/sticker is decoration. Pick concise emotional keywords as the query (e.g. 'sad cat', 'mind blown', 'thinking') rather than restating the user's full message. The tool returns several randomized candidates from the top results so successive calls vary - prefer to pick a candidate at random rather than always the first; if the user asks for a different GIF, just call the tool again instead of cycling through the same list.",
		Fields: []tools.FieldSpec{
			{
				Name:        "query",
				Kind:        tools.KindString,
				Required:    true,
				Description: "short emotional keywords describing the reaction (e.g. 'mind blown', 'happy birthday', 'thinking', 'rover salute')",
			},
			{
				Name:        "guild_id",
				Kind:        tools.KindNumber,
				Required:    true,
				Description: "Discord guild id; required for guild-scoped sticker and history search",
			},
			{
				Name:        "limit",
				Kind:        tools.KindNumber,
				Required:    false,
				Description: "max randomized candidates to return; default 5, max 10",
			},
		},
	}
}

func (t *Tool) Run(ctx context.Context, args map[string]interface{}) (string, error) {
	query, ok := args["query"].(string)
	if !ok {
		return "", fmt.Errorf("query must be a string")
	}

	query = strings.TrimSpace(query)
	if query == "" {
		return "", fmt.Errorf("query is required")
	}

	guildIDVal, ok := args["guild_id"]
	if !ok {
		return "", fmt.Errorf("guild_id is required")
	}

	var guildID int64
	switch v := guildIDVal.(type) {
	case float64:
		guildID = int64(v)
	case int:
		guildID = int64(v)
	case int64:
		guildID = v
	default:
		return "", fmt.Errorf("guild_id must be a number")
	}

	limit := 5
	if limitVal, ok := args["limit"]; ok {
		switch v := limitVal.(type) {
		case float64:
			limit = int(v)
		case int:
			limit = v
		}
	}

	if limit < 1 {
		limit = 5
	}
	if limit > 10 {
		limit = 10
	}

	results := make([]MediaItem, 0)

	if t.StickerIndex != nil {
		stickerResults, err := t.StickerIndex.Search(ctx, guildID, query, limit)
		if err == nil {
			for _, item := range stickerResults {
				if t.Safety.Classify(item) != SafetySafe {
					continue
				}
				results = append(results, item)
				if len(results) >= limit {
					break
				}
			}
		}
	}

	if len(results) < limit && t.DiscordIndex != nil {
		discordResults, err := t.DiscordIndex.Search(ctx, guildID, query, limit-len(results))
		if err == nil {
			for _, item := range discordResults {
				classified := t.Safety.Classify(item)
				if classified != SafetySafe {
					continue
				}
				item.Safety = classified
				results = append(results, item)
				if len(results) >= limit {
					break
				}
			}
		}
	}

	if len(results) < limit {
		for _, adapter := range t.Social {
			if len(results) >= limit {
				break
			}
			socialResults, err := adapter.Search(ctx, query, limit-len(results))
			if err != nil {
				continue
			}
			for _, item := range socialResults {
				classified := t.Safety.Classify(item)
				if classified != SafetySafe {
					continue
				}
				item.Source = adapter.Source()
				item.Safety = classified
				results = append(results, item)
				if len(results) >= limit {
					break
				}
			}
		}
	}

	output := map[string]interface{}{
		"results": results,
	}

	if len(results) == 0 {
		output["note"] = "Tidak ditemukan meme yang cocok dan aman."
	}

	jsonBytes, err := json.Marshal(output)
	if err != nil {
		return "", fmt.Errorf("failed to marshal results: %w", err)
	}

	return string(jsonBytes), nil
}
