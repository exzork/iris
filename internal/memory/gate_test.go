package memory

import (
	"testing"
)

func TestGateDecide(t *testing.T) {
	gate := NewGate()

	tests := []struct {
		name         string
		text         string
		expectSave   bool
		expectReason string
	}{
		{
			name:         "preference accept Indonesian",
			text:         "aku suka jawaban singkat",
			expectSave:   true,
			expectReason: "preference",
		},
		{
			name:         "preference accept English",
			text:         "i prefer concise answers",
			expectSave:   true,
			expectReason: "preference",
		},
		{
			name:         "remember command accept",
			text:         "tolong ingat ini: aku suka lore detail",
			expectSave:   true,
			expectReason: "preference",
		},
		{
			name:         "self-disclosure accept",
			text:         "nama saya Budi dan timezone saya Asia/Jakarta",
			expectSave:   true,
			expectReason: "self_disclosure",
		},
		{
			name:         "question reject",
			text:         "apa itu Rover?",
			expectSave:   false,
			expectReason: "question",
		},
		{
			name:         "command reject slash",
			text:         "/help",
			expectSave:   false,
			expectReason: "command",
		},
		{
			name:         "command reject exclamation",
			text:         "!iris exception add 1",
			expectSave:   false,
			expectReason: "command",
		},
		{
			name:         "chatter reject haha",
			text:         "haha",
			expectSave:   false,
			expectReason: "too_short",
		},
		{
			name:         "chatter reject ok",
			text:         "ok",
			expectSave:   false,
			expectReason: "too_short",
		},
		{
			name:         "lore-only reject",
			text:         "Rover adalah protagonis Wuthering Waves",
			expectSave:   false,
			expectReason: "lore_only",
		},
		{
			name:         "empty reject",
			text:         "",
			expectSave:   false,
			expectReason: "empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := gate.Decide(tt.text)
			if decision.Save != tt.expectSave {
				t.Errorf("Save: got %v, want %v", decision.Save, tt.expectSave)
			}
			if decision.Reason != tt.expectReason {
				t.Errorf("Reason: got %q, want %q", decision.Reason, tt.expectReason)
			}
		})
	}
}
