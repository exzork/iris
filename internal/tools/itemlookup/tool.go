package itemlookup

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/eko/iris-bot/internal/tools"
)

type Tool struct {
	Lookup *Lookup
}

func New(lookup *Lookup) *Tool {
	return &Tool{Lookup: lookup}
}

func (t *Tool) Schema() *tools.Schema {
	return &tools.Schema{
		Name:        "item_lookup",
		Description: "Look up a Wuthering Waves echo, weapon, or material by name or alias.",
		Fields: []tools.FieldSpec{
			{
				Name:        "name",
				Kind:        tools.KindString,
				Required:    true,
				Description: "item name or alias",
			},
			{
				Name:        "category",
				Kind:        tools.KindString,
				Required:    false,
				Description: "optional: echo|weapon|material",
			},
		},
	}
}

func (t *Tool) Run(ctx context.Context, args map[string]interface{}) (string, error) {
	name, ok := args["name"].(string)
	if !ok {
		return "", fmt.Errorf("name must be a string")
	}

	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("name is required")
	}

	var filterCategory Category = CategoryUnknown
	if categoryVal, ok := args["category"]; ok {
		if categoryStr, ok := categoryVal.(string); ok {
			var c Category
			filterCategory = c.FromString(categoryStr)
		}
	}

	result, err := t.Lookup.Find(ctx, name, filterCategory)
	if err != nil {
		return "", err
	}

	responseItems := make([]map[string]interface{}, len(result.Items))
	for i, item := range result.Items {
		responseItems[i] = map[string]interface{}{
			"name":     item.Name,
			"category": string(item.Category),
			"rarity":   item.Rarity,
			"page_url": item.PageURL,
			"summary":  item.Summary,
		}
	}

	response := map[string]interface{}{
		"status":  string(result.Status),
		"items":   responseItems,
		"message": result.Message,
	}

	jsonBytes, err := json.Marshal(response)
	if err != nil {
		return "", err
	}

	return string(jsonBytes), nil
}
