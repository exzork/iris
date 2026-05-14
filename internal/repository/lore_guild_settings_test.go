package repository

import (
	"context"
	"testing"
	"time"

	"github.com/eko/iris-bot/internal/domain"
)

func TestLoreGuildSettingsRepo(t *testing.T) {
	db := setupTestDB(t)
	defer closeTestDB(t, db)

	guildRepo := NewGuildRepo(db)
	repo := NewLoreGuildSettingsRepo(db)
	ctx := context.Background()

	guild := &domain.Guild{ID: 666666666, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	guildRepo.Create(ctx, guild)

	t.Run("SetEnabledCreatesSettings", func(t *testing.T) {
		guildID := int64(666666666)

		err := repo.SetEnabled(ctx, guildID, true)
		if err != nil {
			t.Fatalf("SetEnabled failed: %v", err)
		}

		enabled, err := repo.IsEnabled(ctx, guildID)
		if err != nil {
			t.Fatalf("IsEnabled failed: %v", err)
		}
		if !enabled {
			t.Error("expected IsEnabled to return true")
		}
	})

	t.Run("SetEnabledUpdatesExisting", func(t *testing.T) {
		guildID := int64(666666666)

		err := repo.SetEnabled(ctx, guildID, true)
		if err != nil {
			t.Fatalf("first SetEnabled failed: %v", err)
		}

		err = repo.SetEnabled(ctx, guildID, false)
		if err != nil {
			t.Fatalf("second SetEnabled failed: %v", err)
		}

		enabled, err := repo.IsEnabled(ctx, guildID)
		if err != nil {
			t.Fatalf("IsEnabled failed: %v", err)
		}
		if enabled {
			t.Error("expected IsEnabled to return false after update")
		}
	})

	t.Run("GetSettingsReturnsSettings", func(t *testing.T) {
		guildID := int64(666666666)

		err := repo.SetEnabled(ctx, guildID, true)
		if err != nil {
			t.Fatalf("SetEnabled failed: %v", err)
		}

		settings, err := repo.GetSettings(ctx, guildID)
		if err != nil {
			t.Fatalf("GetSettings failed: %v", err)
		}
		if settings.GuildID != guildID {
			t.Errorf("expected guild ID %d, got %d", guildID, settings.GuildID)
		}
		if !settings.Enabled {
			t.Error("expected Enabled to be true")
		}
		if settings.ThreadCapPerHour != 6 {
			t.Errorf("expected ThreadCapPerHour 6, got %d", settings.ThreadCapPerHour)
		}
	})

	t.Run("IncrementThreadCountCreatesCounter", func(t *testing.T) {
		guildID := int64(555555555)
		guild2 := &domain.Guild{ID: guildID, CreatedAt: time.Now(), UpdatedAt: time.Now()}
		guildRepo.Create(ctx, guild2)

		now := time.Now()
		hourBucket := now.Truncate(time.Hour)

		err := repo.IncrementThreadCount(ctx, guildID, hourBucket)
		if err != nil {
			t.Fatalf("IncrementThreadCount failed: %v", err)
		}

		count, err := repo.CountThreadsThisHour(ctx, guildID, now)
		if err != nil {
			t.Fatalf("CountThreadsThisHour failed: %v", err)
		}
		if count != 1 {
			t.Errorf("expected count 1, got %d", count)
		}
	})

	t.Run("IncrementThreadCountIncrementsExisting", func(t *testing.T) {
		guildID := int64(444444444)
		guild3 := &domain.Guild{ID: guildID, CreatedAt: time.Now(), UpdatedAt: time.Now()}
		guildRepo.Create(ctx, guild3)

		now := time.Now()
		hourBucket := now.Truncate(time.Hour)

		err := repo.IncrementThreadCount(ctx, guildID, hourBucket)
		if err != nil {
			t.Fatalf("first IncrementThreadCount failed: %v", err)
		}

		err = repo.IncrementThreadCount(ctx, guildID, hourBucket)
		if err != nil {
			t.Fatalf("second IncrementThreadCount failed: %v", err)
		}

		count, err := repo.CountThreadsThisHour(ctx, guildID, now)
		if err != nil {
			t.Fatalf("CountThreadsThisHour failed: %v", err)
		}
		if count != 2 {
			t.Errorf("expected count 2, got %d", count)
		}
	})

	t.Run("CountThreadsThisHourIsolatedByHour", func(t *testing.T) {
		guildID := int64(333333333)
		guild4 := &domain.Guild{ID: guildID, CreatedAt: time.Now(), UpdatedAt: time.Now()}
		guildRepo.Create(ctx, guild4)

		now := time.Now()
		currentHour := now.Truncate(time.Hour)
		nextHour := currentHour.Add(time.Hour)

		err := repo.IncrementThreadCount(ctx, guildID, currentHour)
		if err != nil {
			t.Fatalf("IncrementThreadCount for current hour failed: %v", err)
		}

		err = repo.IncrementThreadCount(ctx, guildID, nextHour)
		if err != nil {
			t.Fatalf("IncrementThreadCount for next hour failed: %v", err)
		}

		currentCount, err := repo.CountThreadsThisHour(ctx, guildID, now)
		if err != nil {
			t.Fatalf("CountThreadsThisHour for current hour failed: %v", err)
		}
		if currentCount != 1 {
			t.Errorf("expected current hour count 1, got %d", currentCount)
		}
	})

	t.Run("IsEnabledMissingGuildReturnsDefaultFalse", func(t *testing.T) {
		guildID := int64(777777777)
		guild5 := &domain.Guild{ID: guildID, CreatedAt: time.Now(), UpdatedAt: time.Now()}
		guildRepo.Create(ctx, guild5)

		enabled, err := repo.IsEnabled(ctx, guildID)
		if err != nil {
			t.Fatalf("IsEnabled failed: %v", err)
		}
		if enabled {
			t.Error("expected IsEnabled to return false for missing guild settings")
		}
	})

	t.Run("IsEnabledWithEnabledTrue", func(t *testing.T) {
		guildID := int64(888888888)
		guild6 := &domain.Guild{ID: guildID, CreatedAt: time.Now(), UpdatedAt: time.Now()}
		guildRepo.Create(ctx, guild6)

		err := repo.SetEnabled(ctx, guildID, true)
		if err != nil {
			t.Fatalf("SetEnabled failed: %v", err)
		}

		enabled, err := repo.IsEnabled(ctx, guildID)
		if err != nil {
			t.Fatalf("IsEnabled failed: %v", err)
		}
		if !enabled {
			t.Error("expected IsEnabled to return true")
		}
	})

	t.Run("IsEnabledWithEnabledFalse", func(t *testing.T) {
		guildID := int64(999999999)
		guild7 := &domain.Guild{ID: guildID, CreatedAt: time.Now(), UpdatedAt: time.Now()}
		guildRepo.Create(ctx, guild7)

		err := repo.SetEnabled(ctx, guildID, false)
		if err != nil {
			t.Fatalf("SetEnabled failed: %v", err)
		}

		enabled, err := repo.IsEnabled(ctx, guildID)
		if err != nil {
			t.Fatalf("IsEnabled failed: %v", err)
		}
		if enabled {
			t.Error("expected IsEnabled to return false")
		}
	})

	t.Run("CountThreadsThisHourMissingBucketReturnsZero", func(t *testing.T) {
		guildID := int64(111111111)
		guild8 := &domain.Guild{ID: guildID, CreatedAt: time.Now(), UpdatedAt: time.Now()}
		guildRepo.Create(ctx, guild8)

		now := time.Now()
		count, err := repo.CountThreadsThisHour(ctx, guildID, now)
		if err != nil {
			t.Fatalf("CountThreadsThisHour failed: %v", err)
		}
		if count != 0 {
			t.Errorf("expected count 0 for missing bucket, got %d", count)
		}
	})

	t.Run("CountThreadsThisHourAfterIncrement", func(t *testing.T) {
		guildID := int64(222222222)
		guild9 := &domain.Guild{ID: guildID, CreatedAt: time.Now(), UpdatedAt: time.Now()}
		guildRepo.Create(ctx, guild9)

		now := time.Now()
		hourBucket := now.Truncate(time.Hour)

		err := repo.IncrementThreadCount(ctx, guildID, hourBucket)
		if err != nil {
			t.Fatalf("IncrementThreadCount failed: %v", err)
		}

		count, err := repo.CountThreadsThisHour(ctx, guildID, now)
		if err != nil {
			t.Fatalf("CountThreadsThisHour failed: %v", err)
		}
		if count != 1 {
			t.Errorf("expected count 1, got %d", count)
		}
	})
}
