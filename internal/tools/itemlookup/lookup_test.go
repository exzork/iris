package itemlookup

import (
	"context"
	"strings"
	"testing"
)

func TestLookupFindExactWeapon(t *testing.T) {
	store := NewInMemoryStore()

	weapon := &Item{
		ID:       1,
		Name:     "Emerald of Genesis",
		Aliases:  []string{"Emerald"},
		Category: CategoryWeapon,
		Rarity:   "5-star",
		PageURL:  "https://wuthering-waves.fandom.com/wiki/Emerald_of_Genesis",
		Summary:  "A powerful weapon",
	}

	store.Add(weapon)

	lookup := NewLookup(store)
	result, err := lookup.Find(context.Background(), "Emerald of Genesis", CategoryUnknown)

	if err != nil {
		t.Fatalf("Find failed: %v", err)
	}

	if result.Status != StatusFound {
		t.Errorf("Status = %q, want %q", result.Status, StatusFound)
	}

	if len(result.Items) != 1 {
		t.Errorf("Items length = %d, want 1", len(result.Items))
	}

	if result.Items[0].Category != CategoryWeapon {
		t.Errorf("Category = %q, want %q", result.Items[0].Category, CategoryWeapon)
	}

	if !strings.Contains(result.Message, "https://wuthering-waves.fandom.com/wiki/Emerald_of_Genesis") {
		t.Errorf("Message missing citation URL: %s", result.Message)
	}
}

func TestLookupAmbiguousReturnsAllMatches(t *testing.T) {
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
	result, err := lookup.Find(context.Background(), "Sword", CategoryUnknown)

	if err != nil {
		t.Fatalf("Find failed: %v", err)
	}

	if result.Status != StatusAmbiguous {
		t.Errorf("Status = %q, want %q", result.Status, StatusAmbiguous)
	}

	if len(result.Items) != 2 {
		t.Errorf("Items length = %d, want 2", len(result.Items))
	}

	if !strings.Contains(result.Message, "Sword A") || !strings.Contains(result.Message, "Sword B") {
		t.Errorf("Message missing candidates: %s", result.Message)
	}
}

func TestLookupMissingMessage(t *testing.T) {
	store := NewInMemoryStore()
	lookup := NewLookup(store)

	result, err := lookup.Find(context.Background(), "NonExistent", CategoryUnknown)

	if err != nil {
		t.Fatalf("Find failed: %v", err)
	}

	if result.Status != StatusMissing {
		t.Errorf("Status = %q, want %q", result.Status, StatusMissing)
	}

	if len(result.Items) != 0 {
		t.Errorf("Items length = %d, want 0", len(result.Items))
	}

	if !strings.Contains(result.Message, "tidak ditemukan") {
		t.Errorf("Message missing Indonesian text: %s", result.Message)
	}
}

func TestLookupCategoryFilterNarrows(t *testing.T) {
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

	result, err := lookup.Find(context.Background(), "X", CategoryWeapon)
	if err != nil {
		t.Fatalf("Find failed: %v", err)
	}

	if result.Status != StatusFound {
		t.Errorf("Status = %q, want %q", result.Status, StatusFound)
	}

	if len(result.Items) != 1 {
		t.Errorf("Items length = %d, want 1", len(result.Items))
	}

	if result.Items[0].Category != CategoryWeapon {
		t.Errorf("Category = %q, want %q", result.Items[0].Category, CategoryWeapon)
	}
}

func TestLookupEmptyQuery(t *testing.T) {
	store := NewInMemoryStore()
	lookup := NewLookup(store)

	result, err := lookup.Find(context.Background(), "   ", CategoryUnknown)

	if err != nil {
		t.Fatalf("Find failed: %v", err)
	}

	if result.Status != StatusMissing {
		t.Errorf("Status = %q, want %q", result.Status, StatusMissing)
	}
}
