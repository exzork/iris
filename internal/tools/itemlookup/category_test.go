package itemlookup

import "testing"

func TestCategoryFromString(t *testing.T) {
	tests := []struct {
		input    string
		expected Category
	}{
		{"echo", CategoryEcho},
		{"ECHO", CategoryEcho},
		{"  echo  ", CategoryEcho},
		{"weapon", CategoryWeapon},
		{"WEAPON", CategoryWeapon},
		{"material", CategoryMaterial},
		{"MATERIAL", CategoryMaterial},
		{"unknown", CategoryUnknown},
		{"invalid", CategoryUnknown},
		{"", CategoryUnknown},
	}

	var c Category
	for _, tt := range tests {
		result := c.FromString(tt.input)
		if result != tt.expected {
			t.Errorf("FromString(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestCategoryFromStringUnknown(t *testing.T) {
	var c Category
	result := c.FromString("notacategory")
	if result != CategoryUnknown {
		t.Errorf("FromString(notacategory) = %q, want %q", result, CategoryUnknown)
	}
}

func TestCategoryIsValid(t *testing.T) {
	tests := []struct {
		cat      Category
		expected bool
	}{
		{CategoryEcho, true},
		{CategoryWeapon, true},
		{CategoryMaterial, true},
		{CategoryUnknown, false},
		{Category("invalid"), false},
	}

	for _, tt := range tests {
		result := tt.cat.IsValid()
		if result != tt.expected {
			t.Errorf("IsValid(%q) = %v, want %v", tt.cat, result, tt.expected)
		}
	}
}
