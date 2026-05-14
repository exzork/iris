package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/eko/iris-bot/internal/domain"
)

// ExceptionChannelQuerier defines the contract for exception channel queries.
type ExceptionChannelQuerier interface {
	IsException(ctx context.Context, guildID int64, channelID int64) (bool, error)
}

// AllowedChannelQuerier defines the contract for allowed channel queries.
type AllowedChannelQuerier interface {
	Add(ctx context.Context, guildID int64, channelID int64) error
	Remove(ctx context.Context, guildID int64, channelID int64) error
	IsAllowed(ctx context.Context, guildID int64, channelID int64) (bool, error)
	HasAny(ctx context.Context, guildID int64) (bool, error)
	ListByGuild(ctx context.Context, guildID int64) ([]int64, error)
}

// ChannelMessageQuerier defines the contract for channel message queries.
type ChannelMessageQuerier interface {
	Upsert(ctx context.Context, msg *domain.ChannelMessage) error
	PruneOldest(ctx context.Context, guildID int64, channelID int64, keep int) error
	ListRecent(ctx context.Context, guildID int64, channelID int64, limit int) ([]*domain.ChannelMessage, error)
	GetByID(ctx context.Context, guildID int64, messageID int64) (*domain.ChannelMessage, error)
	ListByUserAcrossChannels(ctx context.Context, guildID int64, userID int64, sinceMinutes int, limit int) ([]*domain.ChannelMessage, error)
	MarkDeleted(ctx context.Context, guildID int64, messageID int64) error
	UpdateContent(ctx context.Context, guildID int64, messageID int64, newContent string, editedAt time.Time) error
}

// ChannelConversationQuerier defines the contract for channel conversation queries.
type ChannelConversationQuerier interface {
	Refresh(ctx context.Context, guildID int64, channelID int64, now time.Time, ttl time.Duration) error
	Active(ctx context.Context, guildID int64, channelID int64, now time.Time) (bool, error)
	Clear(ctx context.Context, guildID int64, channelID int64) error
}

// DB wraps pgxpool.Pool for dependency injection.
type DB struct {
	pool *pgxpool.Pool
}

// NewDB creates a new DB wrapper.
func NewDB(pool *pgxpool.Pool) *DB {
	return &DB{pool: pool}
}

// Tx represents a database transaction context.
type Tx struct {
	tx pgx.Tx
}

// Begin starts a new transaction.
func (db *DB) Begin(ctx context.Context) (*Tx, error) {
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	return &Tx{tx: tx}, nil
}

// Commit commits the transaction.
func (t *Tx) Commit(ctx context.Context) error {
	return t.tx.Commit(ctx)
}

// Rollback rolls back the transaction.
func (t *Tx) Rollback(ctx context.Context) error {
	return t.tx.Rollback(ctx)
}

// QueryRow executes a query that returns a single row.
func (db *DB) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	return db.pool.QueryRow(ctx, sql, args...)
}

// Query executes a query that returns multiple rows.
func (db *DB) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	return db.pool.Query(ctx, sql, args...)
}

// Exec executes a command.
func (db *DB) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	return db.pool.Exec(ctx, sql, args...)
}

// QueryRowTx executes a query on a transaction that returns a single row.
func (t *Tx) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	return t.tx.QueryRow(ctx, sql, args...)
}

// QueryTx executes a query on a transaction that returns multiple rows.
func (t *Tx) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	return t.tx.Query(ctx, sql, args...)
}

// ExecTx executes a command on a transaction.
func (t *Tx) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	return t.tx.Exec(ctx, sql, args...)
}
