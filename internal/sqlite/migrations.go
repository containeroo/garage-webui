package sqlite

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"strconv"
	"strings"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// migrate atomically initializes or upgrades the schema before requests are served.
func (s *Store) migrate(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS webui_schema_migrations (version INTEGER PRIMARY KEY, name TEXT NOT NULL)`); err != nil {
		return err
	}
	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return err
	}
	applied := []string{}
	for _, entry := range entries {
		version, err := strconv.Atoi(strings.SplitN(entry.Name(), "_", 2)[0])
		if err != nil || version <= 0 {
			return fmt.Errorf("invalid SQLite migration %q", entry.Name())
		}
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM webui_schema_migrations WHERE version=?)`, version).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		source, err := migrationFiles.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(source)); err != nil {
			return fmt.Errorf("apply %s: %w", entry.Name(), err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO webui_schema_migrations(version,name) VALUES(?,?)`, version, entry.Name()); err != nil {
			return err
		}
		applied = append(applied, entry.Name())
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	for _, name := range applied {
		s.logger.Info("applied SQLite migration", "migration", name)
	}
	return nil
}
