package itemlookup

import "strings"

// FromString converts a string to a Category.
func (c Category) FromString(s string) Category {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "echo":
		return CategoryEcho
	case "weapon":
		return CategoryWeapon
	case "material":
		return CategoryMaterial
	default:
		return CategoryUnknown
	}
}

// String returns the string representation of a Category.
func (c Category) String() string {
	return string(c)
}

// IsValid checks if the category is a known category.
func (c Category) IsValid() bool {
	switch c {
	case CategoryEcho, CategoryWeapon, CategoryMaterial:
		return true
	default:
		return false
	}
}
