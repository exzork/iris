package itemlookup

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestToolRunWeaponFound(t *testing.T) {
	store := NewInMemoryStore()

	weapon := &Item{
		ID:       1,
		Name:     "Emerald of Genesis",
		Aliases:  []string{"Emerald"},
		Category: CategoryWeapon,
		Rarity:   "5-star",
		PageURL:  "https://wuthering-waves.fandom.com/wiki/Emerald_of_Genesis",
		Summary:  "A powerful weapon for Jianxian",
	}

	store.Add(weapon)

	lookup := NewLookup(store)
	tool := New(lookup)

	result, err := tool.Run(context.Background(), map[string]interface{}{
		"name": "Emerald of Genesis",
	})

	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	var response map[string]interface{}
	err = json.Unmarshal([]byte(result), &response)
	if err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response["status"] != "found" {
		t.Errorf("status = %q, want found", response["status"])
	}

	items, ok := response["items"].([]interface{})
	if !ok || len(items) != 1 {
		t.Errorf("items length = %d, want 1", len(items))
	}

	itemMap := items[0].(map[string]interface{})
	if itemMap["category"] != "weapon" {
		t.Errorf("category = %q, want weapon", itemMap["category"])
	}

	if !strings.Contains(itemMap["page_url"].(string), "Emerald_of_Genesis") {
		t.Errorf("page_url missing expected content: %s", itemMap["page_url"])
	}

	t.Logf("TestToolRunWeaponFound result:\n%s", result)
}

func TestToolRunAmbiguous(t *testing.T) {
	store := NewInMemoryStore()

	sword1 := &Item{
		ID:       1,
		Name:     "Sword A",
		Aliases:  []string{"Sword"},
		Category: CategoryWeapon,
		Rarity:   "5-star",
		PageURL:  "http://example.com/sword-a",
		Summary:  "First sword",
	}

	sword2 := &Item{
		ID:       2,
		Name:     "Sword B",
		Aliases:  []string{"Sword"},
		Category: CategoryWeapon,
		Rarity:   "4-star",
		PageURL:  "http://example.com/sword-b",
		Summary:  "Second sword",
	}

	store.Add(sword1)
	store.Add(sword2)

	lookup := NewLookup(store)
	tool := New(lookup)

	result, err := tool.Run(context.Background(), map[string]interface{}{
		"name": "Sword",
	})

	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	var response map[string]interface{}
	err = json.Unmarshal([]byte(result), &response)
	if err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response["status"] != "ambiguous" {
		t.Errorf("status = %q, want ambiguous", response["status"])
	}

	items, ok := response["items"].([]interface{})
	if !ok || len(items) != 2 {
		t.Errorf("items length = %d, want 2", len(items))
	}

	message := response["message"].(string)
	if !strings.Contains(message, "Sword A") || !strings.Contains(message, "Sword B") {
		t.Errorf("message missing candidates: %s", message)
	}

	if !strings.Contains(message, "ditemukan di beberapa kategori") {
		t.Errorf("message missing Indonesian text: %s", message)
	}

	t.Logf("TestToolRunAmbiguous result:\n%s", result)
}

func TestToolRunMissing(t *testing.T) {
	store := NewInMemoryStore()
	lookup := NewLookup(store)
	tool := New(lookup)

	result, err := tool.Run(context.Background(), map[string]interface{}{
		"name": "NonExistent",
	})

	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	var response map[string]interface{}
	err = json.Unmarshal([]byte(result), &response)
	if err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response["status"] != "missing" {
		t.Errorf("status = %q, want missing", response["status"])
	}

	items, ok := response["items"].([]interface{})
	if !ok || len(items) != 0 {
		t.Errorf("items length = %d, want 0", len(items))
	}
}

func TestToolSchemaContract(t *testing.T) {
	store := NewInMemoryStore()
	lookup := NewLookup(store)
	tool := New(lookup)

	schema := tool.Schema()

	if schema.Name != "item_lookup" {
		t.Errorf("schema name = %q, want item_lookup", schema.Name)
	}

	if len(schema.Fields) != 2 {
		t.Errorf("schema fields length = %d, want 2", len(schema.Fields))
	}

	nameField := schema.Fields[0]
	if nameField.Name != "name" || !nameField.Required {
		t.Errorf("name field invalid: %+v", nameField)
	}

	categoryField := schema.Fields[1]
	if categoryField.Name != "category" || categoryField.Required {
		t.Errorf("category field invalid: %+v", categoryField)
	}

	err := schema.Validate()
	if err != nil {
		t.Errorf("schema validation failed: %v", err)
	}
}

func TestToolRunCategoryFilter(t *testing.T) {
	store := NewInMemoryStore()

	weapon := &Item{
		ID:       1,
		Name:     "Item X",
		Aliases:  []string{"X"},
		Category: CategoryWeapon,
		Rarity:   "5-star",
		PageURL:  "http://example.com/x",
		Summary:  "A weapon",
	}

	echo := &Item{
		ID:       2,
		Name:     "Item X Echo",
		Aliases:  []string{"X"},
		Category: CategoryEcho,
		Rarity:   "3-star",
		PageURL:  "http://example.com/x-echo",
		Summary:  "An echo",
	}

	store.Add(weapon)
	store.Add(echo)

	lookup := NewLookup(store)
	tool := New(lookup)

	result, err := tool.Run(context.Background(), map[string]interface{}{
		"name":     "X",
		"category": "weapon",
	})

	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	var response map[string]interface{}
	err = json.Unmarshal([]byte(result), &response)
	if err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response["status"] != "found" {
		t.Errorf("status = %q, want found", response["status"])
	}

	items, ok := response["items"].([]interface{})
	if !ok || len(items) != 1 {
		t.Errorf("items length = %d, want 1", len(items))
	}

	itemMap := items[0].(map[string]interface{})
	if itemMap["category"] != "weapon" {
		t.Errorf("category = %q, want weapon", itemMap["category"])
	}
}
