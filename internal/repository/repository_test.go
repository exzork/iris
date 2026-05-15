package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/eko/iris-bot/internal/domain"
)

func TestGuildRepo(t *testing.T) {
	db := setupTestDB(t)
	defer closeTestDB(t, db)

	repo := NewGuildRepo(db)
	ctx := context.Background()

	t.Run("Create and GetByID", func(t *testing.T) {
		guild := &domain.Guild{
			ID:        123456789,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		err := repo.Create(ctx, guild)
		if err != nil {
			t.Fatalf("failed to create guild: %v", err)
		}

		retrieved, err := repo.GetByID(ctx, guild.ID)
		if err != nil {
			t.Fatalf("failed to get guild: %v", err)
		}

		if retrieved.ID != guild.ID {
			t.Errorf("expected guild ID %d, got %d", guild.ID, retrieved.ID)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		guild := &domain.Guild{
			ID:        987654321,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		err := repo.Create(ctx, guild)
		if err != nil {
			t.Fatalf("failed to create guild: %v", err)
		}

		err = repo.Delete(ctx, guild.ID)
		if err != nil {
			t.Fatalf("failed to delete guild: %v", err)
		}

		_, err = repo.GetByID(ctx, guild.ID)
		if err == nil {
			t.Errorf("expected error when getting deleted guild, got nil")
		}
	})

	t.Run("EnsureGuild is idempotent", func(t *testing.T) {
		guildID := int64(111222333)

		err := repo.EnsureGuild(ctx, guildID)
		if err != nil {
			t.Fatalf("first EnsureGuild failed: %v", err)
		}

		retrieved1, err := repo.GetByID(ctx, guildID)
		if err != nil {
			t.Fatalf("failed to get guild after first ensure: %v", err)
		}

		err = repo.EnsureGuild(ctx, guildID)
		if err != nil {
			t.Fatalf("second EnsureGuild failed: %v", err)
		}

		retrieved2, err := repo.GetByID(ctx, guildID)
		if err != nil {
			t.Fatalf("failed to get guild after second ensure: %v", err)
		}

		if retrieved1.ID != retrieved2.ID {
			t.Errorf("guild ID changed after second ensure: %d vs %d", retrieved1.ID, retrieved2.ID)
		}
	})
}

func TestSettingsRepo(t *testing.T) {
	db := setupTestDB(t)
	defer closeTestDB(t, db)

	guildRepo := NewGuildRepo(db)
	settingsRepo := NewSettingsRepo(db)
	ctx := context.Background()

	guild := &domain.Guild{ID: 111111111, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	guildRepo.Create(ctx, guild)

	t.Run("Save and GetByKey", func(t *testing.T) {
		config := &domain.GuildConfig{
			GuildID:      guild.ID,
			SettingKey:   "prefix",
			SettingValue: "!",
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}

		err := settingsRepo.Save(ctx, config)
		if err != nil {
			t.Fatalf("failed to save config: %v", err)
		}

		retrieved, err := settingsRepo.GetByKey(ctx, guild.ID, "prefix")
		if err != nil {
			t.Fatalf("failed to get config: %v", err)
		}

		if retrieved.SettingValue != "!" {
			t.Errorf("expected value '!', got '%s'", retrieved.SettingValue)
		}
	})

	t.Run("GetAllByGuild", func(t *testing.T) {
		settingsRepo.Save(ctx, &domain.GuildConfig{
			GuildID:      guild.ID,
			SettingKey:   "lang",
			SettingValue: "id",
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		})

		configs, err := settingsRepo.GetAllByGuild(ctx, guild.ID)
		if err != nil {
			t.Fatalf("failed to get all configs: %v", err)
		}

		if len(configs) < 2 {
			t.Errorf("expected at least 2 configs, got %d", len(configs))
		}
	})

	t.Run("Delete", func(t *testing.T) {
		err := settingsRepo.Delete(ctx, guild.ID, "prefix")
		if err != nil {
			t.Fatalf("failed to delete config: %v", err)
		}

		_, err = settingsRepo.GetByKey(ctx, guild.ID, "prefix")
		if err == nil {
			t.Error("expected error when getting deleted config")
		}
	})
}

func TestMemoryRepo(t *testing.T) {
	db := setupTestDB(t)
	defer closeTestDB(t, db)

	guildRepo := NewGuildRepo(db)
	memoryRepo := NewMemoryRepo(db)
	ctx := context.Background()

	guild := &domain.Guild{ID: 222222222, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	guildRepo.Create(ctx, guild)

	t.Run("Save and GetByGuild", func(t *testing.T) {
		embedding := make([]float32, 1536)
		for i := range embedding {
			embedding[i] = float32(i) / 1536.0
		}

		err := memoryRepo.Save(ctx, guild.ID, 0, "test memory", embedding)
		if err != nil {
			t.Fatalf("failed to save memory: %v", err)
		}

		records, err := memoryRepo.GetByGuild(ctx, guild.ID, 10)
		if err != nil {
			t.Fatalf("failed to get memories: %v", err)
		}

		if len(records) == 0 {
			t.Error("expected at least 1 memory record")
		}

		if records[0].Content != "test memory" {
			t.Errorf("expected content 'test memory', got '%s'", records[0].Content)
		}
	})

	t.Run("SearchSimilar", func(t *testing.T) {
		embedding := make([]float32, 1536)
		for i := range embedding {
			embedding[i] = float32(i) / 1536.0
		}

		records, err := memoryRepo.SearchSimilar(ctx, guild.ID, embedding, 5)
		if err != nil {
			t.Fatalf("failed to search similar: %v", err)
		}

		if len(records) == 0 {
			t.Error("expected at least 1 similar record")
		}
	})

	t.Run("Guild scoping", func(t *testing.T) {
		guild2 := &domain.Guild{ID: 333333333, CreatedAt: time.Now(), UpdatedAt: time.Now()}
		guildRepo.Create(ctx, guild2)

		embedding := make([]float32, 1536)
		memoryRepo.Save(ctx, guild2.ID, 0, "guild2 memory", embedding)

		records, err := memoryRepo.GetByGuild(ctx, guild.ID, 100)
		if err != nil {
			t.Fatalf("failed to get memories: %v", err)
		}

		for _, r := range records {
			if r.GuildID != guild.ID {
				t.Errorf("memory record has wrong guild_id: %d", r.GuildID)
			}
		}
	})
}

func TestLoreRepo(t *testing.T) {
	db := setupTestDB(t)
	defer closeTestDB(t, db)

	guildRepo := NewGuildRepo(db)
	loreRepo := NewLoreRepo(db)
	ctx := context.Background()

	guild := &domain.Guild{ID: 444444444, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	guildRepo.Create(ctx, guild)

	t.Run("CreateDocument and SaveChunk", func(t *testing.T) {
		docID, err := loreRepo.CreateDocument(ctx, guild.ID, "Wuthering Waves", "Lore content")
		if err != nil {
			t.Fatalf("failed to create document: %v", err)
		}

		embedding := make([]float32, 1536)
		for i := range embedding {
			embedding[i] = float32(i) / 1536.0
		}

		err = loreRepo.SaveChunk(ctx, guild.ID, docID, "chunk text", embedding, 0)
		if err != nil {
			t.Fatalf("failed to save chunk: %v", err)
		}

		chunks, err := loreRepo.GetChunksByDocument(ctx, guild.ID, docID)
		if err != nil {
			t.Fatalf("failed to get chunks: %v", err)
		}

		if len(chunks) == 0 {
			t.Error("expected at least 1 chunk")
		}
	})

	t.Run("SearchChunks", func(t *testing.T) {
		docID, _ := loreRepo.CreateDocument(ctx, guild.ID, "Lore2", "Content2")

		embedding := make([]float32, 1536)
		for i := range embedding {
			embedding[i] = float32(i) / 1536.0
		}

		loreRepo.SaveChunk(ctx, guild.ID, docID, "searchable chunk", embedding, 0)

		results, err := loreRepo.SearchChunks(ctx, guild.ID, embedding, 5)
		if err != nil {
			t.Fatalf("failed to search chunks: %v", err)
		}

		if len(results) == 0 {
			t.Error("expected at least 1 search result")
		}
	})
}

func TestExceptionChannelRepo(t *testing.T) {
	db := setupTestDB(t)
	defer closeTestDB(t, db)

	guildRepo := NewGuildRepo(db)
	exRepo := NewExceptionChannelRepo(db)
	ctx := context.Background()

	guild := &domain.Guild{ID: 555555555, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	guildRepo.Create(ctx, guild)

	t.Run("Add and IsException", func(t *testing.T) {
		err := exRepo.Add(ctx, guild.ID, 111)
		if err != nil {
			t.Fatalf("failed to add exception channel: %v", err)
		}

		exists, err := exRepo.IsException(ctx, guild.ID, 111)
		if err != nil {
			t.Fatalf("failed to check exception: %v", err)
		}

		if !exists {
			t.Error("expected exception channel to exist")
		}
	})

	t.Run("GetByGuild", func(t *testing.T) {
		exRepo.Add(ctx, guild.ID, 222)
		exRepo.Add(ctx, guild.ID, 333)

		channels, err := exRepo.GetByGuild(ctx, guild.ID)
		if err != nil {
			t.Fatalf("failed to get exception channels: %v", err)
		}

		if len(channels) < 3 {
			t.Errorf("expected at least 3 channels, got %d", len(channels))
		}
	})

	t.Run("Remove", func(t *testing.T) {
		err := exRepo.Remove(ctx, guild.ID, 111)
		if err != nil {
			t.Fatalf("failed to remove exception channel: %v", err)
		}

		exists, err := exRepo.IsException(ctx, guild.ID, 111)
		if err != nil {
			t.Fatalf("failed to check exception: %v", err)
		}

		if exists {
			t.Error("expected exception channel to be removed")
		}
	})
}

func TestToolLogRepo(t *testing.T) {
	db := setupTestDB(t)
	defer closeTestDB(t, db)

	guildRepo := NewGuildRepo(db)
	toolRepo := NewToolLogRepo(db)
	ctx := context.Background()

	guild := &domain.Guild{ID: 666666666, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	guildRepo.Create(ctx, guild)

	t.Run("Save and GetByGuild", func(t *testing.T) {
		inputData := map[string]interface{}{"query": "test"}
		outputData := map[string]interface{}{"result": "success"}

		err := toolRepo.Save(ctx, guild.ID, 123, "search", inputData, outputData, "success", "")
		if err != nil {
			t.Fatalf("failed to save tool log: %v", err)
		}

		logs, err := toolRepo.GetByGuild(ctx, guild.ID, 10)
		if err != nil {
			t.Fatalf("failed to get tool logs: %v", err)
		}

		if len(logs) == 0 {
			t.Error("expected at least 1 tool log")
		}
	})
}

func TestReminderRepo(t *testing.T) {
	db := setupTestDB(t)
	defer closeTestDB(t, db)

	guildRepo := NewGuildRepo(db)
	reminderRepo := NewReminderRepo(db)
	ctx := context.Background()

	guild := &domain.Guild{ID: 777777777, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	guildRepo.Create(ctx, guild)

	t.Run("Save and GetByGuild", func(t *testing.T) {
		scheduledFor := time.Now().Add(1 * time.Hour)
		err := reminderRepo.Save(ctx, guild.ID, 123, 456, "test reminder", scheduledFor)
		if err != nil {
			t.Fatalf("failed to save reminder: %v", err)
		}

		reminders, err := reminderRepo.GetByGuild(ctx, guild.ID, 10)
		if err != nil {
			t.Fatalf("failed to get reminders: %v", err)
		}

		if len(reminders) == 0 {
			t.Error("expected at least 1 reminder")
		}
	})

	t.Run("GetDue", func(t *testing.T) {
		scheduledFor := time.Now().Add(-1 * time.Hour)
		reminderRepo.Save(ctx, guild.ID, 123, 456, "due reminder", scheduledFor)

		reminders, err := reminderRepo.GetDue(ctx, guild.ID)
		if err != nil {
			t.Fatalf("failed to get due reminders: %v", err)
		}

		if len(reminders) == 0 {
			t.Error("expected at least 1 due reminder")
		}
	})
}

func TestAuditRepo(t *testing.T) {
	db := setupTestDB(t)
	defer closeTestDB(t, db)

	guildRepo := NewGuildRepo(db)
	auditRepo := NewAuditRepo(db)
	ctx := context.Background()

	guild := &domain.Guild{ID: 888888888, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	guildRepo.Create(ctx, guild)

	t.Run("Log and GetByGuild", func(t *testing.T) {
		changes := map[string]interface{}{"old": "value1", "new": "value2"}
		err := auditRepo.Log(ctx, guild.ID, 123, "update", "config", "prefix", changes)
		if err != nil {
			t.Fatalf("failed to log audit event: %v", err)
		}

		events, err := auditRepo.GetByGuild(ctx, guild.ID, 10)
		if err != nil {
			t.Fatalf("failed to get audit events: %v", err)
		}

		if len(events) == 0 {
			t.Error("expected at least 1 audit event")
		}
	})
}

func TestAllowedChannelRepo(t *testing.T) {
	db := setupTestDB(t)
	defer closeTestDB(t, db)

	guildRepo := NewGuildRepo(db)
	allowedRepo := NewAllowedChannelRepo(db)
	ctx := context.Background()

	guild := &domain.Guild{ID: 999999999, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	guildRepo.Create(ctx, guild)

	t.Run("Add and IsAllowed", func(t *testing.T) {
		err := allowedRepo.Add(ctx, guild.ID, 111)
		if err != nil {
			t.Fatalf("failed to add allowed channel: %v", err)
		}

		allowed, err := allowedRepo.IsAllowed(ctx, guild.ID, 111)
		if err != nil {
			t.Fatalf("failed to check allowed channel: %v", err)
		}

		if !allowed {
			t.Error("expected channel to be allowed")
		}
	})

	t.Run("IsAllowed returns false for non-allowed channel", func(t *testing.T) {
		allowed, err := allowedRepo.IsAllowed(ctx, guild.ID, 999)
		if err != nil {
			t.Fatalf("failed to check allowed channel: %v", err)
		}

		if allowed {
			t.Error("expected channel to not be allowed")
		}
	})

	t.Run("HasAny distinguishes empty from denied state", func(t *testing.T) {
		// Create a new guild with no allowed channels
		guild2 := &domain.Guild{ID: 1000000000, CreatedAt: time.Now(), UpdatedAt: time.Now()}
		guildRepo.Create(ctx, guild2)

		hasAny, err := allowedRepo.HasAny(ctx, guild2.ID)
		if err != nil {
			t.Fatalf("failed to check HasAny: %v", err)
		}

		if hasAny {
			t.Error("expected HasAny to be false for guild with no allowed channels")
		}

		// Add one allowed channel
		err = allowedRepo.Add(ctx, guild2.ID, 222)
		if err != nil {
			t.Fatalf("failed to add allowed channel: %v", err)
		}

		hasAny, err = allowedRepo.HasAny(ctx, guild2.ID)
		if err != nil {
			t.Fatalf("failed to check HasAny after add: %v", err)
		}

		if !hasAny {
			t.Error("expected HasAny to be true after adding allowed channel")
		}
	})

	t.Run("ListByGuild", func(t *testing.T) {
		allowedRepo.Add(ctx, guild.ID, 333)
		allowedRepo.Add(ctx, guild.ID, 444)

		channels, err := allowedRepo.ListByGuild(ctx, guild.ID)
		if err != nil {
			t.Fatalf("failed to list allowed channels: %v", err)
		}

		if len(channels) < 3 {
			t.Errorf("expected at least 3 channels, got %d", len(channels))
		}
	})

	t.Run("Remove", func(t *testing.T) {
		err := allowedRepo.Remove(ctx, guild.ID, 111)
		if err != nil {
			t.Fatalf("failed to remove allowed channel: %v", err)
		}

		allowed, err := allowedRepo.IsAllowed(ctx, guild.ID, 111)
		if err != nil {
			t.Fatalf("failed to check allowed channel: %v", err)
		}

		if allowed {
			t.Error("expected channel to be removed")
		}
	})
}

func TestChannelMessageRepo(t *testing.T) {
	db := setupTestDB(t)
	defer closeTestDB(t, db)

	guildRepo := NewGuildRepo(db)
	msgRepo := NewChannelMessageRepo(db)
	ctx := context.Background()

	guild := &domain.Guild{ID: 1111111111, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	guildRepo.Create(ctx, guild)

	t.Run("Upsert and GetByID", func(t *testing.T) {
		authorName := "TestUser"
		msg := &domain.ChannelMessage{
			GuildID:           guild.ID,
			ChannelID:         100,
			MessageID:         1001,
			UserID:            2001,
			AuthorName:        &authorName,
			Content:           "Hello world",
			AttachmentCount:   0,
			ReplyToMessageID:  nil,
			ReplyToChannelID:  nil,
			IsBot:             false,
			TriggerSource:     "observe",
			CreatedAt:         time.Now(),
			EditedAt:          nil,
			DeletedAt:         nil,
		}

		err := msgRepo.Upsert(ctx, msg)
		if err != nil {
			t.Fatalf("failed to upsert message: %v", err)
		}

		retrieved, err := msgRepo.GetByID(ctx, guild.ID, 1001)
		if err != nil {
			t.Fatalf("failed to get message: %v", err)
		}

		if retrieved == nil {
			t.Fatal("expected message to be retrieved")
		}

		if retrieved.Content != "Hello world" {
			t.Errorf("expected content 'Hello world', got '%s'", retrieved.Content)
		}
	})

	t.Run("Upsert does not prune messages (stores all indefinitely)", func(t *testing.T) {
		for i := 1; i <= 25; i++ {
			authorName := "TestUser"
			msg := &domain.ChannelMessage{
				GuildID:         guild.ID,
				ChannelID:       200,
				MessageID:       int64(2000 + i),
				UserID:          3001,
				AuthorName:      &authorName,
				Content:         fmt.Sprintf("Message %d", i),
				AttachmentCount: 0,
				IsBot:           false,
				TriggerSource:   "observe",
				CreatedAt:       time.Now().Add(time.Duration(i) * time.Second),
			}
			err := msgRepo.Upsert(ctx, msg)
			if err != nil {
				t.Fatalf("failed to upsert message %d: %v", i, err)
			}
		}

		// Query recent messages
		recent, err := msgRepo.ListRecent(ctx, guild.ID, 200, 100)
		if err != nil {
			t.Fatalf("failed to list recent messages: %v", err)
		}

		if len(recent) != 25 {
			t.Errorf("expected all 25 messages (no pruning), got %d", len(recent))
		}

		// Verify all messages are retained
		if recent[len(recent)-1].MessageID != 2025 {
			t.Errorf("expected newest message ID 2025, got %d", recent[len(recent)-1].MessageID)
		}
	})

	t.Run("ListRecent returns messages in chronological order", func(t *testing.T) {
		recent, err := msgRepo.ListRecent(ctx, guild.ID, 200, 10)
		if err != nil {
			t.Fatalf("failed to list recent messages: %v", err)
		}

		if len(recent) == 0 {
			t.Fatal("expected at least 1 message")
		}

		// Verify chronological order (oldest first)
		for i := 1; i < len(recent); i++ {
			if recent[i].CreatedAt.Before(recent[i-1].CreatedAt) {
				t.Error("expected messages in chronological order")
			}
		}
	})

	t.Run("Reply metadata round-trips", func(t *testing.T) {
		replyToMsgID := int64(5001)
		replyToChanID := int64(500)
		authorName := "ReplyUser"

		msg := &domain.ChannelMessage{
			GuildID:          guild.ID,
			ChannelID:        300,
			MessageID:        3001,
			UserID:           4001,
			AuthorName:       &authorName,
			Content:          "This is a reply",
			AttachmentCount:  0,
			ReplyToMessageID: &replyToMsgID,
			ReplyToChannelID: &replyToChanID,
			IsBot:            false,
			TriggerSource:    "observe",
			CreatedAt:        time.Now(),
		}

		err := msgRepo.Upsert(ctx, msg)
		if err != nil {
			t.Fatalf("failed to upsert reply message: %v", err)
		}

		retrieved, err := msgRepo.GetByID(ctx, guild.ID, 3001)
		if err != nil {
			t.Fatalf("failed to get reply message: %v", err)
		}

		if retrieved.ReplyToMessageID == nil || *retrieved.ReplyToMessageID != replyToMsgID {
			t.Errorf("expected reply_to_message_id %d, got %v", replyToMsgID, retrieved.ReplyToMessageID)
		}

		if retrieved.ReplyToChannelID == nil || *retrieved.ReplyToChannelID != replyToChanID {
			t.Errorf("expected reply_to_channel_id %d, got %v", replyToChanID, retrieved.ReplyToChannelID)
		}
	})

	t.Run("MarkDeleted", func(t *testing.T) {
		authorName := "DeleteUser"
		msg := &domain.ChannelMessage{
			GuildID:         guild.ID,
			ChannelID:       400,
			MessageID:       4001,
			UserID:          5001,
			AuthorName:      &authorName,
			Content:         "To be deleted",
			AttachmentCount: 0,
			IsBot:           false,
			TriggerSource:   "observe",
			CreatedAt:       time.Now(),
		}

		err := msgRepo.Upsert(ctx, msg)
		if err != nil {
			t.Fatalf("failed to upsert message: %v", err)
		}

		err = msgRepo.MarkDeleted(ctx, guild.ID, 4001)
		if err != nil {
			t.Fatalf("failed to mark message as deleted: %v", err)
		}

		retrieved, err := msgRepo.GetByID(ctx, guild.ID, 4001)
		if err != nil {
			t.Fatalf("failed to get message: %v", err)
		}

		if retrieved.DeletedAt == nil {
			t.Error("expected deleted_at to be set")
		}
	})

	t.Run("UpdateContent", func(t *testing.T) {
		authorName := "EditUser"
		msg := &domain.ChannelMessage{
			GuildID:         guild.ID,
			ChannelID:       500,
			MessageID:       5001,
			UserID:          6001,
			AuthorName:      &authorName,
			Content:         "Original content",
			AttachmentCount: 0,
			IsBot:           false,
			TriggerSource:   "observe",
			CreatedAt:       time.Now(),
		}

		err := msgRepo.Upsert(ctx, msg)
		if err != nil {
			t.Fatalf("failed to upsert message: %v", err)
		}

		newContent := "Updated content"
		editedAt := time.Now()
		err = msgRepo.UpdateContent(ctx, guild.ID, 5001, newContent, editedAt)
		if err != nil {
			t.Fatalf("failed to update message content: %v", err)
		}

		retrieved, err := msgRepo.GetByID(ctx, guild.ID, 5001)
		if err != nil {
			t.Fatalf("failed to get message: %v", err)
		}

		if retrieved.Content != newContent {
			t.Errorf("expected content '%s', got '%s'", newContent, retrieved.Content)
		}

		if retrieved.EditedAt == nil {
			t.Error("expected edited_at to be set")
		}
	})

	t.Run("ListByUserAcrossChannels", func(t *testing.T) {
		userID := int64(7001)

		for ch := 1; ch <= 3; ch++ {
			for i := 1; i <= 3; i++ {
				authorName := "CrossChannelUser"
				msg := &domain.ChannelMessage{
					GuildID:         guild.ID,
					ChannelID:       int64(600 + ch),
					MessageID:       int64(6000 + ch*100 + i),
					UserID:          userID,
					AuthorName:      &authorName,
					Content:         fmt.Sprintf("Channel %d Message %d", ch, i),
					AttachmentCount: 0,
					IsBot:           false,
					TriggerSource:   "observe",
					CreatedAt:       time.Now().Add(time.Duration(i) * time.Second),
				}
				msgRepo.Upsert(ctx, msg)
			}
		}

		// Query messages from user across channels
		messages, err := msgRepo.ListByUserAcrossChannels(ctx, guild.ID, userID, 60, 100)
		if err != nil {
			t.Fatalf("failed to list user messages: %v", err)
		}

		if len(messages) < 9 {
			t.Errorf("expected at least 9 messages, got %d", len(messages))
		}
	})

	t.Run("PruneOldest", func(t *testing.T) {
		for i := 1; i <= 15; i++ {
			authorName := "PruneUser"
			msg := &domain.ChannelMessage{
				GuildID:         guild.ID,
				ChannelID:       700,
				MessageID:       int64(7000 + i),
				UserID:          8001,
				AuthorName:      &authorName,
				Content:         fmt.Sprintf("Prune message %d", i),
				AttachmentCount: 0,
				IsBot:           false,
				TriggerSource:   "observe",
				CreatedAt:       time.Now().Add(time.Duration(i) * time.Second),
			}
			msgRepo.Upsert(ctx, msg)
		}

		// Prune to keep only 5 newest
		err := msgRepo.PruneOldest(ctx, guild.ID, 700, 5)
		if err != nil {
			t.Fatalf("failed to prune messages: %v", err)
		}

		// Verify only 5 remain
		recent, err := msgRepo.ListRecent(ctx, guild.ID, 700, 100)
		if err != nil {
			t.Fatalf("failed to list recent messages: %v", err)
		}

		if len(recent) != 5 {
			t.Errorf("expected exactly 5 messages after pruning, got %d", len(recent))
		}
	})
}
