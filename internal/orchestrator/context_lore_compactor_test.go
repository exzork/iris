package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/eko/iris-bot/internal/domain"
)

// MockArchiver is a test double for EpisodeArchiver.
type MockArchiver struct {
	ArchiveCalls int
	ArchiveErr   error
	LastMessages []*domain.ChannelMessage
	LastLines    []string
}

func (m *MockArchiver) Archive(
	ctx context.Context,
	guildID int64,
	messages []*domain.ChannelMessage,
	taggedLines []string,
	resolver ChannelNameResolver,
) error {
	m.ArchiveCalls++
	m.LastMessages = messages
	m.LastLines = taggedLines
	return m.ArchiveErr
}

// MockCompactor is a test double for Compactor (LLM-based).
type MockCompactor struct {
	CompactCalls int
	CompactErr   error
	Result       string
}

func (m *MockCompactor) Compact(ctx context.Context, guildID int64, lines []string) (string, error) {
	m.CompactCalls++
	return m.Result, m.CompactErr
}

func TestCompactForLoreThread_UnderLimit(t *testing.T) {
	lc := &LoreCompactor{
		Limit:           1000,
		RetentionTarget: 0.70,
		Archiver:        &MockArchiver{},
	}

	anchor := &domain.LoreThreadAnchor{
		GuildID:  123,
		ThreadID: 456,
	}

	lines := []string{
		"[ANCHOR] summary line",
		"line 1",
		"line 2",
	}

	result, report, err := lc.CompactForLoreThread(context.Background(), anchor, lines)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Compacted {
		t.Error("expected Compacted=false for under-limit context")
	}
	if report.RetentionRatio != 1.0 {
		t.Errorf("expected RetentionRatio=1.0, got %f", report.RetentionRatio)
	}
	if len(result) != len(lines) {
		t.Errorf("expected result length %d, got %d", len(lines), len(result))
	}
}

func TestCompactForLoreThread_OverLimitWithAnchor(t *testing.T) {
	archiver := &MockArchiver{}
	lc := &LoreCompactor{
		Limit:           500,
		RetentionTarget: 0.70,
		Archiver:        archiver,
	}

	anchor := &domain.LoreThreadAnchor{
		GuildID:  123,
		ThreadID: 456,
	}

	// Create lines that exceed limit: 10 lines of 60 bytes each = 600 bytes total
	lines := []string{
		"[ANCHOR] " + strings.Repeat("a", 51),
		strings.Repeat("b", 60),
		strings.Repeat("c", 60),
		strings.Repeat("d", 60),
		strings.Repeat("e", 60),
		strings.Repeat("f", 60),
		strings.Repeat("g", 60),
		strings.Repeat("h", 60),
		strings.Repeat("i", 60),
		strings.Repeat("j", 60),
	}

	result, report, err := lc.CompactForLoreThread(context.Background(), anchor, lines)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !report.Compacted {
		t.Error("expected Compacted=true for over-limit context")
	}

	// Verify anchor is preserved
	if len(result) == 0 || !strings.HasPrefix(result[0], "[ANCHOR]") {
		t.Error("anchor line not preserved in result")
	}

	// Verify final size is less than original
	if report.FinalSize >= report.OriginalSize {
		t.Errorf("expected final size %d < original size %d", report.FinalSize, report.OriginalSize)
	}

	// Verify final size is less than limit
	if report.FinalSize > lc.Limit {
		t.Errorf("expected final size %d <= limit %d", report.FinalSize, lc.Limit)
	}

	// Verify archiver was called
	if archiver.ArchiveCalls == 0 {
		t.Error("expected archiver to be called")
	}
	if report.ArchivedCount == 0 {
		t.Error("expected ArchivedCount > 0")
	}
}

func TestCompactForLoreThread_MissingAnchor(t *testing.T) {
	archiver := &MockArchiver{}
	lc := &LoreCompactor{
		Limit:           500,
		RetentionTarget: 0.70,
		Archiver:        archiver,
	}

	lines := []string{
		"line 1",
		"line 2",
		"line 3",
	}

	// Call with nil anchor (non-lore thread)
	result, report, err := lc.CompactForLoreThread(context.Background(), nil, lines)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Error("expected nil result for non-lore thread")
	}
	if report.Compacted {
		t.Error("expected Compacted=false for non-lore thread")
	}
	if archiver.ArchiveCalls > 0 {
		t.Error("archiver should not be called for non-lore thread")
	}
}

func TestCompactForLoreThread_ArchiverError(t *testing.T) {
	archiver := &MockArchiver{
		ArchiveErr: errors.New("archive failed"),
	}
	lc := &LoreCompactor{
		Limit:           500,
		RetentionTarget: 0.70,
		Archiver:        archiver,
	}

	anchor := &domain.LoreThreadAnchor{
		GuildID:  123,
		ThreadID: 456,
	}

	lines := []string{
		"[ANCHOR] " + strings.Repeat("a", 100),
		strings.Repeat("b", 100),
		strings.Repeat("c", 100),
		strings.Repeat("d", 100),
		strings.Repeat("e", 100),
		strings.Repeat("f", 100),
	}

	result, report, err := lc.CompactForLoreThread(context.Background(), anchor, lines)

	if err == nil {
		t.Error("expected error from archiver failure")
	}
	if result != nil {
		t.Error("expected nil result when archiver fails (transactional)")
	}
	if report.Compacted {
		t.Error("expected Compacted=false when archiver fails")
	}
}

func TestCompactForLoreThread_RetentionTargetClamping(t *testing.T) {
	tests := []struct {
		name   string
		target float64
	}{
		{"below min", 0.3},
		{"at min", 0.5},
		{"normal", 0.70},
		{"at max", 0.9},
		{"above max", 0.95},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lc := &LoreCompactor{
				Limit:           1000,
				RetentionTarget: tt.target,
				Archiver:        &MockArchiver{},
			}

			anchor := &domain.LoreThreadAnchor{
				GuildID:  123,
				ThreadID: 456,
			}

			// Create lines that exceed limit: 20 lines of 70 bytes each = 1400 bytes total
			lines := []string{
				"[ANCHOR] " + strings.Repeat("a", 61),
			}
			for i := 0; i < 19; i++ {
				lines = append(lines, strings.Repeat(string(rune('b'+i)), 70))
			}

			_, report, err := lc.CompactForLoreThread(context.Background(), anchor, lines)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Just verify compaction happened and result is smaller than original
			if !report.Compacted {
				t.Error("expected Compacted=true")
			}
			if report.FinalSize >= report.OriginalSize {
				t.Errorf("expected final size %d < original %d", report.FinalSize, report.OriginalSize)
			}
			if report.FinalSize > lc.Limit {
				t.Errorf("expected final size %d <= limit %d", report.FinalSize, lc.Limit)
			}
		})
	}
}

func TestContextSize(t *testing.T) {
	lines := []string{"hello", "world", "test"}
	size := contextSize(lines)
	expected := len("hello") + len("world") + len("test")
	if size != expected {
		t.Errorf("expected size %d, got %d", expected, size)
	}
}

func TestFindAnchorLineIndex(t *testing.T) {
	tests := []struct {
		name     string
		lines    []string
		wantIdx  int
	}{
		{
			name:    "anchor at start",
			lines:   []string{"[ANCHOR] summary", "line 1", "line 2"},
			wantIdx: 0,
		},
		{
			name:    "anchor in middle",
			lines:   []string{"line 1", "[ANCHOR] summary", "line 2"},
			wantIdx: 1,
		},
		{
			name:    "anchor at end",
			lines:   []string{"line 1", "line 2", "[ANCHOR] summary"},
			wantIdx: 2,
		},
		{
			name:    "no anchor",
			lines:   []string{"line 1", "line 2", "line 3"},
			wantIdx: -1,
		},
		{
			name:    "empty lines",
			lines:   []string{},
			wantIdx: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx := findAnchorLineIndex(tt.lines)
			if idx != tt.wantIdx {
				t.Errorf("expected index %d, got %d", tt.wantIdx, idx)
			}
		})
	}
}

func TestCompactForLoreThread_AnchorPreservedAsFirstLine(t *testing.T) {
	archiver := &MockArchiver{}
	lc := &LoreCompactor{
		Limit:           300,
		RetentionTarget: 0.70,
		Archiver:        archiver,
	}

	anchor := &domain.LoreThreadAnchor{
		GuildID:  123,
		ThreadID: 456,
	}

	lines := []string{
		"[ANCHOR] summary line with context",
		strings.Repeat("x", 80),
		strings.Repeat("y", 80),
		strings.Repeat("z", 80),
		strings.Repeat("a", 80),
	}

	result, report, err := lc.CompactForLoreThread(context.Background(), anchor, lines)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !report.Compacted {
		t.Error("expected Compacted=true for over-limit context")
	}
	if len(result) < 1 {
		t.Fatal("expected result to have at least 1 line (anchor)")
	}
	if result[0] != lines[0] {
		t.Errorf("expected first line to be anchor, got: %s", result[0])
	}
	if !strings.HasPrefix(result[0], "[ANCHOR]") {
		t.Error("expected anchor line to have [ANCHOR] prefix")
	}
}

func TestCompactForLoreThread_MissingAnchorMarkerHandled(t *testing.T) {
	archiver := &MockArchiver{}
	lc := &LoreCompactor{
		Limit:           300,
		RetentionTarget: 0.70,
		Archiver:        archiver,
	}

	anchor := &domain.LoreThreadAnchor{
		GuildID:  123,
		ThreadID: 456,
	}

	lines := []string{
		"summary line without anchor marker",
		strings.Repeat("x", 80),
		strings.Repeat("y", 80),
		strings.Repeat("z", 80),
		strings.Repeat("a", 80),
	}

	result, report, err := lc.CompactForLoreThread(context.Background(), anchor, lines)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !report.Compacted {
		t.Error("expected Compacted=true for over-limit context")
	}
	if len(result) < 1 {
		t.Error("expected result to have at least 1 line")
	}
}
