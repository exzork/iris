package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/eko/iris-bot/internal/domain"
	"github.com/jackc/pgx/v5"
)

type UserBehaviorRepo struct {
	db *DB
}

func NewUserBehaviorRepo(db *DB) *UserBehaviorRepo {
	return &UserBehaviorRepo{db: db}
}

// GetByGuildUser returns the profile for (guildID, userID) or nil if none
// exists. guildID and userID must both be non-zero; otherwise
// ErrMissingGuildID is returned to guarantee isolation.
func (r *UserBehaviorRepo) GetByGuildUser(ctx context.Context, guildID int64, userID int64) (*domain.UserBehaviorProfile, error) {
	if guildID == 0 {
		return nil, ErrMissingGuildID
	}
	if userID == 0 {
		return nil, errors.New("repository: user_id is required")
	}
	const sql = `
		SELECT id, guild_id, user_id, communication_style, formality,
		       response_length_preference, formatting_preference, recurring_topics,
		       evidence_count, last_observed_at, created_at, updated_at
		FROM user_behavior_profiles
		WHERE guild_id = $1 AND user_id = $2
	`
	p := &domain.UserBehaviorProfile{}
	err := r.db.QueryRow(ctx, sql, guildID, userID).Scan(
		&p.ID, &p.GuildID, &p.UserID, &p.CommunicationStyle, &p.Formality,
		&p.ResponseLengthPreference, &p.FormattingPreference, &p.RecurringTopics,
		&p.EvidenceCount, &p.LastObservedAt, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query user behavior profile: %w", err)
	}
	return p, nil
}

// Upsert stores or refreshes a profile identified by (guild_id, user_id).
// Returns ErrMissingGuildID if guildID is zero.
func (r *UserBehaviorRepo) Upsert(ctx context.Context, p *domain.UserBehaviorProfile) error {
	if p == nil {
		return errors.New("repository: nil profile")
	}
	if p.GuildID == 0 {
		return ErrMissingGuildID
	}
	if p.UserID == 0 {
		return errors.New("repository: user_id is required")
	}
	if p.RecurringTopics == nil {
		p.RecurringTopics = []string{}
	}
	now := time.Now()
	if p.LastObservedAt.IsZero() {
		p.LastObservedAt = now
	}
	const sql = `
		INSERT INTO user_behavior_profiles (
			guild_id, user_id, communication_style, formality,
			response_length_preference, formatting_preference, recurring_topics,
			evidence_count, last_observed_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (guild_id, user_id) DO UPDATE SET
			communication_style = EXCLUDED.communication_style,
			formality = EXCLUDED.formality,
			response_length_preference = EXCLUDED.response_length_preference,
			formatting_preference = EXCLUDED.formatting_preference,
			recurring_topics = EXCLUDED.recurring_topics,
			evidence_count = EXCLUDED.evidence_count,
			last_observed_at = EXCLUDED.last_observed_at,
			updated_at = EXCLUDED.updated_at
	`
	_, err := r.db.Exec(ctx, sql,
		p.GuildID, p.UserID, p.CommunicationStyle, p.Formality,
		p.ResponseLengthPreference, p.FormattingPreference, p.RecurringTopics,
		p.EvidenceCount, p.LastObservedAt, now, now,
	)
	if err != nil {
		return fmt.Errorf("failed to upsert user behavior profile: %w", err)
	}
	return nil
}

func isNoRows(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, pgx.ErrNoRows) || err.Error() == "no rows in result set"
}
