package charlookup

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/eko/iris-bot/internal/tools"
)

type Tool struct {
	L *Lookup
}

func (t *Tool) Schema() *tools.Schema {
	return &tools.Schema{
		Name:        "character_lookup",
		Description: "Look up a Wuthering Waves character by name or alias; returns Indonesian summary with wiki citation.",
		Fields: []tools.FieldSpec{
			{
				Name:        "name",
				Kind:        tools.KindString,
				Required:    true,
				Description: "character name or alias",
			},
		},
	}
}

func (t *Tool) Run(ctx context.Context, args map[string]interface{}) (string, error) {
	if t.L == nil {
		return "", fmt.Errorf("Lookup not initialized")
	}

	name, ok := args["name"].(string)
	if !ok {
		return "", fmt.Errorf("name argument must be a string")
	}

	result, err := t.L.Find(ctx, name)
	if err != nil {
		return "", err
	}

	if !result.Found {
		response := map[string]interface{}{
			"found":   false,
			"message": result.Missing,
		}
		data, _ := json.Marshal(response)
		return string(data), nil
	}

	citations := make([]map[string]string, len(result.Citations))
	for i, c := range result.Citations {
		citations[i] = map[string]string{
			"title": c.Title,
			"url":   c.URL,
		}
	}

	response := map[string]interface{}{
		"found":     true,
		"name":      result.Character.Name,
		"element":   result.Character.Element,
		"weapon":    result.Character.Weapon,
		"rarity":    result.Character.Rarity,
		"summary":   result.Summary,
		"citations": citations,
	}

	data, err := json.Marshal(response)
	if err != nil {
		return "", err
	}

	return string(data), nil
}
