package itemlookup

import (
	"context"
	"testing"
)

func TestInMemoryStoreFindByAliasMulti(t *testing.T) {
	store := NewInMemoryStore()

	item1 := &Item{
		ID:       1,
		Name:     "Sword A",
		Aliases:  []string{"Sword"},
		Category: CategoryWeapon,
		Rarity:   "5-star",
		PageURL:  "http://example.com/sword-a",
		Summary:  "A sword",
	}

	item2 := &Item{
		ID:       2,
		Name:     "Sword B",
		Aliases:  []string{"Sword"},
		Category: CategoryWeapon,
		Rarity:   "4-star",
		PageURL:  "http://example.com/sword-b",
		Summary:  "Another sword",
	}

	store.Add(item1)
	store.Add(item2)

	results, err := store.FindByAlias(context.Background(), "Sword")
	if err != nil {
		t.Fatalf("FindByAlias failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("FindByAlias(Sword) returned %d items, want 2", len(results))
	}
}

func TestInMemoryStoreFindByAliasCaseInsensitive(t *testing.T) {
	store := NewInMemoryStore()

	item := &Item{
		ID:       1,
		Name:     "Emerald of Genesis",
		Aliases:  []string{"Emerald"},
		Category: CategoryWeapon,
		Rarity:   "5-star",
		PageURL:  "http://example.com/emerald",
		Summary:  "A weapon",
	}

	store.Add(item)

	tests := []string{"emerald of genesis", "EMERALD OF GENESIS", "Emerald Of Genesis", "  emerald of genesis  "}

	for _, query := range tests {
		results, err := store.FindByAlias(context.Background(), query)
		if err != nil {
			t.Fatalf("FindByAlias(%q) failed: %v", query, err)
		}
		if len(results) != 1 {
			t.Errorf("FindByAlias(%q) returned %d items, want 1", query, len(results))
		}
	}
}

func TestInMemoryStoreGetByID(t *testing.T) {
	store := NewInMemoryStore()

	item := &Item{
		ID:       42,
		Name:     "Test Item",
		Aliases:  []string{},
		Category: CategoryEcho,
		Rarity:   "3-star",
		PageURL:  "http://example.com/test",
		Summary:  "Test",
	}

	store.Add(item)

	retrieved, err := store.GetByID(context.Background(), 42)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("GetByID returned nil")
	}

	if retrieved.ID != 42 || retrieved.Name != "Test Item" {
		t.Errorf("GetByID returned wrong item: %+v", retrieved)
	}

	notFound, err := store.GetByID(context.Background(), 999)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if notFound != nil {
		t.Errorf("GetByID(999) should return nil, got %+v", notFound)
	}
}
