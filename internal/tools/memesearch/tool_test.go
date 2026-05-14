package memesearch

import (
	"context"
	"encoding/json"
	"testing"
)

func TestToolRunReturnsDiscordGif(t *testing.T) {
	idx := NewInMemoryDiscordIndex()
	idx.AddMessage(12345, "reaksi kaget", "https://media.tenor.com/example.gif", "image/gif")

	tool := New(idx, []SocialAdapter{}, NewDefaultSafetyClassifier())

	result, err := tool.Run(context.Background(), map[string]interface{}{
		"query":    "reaksi kaget",
		"guild_id": 12345,
	})

	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	var output map[string]interface{}
	if err := json.Unmarshal([]byte(result), &output); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	results, ok := output["results"].([]interface{})
	if !ok {
		t.Fatalf("results is not an array")
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	item := results[0].(map[string]interface{})
	if item["URL"] != "https://media.tenor.com/example.gif" {
		t.Errorf("expected URL https://media.tenor.com/example.gif, got %v", item["URL"])
	}
	if item["Source"] != "discord_history" {
		t.Errorf("expected source discord_history, got %v", item["Source"])
	}
}

func TestToolRunBlocksUnsafeMeme(t *testing.T) {
	idx := NewInMemoryDiscordIndex()

	unsafeResults := []MediaItem{
		{
			URL:      "https://example.com/nsfw.gif",
			Source:   SourceX,
			MimeType: "image/gif",
			Caption:  "this is nsfw content",
			Safety:   SafetyUnknown,
		},
	}

	adapter := NewFakeSocialAdapter(SourceX, unsafeResults)
	tool := New(idx, []SocialAdapter{adapter}, NewDefaultSafetyClassifier())

	result, err := tool.Run(context.Background(), map[string]interface{}{
		"query":    "meme",
		"guild_id": 12345,
	})

	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	var output map[string]interface{}
	if err := json.Unmarshal([]byte(result), &output); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	results, ok := output["results"].([]interface{})
	if !ok {
		t.Fatalf("results is not an array")
	}

	if len(results) != 0 {
		t.Fatalf("expected 0 results (unsafe blocked), got %d", len(results))
	}

	note, ok := output["note"].(string)
	if !ok || note != "Tidak ditemukan meme yang cocok dan aman." {
		t.Errorf("expected Indonesian note, got %v", note)
	}
}

func TestToolRunDiscordFirstThenSocial(t *testing.T) {
	idx := NewInMemoryDiscordIndex()
	idx.AddMessage(12345, "discord meme", "https://media.tenor.com/discord.gif", "image/gif")

	socialResults := []MediaItem{
		{
			URL:      "https://example.com/social.gif",
			Source:   SourceX,
			MimeType: "image/gif",
			Caption:  "social meme",
			Safety:   SafetyUnknown,
		},
	}

	adapter := NewFakeSocialAdapter(SourceX, socialResults)
	tool := New(idx, []SocialAdapter{adapter}, NewDefaultSafetyClassifier())

	result, err := tool.Run(context.Background(), map[string]interface{}{
		"query":    "meme",
		"guild_id": 12345,
	})

	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	var output map[string]interface{}
	if err := json.Unmarshal([]byte(result), &output); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	results, ok := output["results"].([]interface{})
	if !ok {
		t.Fatalf("results is not an array")
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	item := results[0].(map[string]interface{})
	if item["Source"] != "discord_history" {
		t.Errorf("expected Discord result first, got %v", item["Source"])
	}
}

func TestToolRunAllBlockedEmptyNote(t *testing.T) {
	idx := NewInMemoryDiscordIndex()

	unsafeResults := []MediaItem{
		{
			URL:      "https://example.com/unsafe1.gif",
			Source:   SourceX,
			MimeType: "image/gif",
			Caption:  "xxx content",
			Safety:   SafetyUnknown,
		},
	}

	adapter := NewFakeSocialAdapter(SourceX, unsafeResults)
	tool := New(idx, []SocialAdapter{adapter}, NewDefaultSafetyClassifier())

	result, err := tool.Run(context.Background(), map[string]interface{}{
		"query":    "meme",
		"guild_id": 12345,
	})

	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	var output map[string]interface{}
	if err := json.Unmarshal([]byte(result), &output); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	results, ok := output["results"].([]interface{})
	if !ok {
		t.Fatalf("results is not an array")
	}

	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}

	note, ok := output["note"].(string)
	if !ok || note != "Tidak ditemukan meme yang cocok dan aman." {
		t.Errorf("expected Indonesian note, got %v", note)
	}
}

func TestToolRunMissingQueryError(t *testing.T) {
	idx := NewInMemoryDiscordIndex()
	tool := New(idx, []SocialAdapter{}, NewDefaultSafetyClassifier())

	_, err := tool.Run(context.Background(), map[string]interface{}{
		"guild_id": 12345,
	})

	if err == nil {
		t.Fatalf("expected error for missing query")
	}
}

func TestToolSchemaContract(t *testing.T) {
	idx := NewInMemoryDiscordIndex()
	tool := New(idx, []SocialAdapter{}, NewDefaultSafetyClassifier())

	schema := tool.Schema()

	if schema.Name != "meme_search" {
		t.Errorf("expected schema name meme_search, got %s", schema.Name)
	}

	if len(schema.Fields) != 3 {
		t.Errorf("expected 3 fields, got %d", len(schema.Fields))
	}

	queryField := schema.Fields[0]
	if queryField.Name != "query" || !queryField.Required {
		t.Errorf("query field invalid")
	}

	guildIDField := schema.Fields[1]
	if guildIDField.Name != "guild_id" || !guildIDField.Required {
		t.Errorf("guild_id field invalid")
	}

	limitField := schema.Fields[2]
	if limitField.Name != "limit" || limitField.Required {
		t.Errorf("limit field invalid")
	}
}
