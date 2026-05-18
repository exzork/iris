package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"

	"github.com/jackc/pgx/v5"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: migrate <up|status>")
		os.Exit(1)
	}

	command := os.Args[1]

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL not set")
	}

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer conn.Close(ctx)

	switch command {
	case "up":
		if err := migrateUp(ctx, conn); err != nil {
			log.Fatalf("Migration up failed: %v", err)
		}
		fmt.Println("Migrations applied successfully")
	case "status":
		if err := migrateStatus(ctx, conn); err != nil {
			log.Fatalf("Migration status failed: %v", err)
		}
	default:
		fmt.Printf("Unknown command: %s\n", command)
		os.Exit(1)
	}
}

func ensureMigrationsTable(ctx context.Context, conn *pgx.Conn) error {
	const ddl = `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			filename   TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`
	if _, err := conn.Exec(ctx, ddl); err != nil {
		return fmt.Errorf("failed to create schema_migrations: %w", err)
	}
	return nil
}

func appliedMigrations(ctx context.Context, conn *pgx.Conn) (map[string]struct{}, error) {
	rows, err := conn.Query(ctx, "SELECT filename FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("failed to query schema_migrations: %w", err)
	}
	defer rows.Close()
	out := make(map[string]struct{})
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out[name] = struct{}{}
	}
	return out, nil
}

func stampLegacyMigrations(ctx context.Context, conn *pgx.Conn, files []string) error {
	var hasGuilds bool
	if err := conn.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name='guilds')").Scan(&hasGuilds); err != nil {
		return fmt.Errorf("failed to detect legacy schema: %w", err)
	}
	if !hasGuilds {
		return nil
	}
	for _, f := range files {
		if _, err := conn.Exec(ctx,
			"INSERT INTO schema_migrations (filename) VALUES ($1) ON CONFLICT DO NOTHING", f,
		); err != nil {
			return fmt.Errorf("failed to stamp legacy migration %s: %w", f, err)
		}
	}
	return nil
}

func migrateUp(ctx context.Context, conn *pgx.Conn) error {
	if err := ensureMigrationsTable(ctx, conn); err != nil {
		return err
	}

	migrationsDir := os.Getenv("MIGRATIONS_DIR")
	if migrationsDir == "" {
		migrationsDir = "migrations"
	}
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("failed to read migrations directory: %w", err)
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		files = append(files, entry.Name())
	}
	sort.Strings(files)

	applied, err := appliedMigrations(ctx, conn)
	if err != nil {
		return err
	}

	if len(applied) == 0 {
		if err := stampLegacyMigrations(ctx, conn, files); err != nil {
			return err
		}
		applied, err = appliedMigrations(ctx, conn)
		if err != nil {
			return err
		}
	}

	for _, name := range files {
		if _, done := applied[name]; done {
			fmt.Printf("Skipping migration (already applied): %s\n", name)
			continue
		}

		content, err := os.ReadFile(filepath.Join(migrationsDir, name))
		if err != nil {
			return fmt.Errorf("failed to read migration file %s: %w", name, err)
		}

		fmt.Printf("Applying migration: %s\n", name)

		tx, err := conn.Begin(ctx)
		if err != nil {
			return fmt.Errorf("failed to begin tx for %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx, string(content)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("failed to execute migration %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx,
			"INSERT INTO schema_migrations (filename) VALUES ($1)", name,
		); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("failed to record migration %s: %w", name, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("failed to commit migration %s: %w", name, err)
		}
	}

	return nil
}

func migrateStatus(ctx context.Context, conn *pgx.Conn) error {
	if err := ensureMigrationsTable(ctx, conn); err != nil {
		return err
	}
	rows, err := conn.Query(ctx, "SELECT filename, applied_at FROM schema_migrations ORDER BY filename")
	if err != nil {
		return fmt.Errorf("failed to query schema_migrations: %w", err)
	}
	defer rows.Close()

	any := false
	for rows.Next() {
		var name, applied string
		if err := rows.Scan(&name, &applied); err != nil {
			return err
		}
		fmt.Printf("OK %s (applied %s)\n", name, applied)
		any = true
	}
	if !any {
		fmt.Println("No migrations recorded.")
	}
	return nil
}
