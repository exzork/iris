package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type GlobalSettingsRepo struct {
	db *DB
}

func NewGlobalSettingsRepo(db *DB) *GlobalSettingsRepo {
	return &GlobalSettingsRepo{db: db}
}

func (r *GlobalSettingsRepo) Get(ctx context.Context, key string) (value string, found bool, err error) {
	const sql = `SELECT setting_value FROM global_settings WHERE setting_key = $1`
	row := r.db.QueryRow(ctx, sql, key)
	if scanErr := row.Scan(&value); scanErr != nil {
		if errors.Is(scanErr, pgx.ErrNoRows) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("failed to get global setting %q: %w", key, scanErr)
	}
	return value, true, nil
}

func (r *GlobalSettingsRepo) Set(ctx context.Context, key, value string, updatedBy int64) error {
	const sql = `
		INSERT INTO global_settings (setting_key, setting_value, updated_by, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $4)
		ON CONFLICT (setting_key) DO UPDATE
		SET setting_value = EXCLUDED.setting_value,
		    updated_by    = EXCLUDED.updated_by,
		    updated_at    = EXCLUDED.updated_at
	`
	if _, err := r.db.Exec(ctx, sql, key, value, updatedBy, time.Now()); err != nil {
		return fmt.Errorf("failed to set global setting %q: %w", key, err)
	}
	return nil
}

func (r *GlobalSettingsRepo) Delete(ctx context.Context, key string) error {
	const sql = `DELETE FROM global_settings WHERE setting_key = $1`
	if _, err := r.db.Exec(ctx, sql, key); err != nil {
		return fmt.Errorf("failed to delete global setting %q: %w", key, err)
	}
	return nil
}

func (r *GlobalSettingsRepo) List(ctx context.Context) (map[string]string, error) {
	const sql = `SELECT setting_key, setting_value FROM global_settings`
	rows, err := r.db.Query(ctx, sql)
	if err != nil {
		return nil, fmt.Errorf("failed to query global settings: %w", err)
	}
	defer rows.Close()

	out := make(map[string]string)
	for rows.Next() {
		var k, v string
		if scanErr := rows.Scan(&k, &v); scanErr != nil {
			return nil, fmt.Errorf("failed to scan global setting: %w", scanErr)
		}
		out[k] = v
	}
	return out, nil
}
