package postgres

import (
	"cmp"
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"slices"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

// migrationFiles contains the database schema and subsequent upgrades.
//
//go:embed migrations/*.sql
var migrationFiles embed.FS

// migration describes one schema change.
type migration struct {
	// version orders and identifies the migration.
	version int64
	// name identifies the source file.
	name string
	// sql contains the migration statements.
	sql string
}

// loadMigrations validates and orders the embedded migration files.
func loadMigrations(source fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(source, "migrations")
	if err != nil {
		return nil, fmt.Errorf("read migrations: %w", err)
	}

	migrations := make([]migration, 0, len(entries))
	for _, entry := range entries {
		if !isMigrationFile(entry) {
			continue
		}

		version, err := migrationVersion(entry.Name())
		if err != nil {
			return nil, fmt.Errorf("invalid migration %q: %w", entry.Name(), err)
		}

		data, err := fs.ReadFile(source, "migrations/"+entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read migration %q: %w", entry.Name(), err)
		}

		migrations = append(migrations, migration{
			version: version,
			name:    entry.Name(),
			sql:     string(data),
		})
	}

	if len(migrations) == 0 {
		return nil, errors.New("no database migrations found")
	}

	slices.SortFunc(migrations, func(a, b migration) int {
		return cmp.Compare(a.version, b.version)
	})

	for index := 1; index < len(migrations); index++ {
		if migrations[index-1].version == migrations[index].version {
			return nil, fmt.Errorf("duplicate migration version %d", migrations[index].version)
		}
	}

	return migrations, nil
}

// isMigrationFile selects regular SQL files from the migrations directory.
func isMigrationFile(entry fs.DirEntry) bool {
	return entry.Type().IsRegular() && strings.HasSuffix(entry.Name(), ".sql")
}

// migrationVersion extracts a positive numeric prefix from a SQL migration filename.
func migrationVersion(name string) (int64, error) {
	prefix, suffix, ok := strings.Cut(name, "_")
	if !ok || suffix == ".sql" || !strings.HasSuffix(suffix, ".sql") {
		return 0, errors.New("expected version_description.sql")
	}

	version, err := strconv.ParseInt(prefix, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse version: %w", err)
	}

	if version <= 0 {
		return 0, errors.New("migration version must be positive")
	}

	return version, nil
}

// Migrate applies pending schema changes atomically.
func (s *Store) Migrate(ctx context.Context, logger *slog.Logger) error {
	if logger == nil {
		return errors.New("database logger is required")
	}

	return s.migrate(ctx, logger, migrationFiles)
}

// migrate serializes schema changes across application instances.
func (s *Store) migrate(ctx context.Context, logger *slog.Logger, source fs.FS) error {
	migrations, err := loadMigrations(source)
	if err != nil {
		return err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin migration: %w", err)
	}

	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, `
  SELECT
    pg_advisory_xact_lock(748403197614)
`); err != nil {
		return fmt.Errorf("lock migrations: %w", err)
	}

	if _, err := tx.Exec(ctx, `
  CREATE TABLE IF NOT EXISTS webui_schema_migrations (
    version bigint PRIMARY KEY,
    name text NOT NULL,
    applied_at timestamptz NOT NULL DEFAULT now()
  )
`); err != nil {
		return fmt.Errorf("create migration table: %w", err)
	}

	applied, err := applyMigrations(ctx, tx, migrations)
	if err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit migrations: %w", err)
	}

	for _, name := range applied {
		logger.Info("applied database migration", "migration", name)
	}

	return nil
}

// applyMigrations runs and records changes within the migration transaction.
func applyMigrations(ctx context.Context, tx pgx.Tx, migrations []migration) ([]string, error) {
	var applied []string
	for _, migration := range migrations {
		var exists bool
		err := tx.QueryRow(ctx, `
  SELECT EXISTS (
    SELECT 1
    FROM webui_schema_migrations
    WHERE version = $1
  )
`, migration.version).Scan(&exists)
		if err != nil {
			return nil, fmt.Errorf("check migration %q: %w", migration.name, err)
		}

		if exists {
			continue
		}

		if _, err := tx.Exec(ctx, migration.sql); err != nil {
			return nil, fmt.Errorf("apply migration %q: %w", migration.name, err)
		}

		if _, err := tx.Exec(ctx, `
  INSERT INTO webui_schema_migrations (version, name)
  VALUES ($1, $2)
`, migration.version, migration.name); err != nil {
			return nil, fmt.Errorf("record migration %q: %w", migration.name, err)
		}

		applied = append(applied, migration.name)
	}

	return applied, nil
}
