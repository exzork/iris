package repository

import (
	"context"
	"testing"
	"time"

	"github.com/eko/iris-bot/internal/domain"
)

func TestChannelMessage_UpsertWithEmbedding_PersistsAndReads(t *testing.T) {
	db := setupTestDB(t)
	defer closeTestDB(t, db)

	repo := NewChannelMessageRepo(db)
	ctx := context.Background()

	guildRepo := NewGuildRepo(db)
	guild := &domain.Guild{ID: 123, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := guildRepo.Create(ctx, guild); err != nil {
		t.Fatalf("failed to create guild: %v", err)
	}

	knownEmbedding := make([]float32, 384)
	for i := 0; i < 384; i++ {
		knownEmbedding[i] = float32(i) / 384.0
	}

	msg := &domain.ChannelMessage{
		GuildID:          123,
		ChannelID:        456,
		MessageID:        789,
		UserID:           111,
		AuthorName:       strPtr("testuser"),
		Content:          "test message",
		AttachmentCount:  0,
		IsBot:            false,
		TriggerSource:    "observe",
		CreatedAt:        time.Now(),
		ContentEmbedding: knownEmbedding,
	}

	err := repo.Upsert(ctx, msg)
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	retrieved, err := repo.GetByID(ctx, msg.GuildID, msg.MessageID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("retrieved message is nil")
	}

	if len(retrieved.ContentEmbedding) != 384 {
		t.Errorf("expected embedding length 384, got %d", len(retrieved.ContentEmbedding))
	}

	for i := 0; i < 384; i++ {
		expected := float32(i) / 384.0
		actual := retrieved.ContentEmbedding[i]
		if actual != expected {
			t.Errorf("embedding[%d]: expected %f, got %f", i, expected, actual)
		}
	}

	recent, err := repo.ListRecent(ctx, msg.GuildID, msg.ChannelID, 10)
	if err != nil {
		t.Fatalf("ListRecent failed: %v", err)
	}

	if len(recent) == 0 {
		t.Fatal("ListRecent returned no messages")
	}

	found := false
	for _, m := range recent {
		if m.MessageID == msg.MessageID {
			found = true
			if len(m.ContentEmbedding) != 384 {
				t.Errorf("ListRecent embedding length: expected 384, got %d", len(m.ContentEmbedding))
			}
			break
		}
	}

	if !found {
		t.Fatal("message not found in ListRecent results")
	}
}

func TestChannelMessage_UpsertWithoutEmbedding_StillWorks(t *testing.T) {
	db := setupTestDB(t)
	defer closeTestDB(t, db)

	repo := NewChannelMessageRepo(db)
	ctx := context.Background()

	guildRepo := NewGuildRepo(db)
	guild := &domain.Guild{ID: 124, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := guildRepo.Create(ctx, guild); err != nil {
		t.Fatalf("failed to create guild: %v", err)
	}

	msg := &domain.ChannelMessage{
		GuildID:          124,
		ChannelID:        456,
		MessageID:        790,
		UserID:           111,
		AuthorName:       strPtr("testuser"),
		Content:          "test message without embedding",
		AttachmentCount:  0,
		IsBot:            false,
		TriggerSource:    "observe",
		CreatedAt:        time.Now(),
		ContentEmbedding: nil,
	}

	err := repo.Upsert(ctx, msg)
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	retrieved, err := repo.GetByID(ctx, msg.GuildID, msg.MessageID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("retrieved message is nil")
	}

	if len(retrieved.ContentEmbedding) != 0 {
		t.Errorf("expected empty embedding, got length %d", len(retrieved.ContentEmbedding))
	}
}

func TestChannelMessage_UpsertUpdatesEmbedding(t *testing.T) {
	db := setupTestDB(t)
	defer closeTestDB(t, db)

	repo := NewChannelMessageRepo(db)
	ctx := context.Background()

	guildRepo := NewGuildRepo(db)
	guild := &domain.Guild{ID: 125, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := guildRepo.Create(ctx, guild); err != nil {
		t.Fatalf("failed to create guild: %v", err)
	}

	embedding1 := make([]float32, 384)
	for i := 0; i < 384; i++ {
		embedding1[i] = 0.1
	}

	msg := &domain.ChannelMessage{
		GuildID:          125,
		ChannelID:        456,
		MessageID:        791,
		UserID:           111,
		AuthorName:       strPtr("testuser"),
		Content:          "test message",
		AttachmentCount:  0,
		IsBot:            false,
		TriggerSource:    "observe",
		CreatedAt:        time.Now(),
		ContentEmbedding: embedding1,
	}

	err := repo.Upsert(ctx, msg)
	if err != nil {
		t.Fatalf("first Upsert failed: %v", err)
	}

	embedding2 := make([]float32, 384)
	for i := 0; i < 384; i++ {
		embedding2[i] = 0.2
	}

	msg.ContentEmbedding = embedding2
	msg.Content = "updated content"

	err = repo.Upsert(ctx, msg)
	if err != nil {
		t.Fatalf("second Upsert failed: %v", err)
	}

	retrieved, err := repo.GetByID(ctx, msg.GuildID, msg.MessageID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if len(retrieved.ContentEmbedding) != 384 {
		t.Errorf("expected embedding length 384, got %d", len(retrieved.ContentEmbedding))
	}

	for i := 0; i < 384; i++ {
		if retrieved.ContentEmbedding[i] != 0.2 {
			t.Errorf("embedding[%d]: expected 0.2, got %f", i, retrieved.ContentEmbedding[i])
		}
	}

	if retrieved.Content != "updated content" {
		t.Errorf("expected content 'updated content', got '%s'", retrieved.Content)
	}
}

func TestChannelMessage_UpsertDoesNotPruneMessages(t *testing.T) {
	db := setupTestDB(t)
	defer closeTestDB(t, db)

	repo := NewChannelMessageRepo(db)
	ctx := context.Background()

	guildRepo := NewGuildRepo(db)
	guild := &domain.Guild{ID: 126, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := guildRepo.Create(ctx, guild); err != nil {
		t.Fatalf("failed to create guild: %v", err)
	}

	// Insert 30 messages to verify none are pruned
	for i := 0; i < 30; i++ {
		msg := &domain.ChannelMessage{
			GuildID:       126,
			ChannelID:     456,
			MessageID:     int64(1000 + i),
			UserID:        111,
			AuthorName:    strPtr("testuser"),
			Content:       "test message " + string(rune('a'+i%26)),
			AttachmentCount: 0,
			IsBot:         false,
			TriggerSource: "observe",
			CreatedAt:     time.Now().Add(time.Duration(i) * time.Second),
		}

		err := repo.Upsert(ctx, msg)
		if err != nil {
			t.Fatalf("Upsert %d failed: %v", i, err)
		}
	}

	// Verify all 30 messages still exist
	recent, err := repo.ListRecent(ctx, 126, 456, 100)
	if err != nil {
		t.Fatalf("ListRecent failed: %v", err)
	}

	if len(recent) != 30 {
		t.Errorf("expected 30 messages, got %d (messages were pruned)", len(recent))
	}

	// Verify each message ID is present
	messageIDs := make(map[int64]bool)
	for _, msg := range recent {
		messageIDs[msg.MessageID] = true
	}

	for i := 0; i < 30; i++ {
		expectedID := int64(1000 + i)
		if !messageIDs[expectedID] {
			t.Errorf("message ID %d not found (was pruned)", expectedID)
		}
	}
}

func strPtr(s string) *string {
	return &s
}

func TestChannelMessageRepo_UpsertDoesNotPrune(t *testing.T) {
	// Verify that Upsert does not prune messages. Plan requires indefinite retention.
	db := setupTestDB(t)
	defer closeTestDB(t, db)

	repo := NewChannelMessageRepo(db)
	ctx := context.Background()

	guildRepo := NewGuildRepo(db)
	guild := &domain.Guild{ID: 999, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := guildRepo.Create(ctx, guild); err != nil {
		t.Fatalf("failed to create guild: %v", err)
	}

	// Insert 25 messages
	for i := 0; i < 25; i++ {
		msg := &domain.ChannelMessage{
			GuildID:       999,
			ChannelID:     888,
			MessageID:     int64(5000 + i),
			UserID:        222,
			AuthorName:    strPtr("testuser"),
			Content:       "message " + string(rune('a'+i%26)),
			AttachmentCount: 0,
			IsBot:         false,
			TriggerSource: "observe",
			CreatedAt:     time.Now().Add(time.Duration(i) * time.Second),
		}

		err := repo.Upsert(ctx, msg)
		if err != nil {
			t.Fatalf("Upsert %d failed: %v", i, err)
		}
	}

	// Verify all 25 messages remain (no pruning)
	recent, err := repo.ListRecent(ctx, 999, 888, 100)
	if err != nil {
		t.Fatalf("ListRecent failed: %v", err)
	}

	if len(recent) != 25 {
		t.Fatalf("expected 25 messages, got %d (pruning occurred)", len(recent))
	}

	// Verify each message ID is present
	for i := 0; i < 25; i++ {
		expectedID := int64(5000 + i)
		found := false
		for _, msg := range recent {
			if msg.MessageID == expectedID {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("message ID %d not found (was pruned)", expectedID)
		}
	}
}
