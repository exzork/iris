package orchestrator

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/eko/iris-bot/internal/domain"
)

func TestE2E_ThreadReplyGetsAnchoredContext(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC)

	anchorResolver := NewFakeLoreAnchorResolver()
	channelResolver := NewFakeChannelNameResolver()
	allowedLister := NewFakeAllowedChannelLister()

	guildID := int64(111111)
	threadID := int64(999999)
	channelID := int64(222222)

	anchor := &domain.LoreThreadAnchor{
		ID:               1,
		GuildID:          guildID,
		ChannelID:        channelID,
		ThreadID:         threadID,
		SummaryMessageID: nil,
		SummaryText:      stringPtr("This is the anchor summary message with lore context."),
		Title:            stringPtr("Ringkasan Lore"),
		SourceSessionID:  nil,
		CreatedAt:        now,
	}

	anchorResolver.AddAnchor(anchor)
	channelResolver.SetNames(threadID, "lore-channel", "lore-thread")
	allowedLister.SetAllowed(guildID, []int64{channelID})

	anchorLines, err := buildLoreAnchorLines(ctx, channelResolver, anchor)
	if err != nil {
		t.Fatalf("buildLoreAnchorLines failed: %v", err)
	}

	if len(anchorLines) != 1 {
		t.Errorf("Expected 1 anchor line, got %d", len(anchorLines))
	}

	if len(anchorLines) > 0 {
		line := anchorLines[0]
		if !strings.Contains(line, "This is the anchor summary message") {
			t.Errorf("Anchor line does not contain summary text: %q", line)
		}
	}

	allowed, err := isThreadAllowed(ctx, allowedLister, anchor)
	if err != nil {
		t.Fatalf("isThreadAllowed failed: %v", err)
	}
	if !allowed {
		t.Error("Expected thread to be allowed")
	}
}

func TestE2E_CompactionAt70Percent(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC)

	archiver := NewFakeEpisodeArchiver()
	compactor := NewLoreCompactor(1000, archiver)

	guildID := int64(111111)
	threadID := int64(999999)
	channelID := int64(222222)

	anchor := &domain.LoreThreadAnchor{
		ID:               1,
		GuildID:          guildID,
		ChannelID:        channelID,
		ThreadID:         threadID,
		SummaryMessageID: nil,
		SummaryText:      stringPtr("Anchor summary message"),
		Title:            stringPtr("Ringkasan"),
		SourceSessionID:  nil,
		CreatedAt:        now,
	}

	lines := []string{
		"lore-channel|lore-thread|0|2026-05-13T10:00:00Z|Anchor summary message",
		"lore-channel|lore-thread|333333|2026-05-13T10:01:00Z|Message 1 with some content",
		"lore-channel|lore-thread|333334|2026-05-13T10:02:00Z|Message 2 with more content",
		"lore-channel|lore-thread|333335|2026-05-13T10:03:00Z|Message 3 with additional content",
		"lore-channel|lore-thread|333336|2026-05-13T10:04:00Z|Message 4 with even more content",
		"lore-channel|lore-thread|333337|2026-05-13T10:05:00Z|Message 5 with lots of content",
		"lore-channel|lore-thread|333338|2026-05-13T10:06:00Z|Message 6 with extensive content",
		"lore-channel|lore-thread|333339|2026-05-13T10:07:00Z|Message 7 with comprehensive content",
		"lore-channel|lore-thread|333340|2026-05-13T10:08:00Z|Message 8 with detailed content",
		"lore-channel|lore-thread|333341|2026-05-13T10:09:00Z|Message 9 with thorough content",
		"lore-channel|lore-thread|333342|2026-05-13T10:10:00Z|Message 10 with complete content",
	}

	originalSize := contextSize(lines)
	if originalSize <= compactor.Limit {
		t.Skipf("Test context size (%d) not over limit (%d), skipping compaction test", originalSize, compactor.Limit)
	}

	compacted, report, err := compactor.CompactForLoreThread(ctx, anchor, lines)
	if err != nil {
		t.Fatalf("CompactForLoreThread failed: %v", err)
	}

	if !report.Compacted {
		t.Error("Expected compaction to occur")
	}

	if report.OriginalSize <= compactor.Limit {
		t.Errorf("Original size (%d) should exceed limit (%d)", report.OriginalSize, compactor.Limit)
	}

	if report.FinalSize > compactor.Limit {
		t.Errorf("Final size (%d) exceeds limit (%d)", report.FinalSize, compactor.Limit)
	}

	targetSize := int(float64(compactor.Limit) * compactor.RetentionTarget)
	tolerance := int(float64(compactor.Limit) * 0.05)
	minSize := targetSize - tolerance
	maxSize := targetSize + tolerance

	if report.FinalSize < minSize || report.FinalSize > maxSize {
		t.Errorf("Final size (%d) not within ±5%% of target (%d): range [%d, %d]", report.FinalSize, targetSize, minSize, maxSize)
	}

	if report.RetentionRatio < 0.65 || report.RetentionRatio > 0.75 {
		t.Errorf("Retention ratio (%.2f) not within expected range [0.65, 0.75]", report.RetentionRatio)
	}

	if len(compacted) == 0 {
		t.Error("Expected compacted lines, got empty slice")
	}

	anchorFound := false
	for _, line := range compacted {
		if strings.Contains(line, "Anchor summary message") {
			anchorFound = true
			break
		}
	}
	if !anchorFound {
		t.Error("Anchor line not preserved in compacted context")
	}

	if archiver.ArchivedCount() == 0 {
		t.Error("Expected archived content, got none")
	}
}

func TestE2E_CompactionPreservesAnchor(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC)

	archiver := NewFakeEpisodeArchiver()
	compactor := NewLoreCompactor(500, archiver)

	anchor := &domain.LoreThreadAnchor{
		ID:               1,
		GuildID:          111111,
		ChannelID:        222222,
		ThreadID:         999999,
		SummaryMessageID: nil,
		SummaryText:      stringPtr("Critical anchor summary that must be preserved"),
		Title:            stringPtr("Ringkasan"),
		SourceSessionID:  nil,
		CreatedAt:        now,
	}

	lines := []string{
		"lore-channel|lore-thread|0|2026-05-13T10:00:00Z|Critical anchor summary that must be preserved",
		"lore-channel|lore-thread|333333|2026-05-13T10:01:00Z|Message 1",
		"lore-channel|lore-thread|333334|2026-05-13T10:02:00Z|Message 2",
		"lore-channel|lore-thread|333335|2026-05-13T10:03:00Z|Message 3",
		"lore-channel|lore-thread|333336|2026-05-13T10:04:00Z|Message 4",
		"lore-channel|lore-thread|333337|2026-05-13T10:05:00Z|Message 5",
	}

	compacted, report, err := compactor.CompactForLoreThread(ctx, anchor, lines)
	if err != nil {
		t.Fatalf("CompactForLoreThread failed: %v", err)
	}

	if len(compacted) == 0 {
		t.Fatal("Expected compacted lines, got empty slice")
	}

	anchorFound := false
	for _, line := range compacted {
		if strings.Contains(line, "Critical anchor summary that must be preserved") {
			anchorFound = true
			break
		}
	}
	if !anchorFound {
		t.Error("Anchor line was not preserved during compaction")
	}

	if report.Compacted {
		if report.RetentionRatio < 0.5 {
			t.Errorf("Retention ratio too low: %.2f", report.RetentionRatio)
		}
	}
}

func TestE2E_NonLoreThreadNoCompaction(t *testing.T) {
	ctx := context.Background()

	archiver := NewFakeEpisodeArchiver()
	compactor := NewLoreCompactor(500, archiver)

	lines := []string{
		"channel|thread|333333|2026-05-13T10:01:00Z|Message 1",
		"channel|thread|333334|2026-05-13T10:02:00Z|Message 2",
	}

	compacted, report, err := compactor.CompactForLoreThread(ctx, nil, lines)
	if err != nil {
		t.Fatalf("CompactForLoreThread failed: %v", err)
	}

	if compacted != nil {
		t.Error("Expected nil return for non-lore thread, got compacted lines")
	}

	if report.Compacted {
		t.Error("Expected no compaction for non-lore thread")
	}

	if archiver.ArchivedCount() != 0 {
		t.Errorf("Expected no archiving for non-lore thread, got %d archived items", archiver.ArchivedCount())
	}
}

func stringPtr(s string) *string {
	return &s
}
