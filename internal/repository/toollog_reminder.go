package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type ToolLogRepo struct {
	db *DB
}

func NewToolLogRepo(db *DB) *ToolLogRepo {
	return &ToolLogRepo{db: db}
}

func (r *ToolLogRepo) Save(ctx context.Context, guildID int64, userID int64, toolName string, inputData, outputData map[string]interface{}, status, errorMsg string) error {
	inputJSON, _ := json.Marshal(inputData)
	outputJSON, _ := json.Marshal(outputData)

	sql := `
		INSERT INTO tool_logs (guild_id, user_id, tool_name, input_data, output_data, status, error_message, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	_, err := r.db.Exec(ctx, sql, guildID, userID, toolName, inputJSON, outputJSON, status, errorMsg, time.Now())
	if err != nil {
		return fmt.Errorf("failed to save tool log: %w", err)
	}
	return nil
}

func (r *ToolLogRepo) GetByGuild(ctx context.Context, guildID int64, limit int) ([]map[string]interface{}, error) {
	sql := `
		SELECT id, guild_id, user_id, tool_name, input_data, output_data, status, error_message, created_at
		FROM tool_logs
		WHERE guild_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`
	rows, err := r.db.Query(ctx, sql, guildID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query tool logs: %w", err)
	}
	defer rows.Close()

	var logs []map[string]interface{}
	for rows.Next() {
		var id int64
		var gid int64
		var uid int64
		var toolName string
		var inputJSON, outputJSON []byte
		var status, errorMsg string
		var createdAt time.Time

		err := rows.Scan(&id, &gid, &uid, &toolName, &inputJSON, &outputJSON, &status, &errorMsg, &createdAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan tool log: %w", err)
		}

		log := map[string]interface{}{
			"id":            id,
			"guild_id":      gid,
			"user_id":       uid,
			"tool_name":     toolName,
			"status":        status,
			"error_message": errorMsg,
			"created_at":    createdAt,
		}

		var inputData map[string]interface{}
		json.Unmarshal(inputJSON, &inputData)
		log["input_data"] = inputData

		var outputData map[string]interface{}
		json.Unmarshal(outputJSON, &outputData)
		log["output_data"] = outputData

		logs = append(logs, log)
	}
	return logs, nil
}

type ReminderRepo struct {
	db *DB
}

func NewReminderRepo(db *DB) *ReminderRepo {
	return &ReminderRepo{db: db}
}

func (r *ReminderRepo) Save(ctx context.Context, guildID int64, userID int64, channelID int64, reminderText string, scheduledFor time.Time) error {
	sql := `
		INSERT INTO reminders (guild_id, user_id, channel_id, reminder_text, scheduled_for, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	_, err := r.db.Exec(ctx, sql, guildID, userID, channelID, reminderText, scheduledFor, time.Now())
	if err != nil {
		return fmt.Errorf("failed to save reminder: %w", err)
	}
	return nil
}

func (r *ReminderRepo) GetByGuild(ctx context.Context, guildID int64, limit int) ([]map[string]interface{}, error) {
	sql := `
		SELECT id, guild_id, user_id, channel_id, reminder_text, scheduled_for, created_at
		FROM reminders
		WHERE guild_id = $1
		ORDER BY scheduled_for ASC
		LIMIT $2
	`
	rows, err := r.db.Query(ctx, sql, guildID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query reminders: %w", err)
	}
	defer rows.Close()

	var reminders []map[string]interface{}
	for rows.Next() {
		var id int64
		var gid int64
		var uid int64
		var cid int64
		var reminderText string
		var scheduledFor time.Time
		var createdAt time.Time

		err := rows.Scan(&id, &gid, &uid, &cid, &reminderText, &scheduledFor, &createdAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan reminder: %w", err)
		}

		reminder := map[string]interface{}{
			"id":             id,
			"guild_id":       gid,
			"user_id":        uid,
			"channel_id":     cid,
			"reminder_text":  reminderText,
			"scheduled_for":  scheduledFor,
			"created_at":     createdAt,
		}
		reminders = append(reminders, reminder)
	}
	return reminders, nil
}

func (r *ReminderRepo) GetDue(ctx context.Context, guildID int64) ([]map[string]interface{}, error) {
	sql := `
		SELECT id, guild_id, user_id, channel_id, reminder_text, scheduled_for, created_at
		FROM reminders
		WHERE guild_id = $1 AND scheduled_for <= NOW()
		ORDER BY scheduled_for ASC
	`
	rows, err := r.db.Query(ctx, sql, guildID)
	if err != nil {
		return nil, fmt.Errorf("failed to query due reminders: %w", err)
	}
	defer rows.Close()

	var reminders []map[string]interface{}
	for rows.Next() {
		var id int64
		var gid int64
		var uid int64
		var cid int64
		var reminderText string
		var scheduledFor time.Time
		var createdAt time.Time

		err := rows.Scan(&id, &gid, &uid, &cid, &reminderText, &scheduledFor, &createdAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan reminder: %w", err)
		}

		reminder := map[string]interface{}{
			"id":             id,
			"guild_id":       gid,
			"user_id":        uid,
			"channel_id":     cid,
			"reminder_text":  reminderText,
			"scheduled_for":  scheduledFor,
			"created_at":     createdAt,
		}
		reminders = append(reminders, reminder)
	}
	return reminders, nil
}

func (r *ReminderRepo) Delete(ctx context.Context, guildID int64, reminderID int64) error {
	sql := `DELETE FROM reminders WHERE guild_id = $1 AND id = $2`
	_, err := r.db.Exec(ctx, sql, guildID, reminderID)
	if err != nil {
		return fmt.Errorf("failed to delete reminder: %w", err)
	}
	return nil
}
