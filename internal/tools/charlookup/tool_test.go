package charlookup

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/eko/iris-bot/internal/tools"
)

func TestToolSchemaContract(t *testing.T) {
	tool := &Tool{L: &Lookup{}}
	schema := tool.Schema()

	if schema.Name != "character_lookup" {
		t.Errorf("Schema name: got %q, want character_lookup", schema.Name)
	}
	if schema.Description == "" {
		t.Fatal("Schema description is empty")
	}
	if len(schema.Fields) != 1 {
		t.Errorf("Schema fields count: got %d, want 1", len(schema.Fields))
	}

	field := schema.Fields[0]
	if field.Name != "name" {
		t.Errorf("Field name: got %q, want name", field.Name)
	}
	if field.Kind != tools.KindString {
		t.Errorf("Field kind: got %q, want string", field.Kind)
	}
	if !field.Required {
		t.Fatal("Field should be required")
	}

	if err := schema.Validate(); err != nil {
		t.Fatalf("Schema validation failed: %v", err)
	}
}

func TestToolRunCharacterFound(t *testing.T) {
	store := NewInMemoryStore()
	char := &Character{
		ID:      1,
		Name:    "Rover",
		Aliases: []string{"protagonist"},
		Element: "Spectro",
		Weapon:  "Sword",
		Rarity:  "5-Star",
		PageURL: "https://wutheringwaves.fandom.com/wiki/Rover",
		Summary: "Rover adalah karakter utama dalam Wuthering Waves, seorang pejuang misterius dengan kekuatan Spectro.",
	}
	store.Add(char)

	idx := NewAliasIndex()
	idx.Add(char)

	lookup := &Lookup{
		Store: store,
		Alias: idx,
	}

	tool := &Tool{L: lookup}

	result, err := tool.Run(context.Background(), map[string]interface{}{"name": "protagonist"})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	var response map[string]interface{}
	if err := json.Unmarshal([]byte(result), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if found, ok := response["found"].(bool); !ok || !found {
		t.Fatal("Expected found=true")
	}

	if name, ok := response["name"].(string); !ok || name != "Rover" {
		t.Errorf("Name: got %q, want Rover", name)
	}

	if element, ok := response["element"].(string); !ok || element != "Spectro" {
		t.Errorf("Element: got %q, want Spectro", element)
	}

	if weapon, ok := response["weapon"].(string); !ok || weapon != "Sword" {
		t.Errorf("Weapon: got %q, want Sword", weapon)
	}

	if summary, ok := response["summary"].(string); !ok || summary == "" {
		t.Fatal("Summary is empty or not a string")
	}

	t.Logf("Character found response:\n%s", result)
}

func TestToolRunCharacterMissing(t *testing.T) {
	store := NewInMemoryStore()
	idx := NewAliasIndex()

	lookup := &Lookup{
		Store: store,
		Alias: idx,
	}

	tool := &Tool{L: lookup}

	result, err := tool.Run(context.Background(), map[string]interface{}{"name": "TidakAda999"})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	var response map[string]interface{}
	if err := json.Unmarshal([]byte(result), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if found, ok := response["found"].(bool); !ok || found {
		t.Fatal("Expected found=false")
	}

	if message, ok := response["message"].(string); !ok || message == "" {
		t.Fatal("Message is empty or not a string")
	}

	message := response["message"].(string)
	if !findSubstring(message, "belum terindeks") {
		t.Errorf("Message should contain 'belum terindeks': %q", message)
	}
	if !findSubstring(message, "TidakAda999") {
		t.Errorf("Message should contain query: %q", message)
	}

	t.Logf("Character missing response:\n%s", result)
}

func TestToolRunMissingArgError(t *testing.T) {
	tool := &Tool{L: &Lookup{}}

	_, err := tool.Run(context.Background(), map[string]interface{}{})
	if err == nil {
		t.Fatal("Expected error for missing name argument")
	}
}

func TestToolRunInvalidArgType(t *testing.T) {
	tool := &Tool{L: &Lookup{}}

	_, err := tool.Run(context.Background(), map[string]interface{}{"name": 123})
	if err == nil {
		t.Fatal("Expected error for invalid name type")
	}
}

func TestToolRunNilLookup(t *testing.T) {
	tool := &Tool{L: nil}

	_, err := tool.Run(context.Background(), map[string]interface{}{"name": "test"})
	if err == nil {
		t.Fatal("Expected error for nil Lookup")
	}
}
