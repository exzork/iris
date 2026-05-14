package repository

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var errEmptyDSN = errors.New("empty DSN")

// validateTestDSN checks that a DSN points to a test database, not production.
// Returns errEmptyDSN if dsn is empty (caller should skip).
// Returns error if dsn appears to point at live database.
// livePOSTGRESDB is the production database name (e.g., "iris").
func validateTestDSN(dsn, livePOSTGRESDB string) error {
	if dsn == "" {
		return errEmptyDSN
	}

	// Parse the DSN to extract host and database name.
	u, err := url.Parse(dsn)
	if err != nil {
		return fmt.Errorf("invalid DSN: %w", err)
	}

	// Extract database name from path (last component, strip leading /).
	dbName := strings.TrimPrefix(u.Path, "/")
	if dbName == "" {
		return errors.New("DSN has no database name")
	}

	// Safelist: explicitly allowed test database names.
	safelist := map[string]bool{
		"iris_test":      true,
		"iris_repo_test": true,
	}

	// Check safelist first.
	if safelist[dbName] {
		return nil
	}

	// Check if dbName contains "test" or "_test" substring.
	if strings.Contains(dbName, "test") || strings.Contains(dbName, "_test") {
		return nil
	}

	// If we reach here, dbName doesn't look like a test database.
	// Reject if it matches the live database name.
	if dbName == livePOSTGRESDB {
		return fmt.Errorf("DSN points to live database: %s", dbName)
	}

	// Reject any other non-test database name as a precaution.
	return fmt.Errorf("DSN database name does not contain 'test': %s", dbName)
}

// redactDSN replaces the password in a DSN with "REDACTED" for safe logging.
func redactDSN(dsn string) string {
	return strings.ReplaceAll(dsn, strings.Split(dsn, "@")[0], strings.Split(strings.Split(dsn, "@")[0], ":")[0]+":REDACTED")
}

func setupTestDB(t *testing.T) *DB {
	// HARD GUARDRAIL: Prevent tests from running against the live database.
	// This protects against accidental TRUNCATE CASCADE operations wiping production.
	testDBURL := os.Getenv("TEST_DATABASE_URL")
	if testDBURL == "" {
		// Preserve existing behavior: skip tests when TEST_DATABASE_URL is not set.
		t.Skip("TEST_DATABASE_URL not set; skipping integration tests")
	}

	// Validate that the test DSN points to a test database, not production.
	liveDB := os.Getenv("POSTGRES_DB")
	if liveDB == "" {
		liveDB = "iris" // Default production database name.
	}

	if err := validateTestDSN(testDBURL, liveDB); err != nil {
		if errors.Is(err, errEmptyDSN) {
			t.Skip("TEST_DATABASE_URL not set; skipping integration tests")
		}
		t.Fatalf("refusing to run tests against suspected live database: dsn=%s, reason=%v", redactDSN(testDBURL), err)
	}

	// Extract and log the chosen test database for visibility.
	u, _ := url.Parse(testDBURL)
	dbName := strings.TrimPrefix(u.Path, "/")
	host := u.Host
	t.Logf("using test DB: %s/%s", host, dbName)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, testDBURL)
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("failed to ping test database: %v", err)
	}

	cleanupTestDB(t, pool)

	return NewDB(pool)
}

func cleanupTestDB(t *testing.T, pool *pgxpool.Pool) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tables := []string{
		"audit_events",
		"reminders",
		"tool_logs",
		"lore_chunks",
		"lore_documents",
		"memory_records",
		"channel_messages",
		"channel_conversations",
		"allowed_channels",
		"exception_channels",
		"guild_settings",
		"guilds",
	}

	for _, table := range tables {
		_, err := pool.Exec(ctx, fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table))
		if err != nil {
			t.Logf("warning: failed to truncate %s: %v", table, err)
		}
	}
}

func closeTestDB(t *testing.T, db *DB) {
	db.pool.Close()
}
