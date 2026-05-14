package settings

import (
	"testing"
)

func TestParseBool(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"true", true},
		{"True", true},
		{"TRUE", true},
		{"yes", true},
		{"YES", true},
		{"1", true},
		{"false", false},
		{"False", false},
		{"FALSE", false},
		{"no", false},
		{"NO", false},
		{"0", false},
		{"", false},
		{"  true  ", true},
		{"  false  ", false},
		{"invalid", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseBool(tt.input)
			if got != tt.expected {
				t.Errorf("parseBool(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestParseInt64Slice(t *testing.T) {
	tests := []struct {
		input    string
		expected []int64
		wantErr  bool
	}{
		{"1,2,3", []int64{1, 2, 3}, false},
		{"1", []int64{1}, false},
		{"", []int64{}, false},
		{"  1  ,  2  ,  3  ", []int64{1, 2, 3}, false},
		{"123456789012345", []int64{123456789012345}, false},
		{"abc", nil, true},
		{"1,abc,3", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseInt64Slice(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseInt64Slice(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}
			if len(got) != len(tt.expected) {
				t.Errorf("parseInt64Slice(%q) len = %d, want %d", tt.input, len(got), len(tt.expected))
				return
			}
			for i, v := range got {
				if v != tt.expected[i] {
					t.Errorf("parseInt64Slice(%q)[%d] = %d, want %d", tt.input, i, v, tt.expected[i])
				}
			}
		})
	}
}
