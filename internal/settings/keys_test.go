package settings

import (
	"testing"
)

func TestDefaultValueKnown(t *testing.T) {
	tests := []struct {
		key      Key
		expected string
	}{
		{KeyDefaultLocale, "id-ID"},
		{KeyMemoryEnabled, "true"},
		{KeyLoreCitationsRequired, "true"},
		{KeyMaxResponseChars, "2000"},
		{KeyImageCooldownSec, "60"},
		{KeyMemesEnabled, "true"},
		{KeyRemindersEnabled, "true"},
		{KeyRateLimitUserPerMin, "30"},
		{KeyRateLimitGuildPerMin, "120"},
	}
	for _, tt := range tests {
		t.Run(string(tt.key), func(t *testing.T) {
			val, ok := DefaultValue(tt.key)
			if !ok {
				t.Fatalf("DefaultValue(%q) ok=false, want true", tt.key)
			}
			if val != tt.expected {
				t.Errorf("DefaultValue(%q) = %q, want %q", tt.key, val, tt.expected)
			}
		})
	}
}

func TestDefaultValueUnknown(t *testing.T) {
	val, ok := DefaultValue(Key("unknown_key"))
	if ok {
		t.Errorf("DefaultValue(unknown_key) ok=true, want false")
	}
	if val != "" {
		t.Errorf("DefaultValue(unknown_key) = %q, want empty", val)
	}
}

func TestIsKnown(t *testing.T) {
	tests := []struct {
		key      Key
		expected bool
	}{
		{KeyAdminRoleIDs, true},
		{KeyDefaultLocale, true},
		{KeyMemoryEnabled, true},
		{KeyReminderChannelID, true},
		{Key("unknown"), false},
		{Key(""), false},
	}
	for _, tt := range tests {
		t.Run(string(tt.key), func(t *testing.T) {
			got := IsKnown(tt.key)
			if got != tt.expected {
				t.Errorf("IsKnown(%q) = %v, want %v", tt.key, got, tt.expected)
			}
		})
	}
}

func TestParseKeyCaseInsensitive(t *testing.T) {
	tests := []struct {
		input    string
		expected Key
		ok       bool
	}{
		{"admin_role_ids", KeyAdminRoleIDs, true},
		{"ADMIN_ROLE_IDS", KeyAdminRoleIDs, true},
		{"Admin_Role_IDs", KeyAdminRoleIDs, true},
		{"  default_locale  ", KeyDefaultLocale, true},
		{"memory_enabled", KeyMemoryEnabled, true},
		{"unknown_key", Key("unknown_key"), false},
		{"", Key(""), false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, ok := ParseKey(tt.input)
			if ok != tt.ok {
				t.Errorf("ParseKey(%q) ok=%v, want %v", tt.input, ok, tt.ok)
			}
			if got != tt.expected {
				t.Errorf("ParseKey(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
