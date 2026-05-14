package charlookup

import (
	"context"
	"testing"
)

func TestInMemoryStoreAddGet(t *testing.T) {
	store := NewInMemoryStore()
	char := &Character{
		ID:      1,
		Name:    "Rover",
		Element: "Spectro",
		Weapon:  "Sword",
	}

	err := store.Add(char)
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	retrieved, err := store.GetByID(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if retrieved.ID != char.ID || retrieved.Name != char.Name {
		t.Errorf("Retrieved character mismatch: got %+v, want %+v", retrieved, char)
	}
}

func TestInMemoryStoreList(t *testing.T) {
	store := NewInMemoryStore()
	chars := []*Character{
		{ID: 1, Name: "Rover"},
		{ID: 2, Name: "Jinshi"},
		{ID: 3, Name: "Calcharo"},
	}

	for _, char := range chars {
		if err := store.Add(char); err != nil {
			t.Fatalf("Add failed: %v", err)
		}
	}

	list, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(list) != len(chars) {
		t.Errorf("List length: got %d, want %d", len(list), len(chars))
	}
}

func TestInMemoryStoreGetNotFound(t *testing.T) {
	store := NewInMemoryStore()

	_, err := store.GetByID(context.Background(), 999)
	if err == nil {
		t.Fatal("GetByID should return error for missing character")
	}
}

func TestInMemoryStoreAddNil(t *testing.T) {
	store := NewInMemoryStore()

	err := store.Add(nil)
	if err == nil {
		t.Fatal("Add(nil) should return error")
	}
}

func TestInMemoryStoreAddZeroID(t *testing.T) {
	store := NewInMemoryStore()
	char := &Character{
		ID:   0,
		Name: "Invalid",
	}

	err := store.Add(char)
	if err == nil {
		t.Fatal("Add with zero ID should return error")
	}
}
