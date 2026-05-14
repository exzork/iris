package itemlookup

import (
	"context"
	"fmt"
	"strings"
)

type ResultStatus string

const (
	StatusFound     ResultStatus = "found"
	StatusAmbiguous ResultStatus = "ambiguous"
	StatusMissing   ResultStatus = "missing"
)

type Result struct {
	Status  ResultStatus
	Items   []*Item
	Message string
}

type Lookup struct {
	Store ItemStore
}

func NewLookup(store ItemStore) *Lookup {
	return &Lookup{Store: store}
}

func (l *Lookup) Find(ctx context.Context, query string, filterCategory Category) (*Result, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return &Result{
			Status:  StatusMissing,
			Items:   []*Item{},
			Message: "Silakan berikan nama item yang ingin dicari.",
		}, nil
	}

	items, err := l.Store.FindByAlias(ctx, query)
	if err != nil {
		return nil, err
	}

	if filterCategory != CategoryUnknown && filterCategory.IsValid() {
		filtered := make([]*Item, 0)
		for _, item := range items {
			if item.Category == filterCategory {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}

	if len(items) == 0 {
		return &Result{
			Status:  StatusMissing,
			Items:   []*Item{},
			Message: fmt.Sprintf("Item %q tidak ditemukan di indeks.", query),
		}, nil
	}

	if len(items) == 1 {
		item := items[0]
		message := fmt.Sprintf("**%s** (%s, %s)\n%s\n[Sumber](%s)", item.Name, item.Category, item.Rarity, item.Summary, item.PageURL)
		return &Result{
			Status:  StatusFound,
			Items:   items,
			Message: message,
		}, nil
	}

	var candidates []string
	for _, item := range items {
		candidates = append(candidates, fmt.Sprintf("• %s (%s)", item.Name, item.Category))
	}

	message := fmt.Sprintf("Item %q ditemukan di beberapa kategori. Pilih salah satu:\n%s", query, strings.Join(candidates, "\n"))
	return &Result{
		Status:  StatusAmbiguous,
		Items:   items,
		Message: message,
	}, nil
}
