package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/eko/iris-bot/internal/domain"
	"github.com/eko/iris-bot/internal/lorethread"
)

// ContextLine represents a single line of context in the tagged format.
type ContextLine struct {
	Raw string // The raw tagged line: <channel>|<thread>|<userid>|<timestamp>|<message>
}

// CompactionReport contains metrics about a compaction operation.
type CompactionReport struct {
	Compacted      bool
	OriginalSize   int
	FinalSize      int
	RetentionRatio float64
	ArchivedCount  int
}

// LoreCompactor handles context compaction for lore threads with 70% retention target.
type LoreCompactor struct {
	Limit           int                      // Max context size (chars or tokens, same unit as existing compactor)
	RetentionTarget float64                  // Target retention ratio (default 0.70), clamped to [0.5, 0.9]
	Archiver        EpisodeArchiver          // Archives compacted-out content
	LLMCompactor    Compactor                // Optional: summarizes compacted-out portion
	MetricsHooks    *lorethread.MetricsHooks // Optional: emits metrics on compaction
}

// NewLoreCompactor creates a new lore compactor with sensible defaults.
func NewLoreCompactor(limit int, archiver EpisodeArchiver) *LoreCompactor {
	return &LoreCompactor{
		Limit:           limit,
		RetentionTarget: 0.70,
		Archiver:        archiver,
		LLMCompactor:    nil,
	}
}

// CompactForLoreThread compacts context for a lore thread, preserving the anchor
// and recent messages while archiving the middle section.
//
// Behavior:
// - If total size <= Limit, returns unchanged with Compacted=false
// - If anchor is present, PRESERVES it (never drops)
// - Keeps recent N lines until total size ≈ Limit * RetentionTarget ± 5%
// - Middle section (between anchor and recent tail) is archived via Archiver and removed from returned context
// - Returns error if archiver fails (transactional: context not modified)
//
// If anchor is nil (non-lore thread), returns nil without modifying context.
func (lc *LoreCompactor) CompactForLoreThread(
	ctx context.Context,
	anchor *domain.LoreThreadAnchor,
	lines []string,
) ([]string, CompactionReport, error) {
	report := CompactionReport{}

	// Non-lore thread: opt-out path
	if anchor == nil {
		return nil, report, nil
	}

	// Clamp retention target to [0.5, 0.9]
	target := lc.RetentionTarget
	if target < 0.5 {
		target = 0.5
	} else if target > 0.9 {
		target = 0.9
	}

	// Calculate original size
	originalSize := contextSize(lines)
	report.OriginalSize = originalSize

	// If under limit, no compaction needed
	if originalSize <= lc.Limit {
		report.Compacted = false
		report.FinalSize = originalSize
		report.RetentionRatio = 1.0
		return lines, report, nil
	}

	// Find anchor line index
	anchorIdx := findAnchorLineIndex(lines)

	// Calculate target size for retention
	targetSize := int(float64(lc.Limit) * target)
	tolerance := int(float64(lc.Limit) * 0.05)
	minSize := targetSize - tolerance
	maxSize := targetSize + tolerance

	// Strategy: keep anchor + recent lines that fit within target size
	// Start from the end and work backwards, collecting lines until we reach target size
	var recentLines []string
	currentSize := 0

	for i := len(lines) - 1; i >= 0; i-- {
		// Skip anchor line (will add separately)
		if i == anchorIdx {
			continue
		}

		lineSize := len(lines[i])
		newSize := currentSize + lineSize

		// Add line if it keeps us within max size
		if newSize <= maxSize {
			recentLines = append([]string{lines[i]}, recentLines...)
			currentSize = newSize
		} else if currentSize < minSize {
			// Still below minimum, add anyway to reach minimum
			recentLines = append([]string{lines[i]}, recentLines...)
			currentSize = newSize
		}
		// else: we're at or above minSize and adding this line would exceed maxSize, so stop
	}

	// Build final result: anchor + recent lines
	var result []string
	if anchorIdx >= 0 && anchorIdx < len(lines) {
		result = append(result, lines[anchorIdx])
	}
	result = append(result, recentLines...)

	// Determine which lines were archived (everything not in result)
	var archivedLines []string
	for i := 0; i < len(lines); i++ {
		if i == anchorIdx {
			continue
		}
		found := false
		for _, r := range result {
			if r == lines[i] {
				found = true
				break
			}
		}
		if !found {
			archivedLines = append(archivedLines, lines[i])
		}
	}

	// Archive the middle section if we have lines to archive
	if len(archivedLines) > 0 && lc.Archiver != nil {
		err := lc.Archiver.Archive(ctx, anchor.GuildID, nil, archivedLines, nil)
		if err != nil {
			// Transactional: return error without modifying context
			return nil, report, fmt.Errorf("archiver failed: %w", err)
		}
		report.ArchivedCount = len(archivedLines)
	}

	// Calculate final metrics
	finalSize := contextSize(result)
	report.Compacted = true
	report.FinalSize = finalSize
	report.RetentionRatio = float64(finalSize) / float64(originalSize)

	if lc.MetricsHooks != nil {
		lc.MetricsHooks.OnCompaction()
	}

	return result, report, nil
}

// contextSize calculates the total size of context lines in bytes.
func contextSize(lines []string) int {
	total := 0
	for _, line := range lines {
		total += len(line)
	}
	return total
}

// findAnchorLineIndex finds the index of the anchor line (marked with [ANCHOR] prefix).
// Returns -1 if not found.
func findAnchorLineIndex(lines []string) int {
	for i, line := range lines {
		if strings.HasPrefix(line, "[ANCHOR]") {
			return i
		}
	}
	return -1
}

// containsString checks if a slice contains a specific string.
func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

// contextSizeFn is a helper function to compute context size in runes (for UTF-8 safety).
// This can be used as an alternative to byte-based sizing if needed.
func contextSizeFn(lines []string) int {
	total := 0
	for _, line := range lines {
		total += utf8.RuneCountInString(line)
	}
	return total
}
