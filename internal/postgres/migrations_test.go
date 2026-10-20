package postgres

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoadMigrations verifies ordering and filename validation.
func TestLoadMigrations(t *testing.T) {
	t.Parallel()

	files := fstest.MapFS{
		"migrations/002_second.sql": {Data: []byte("SELECT 2")},
		"migrations/001_first.sql":  {Data: []byte("SELECT 1")},
	}
	migrations, err := loadMigrations(files)
	require.NoError(t, err)
	require.Len(t, migrations, 2)
	assert.Equal(t, int64(1), migrations[0].version)
	assert.Equal(t, int64(2), migrations[1].version)

	t.Run("invalid name", func(t *testing.T) {
		files := fstest.MapFS{"migrations/bad.sql": {}}
		_, err := loadMigrations(files)
		require.Error(t, err)
	})

	t.Run("zero version", func(t *testing.T) {
		files := fstest.MapFS{"migrations/000_first.sql": {}}
		_, err := loadMigrations(files)
		require.Error(t, err)
	})

	t.Run("missing description", func(t *testing.T) {
		files := fstest.MapFS{"migrations/001_.sql": {}}
		_, err := loadMigrations(files)
		require.Error(t, err)
	})

	t.Run("negative version", func(t *testing.T) {
		files := fstest.MapFS{"migrations/-2_bad.sql": {}}
		_, err := loadMigrations(files)
		require.Error(t, err)
	})

	t.Run("duplicate", func(t *testing.T) {
		files := fstest.MapFS{"migrations/001_first.sql": {}, "migrations/1_second.sql": {}}
		_, err := loadMigrations(files)
		require.Error(t, err)
	})
}

// TestMigrationVersion validates the simple numeric-prefix filename convention.
func TestMigrationVersion(t *testing.T) {
	t.Parallel()

	t.Run("001_initial.sql", func(t *testing.T) {
		t.Parallel()

		version, err := migrationVersion("001_initial.sql")

		require.NoError(t, err)
		assert.Equal(t, int64(1), version)
	})

	t.Run("12_add_indexes.sql", func(t *testing.T) {
		t.Parallel()

		version, err := migrationVersion("12_add_indexes.sql")

		require.NoError(t, err)
		assert.Equal(t, int64(12), version)
	})

	t.Run("bad.sql", func(t *testing.T) {
		t.Parallel()

		_, err := migrationVersion("bad.sql")

		require.Error(t, err)
	})

	t.Run("3_.sql", func(t *testing.T) {
		t.Parallel()

		_, err := migrationVersion("3_.sql")

		require.Error(t, err)
	})

	t.Run("0_baseline.sql", func(t *testing.T) {
		t.Parallel()

		_, err := migrationVersion("0_baseline.sql")

		require.Error(t, err)
	})

	t.Run("-1_removed.sql", func(t *testing.T) {
		t.Parallel()

		_, err := migrationVersion("-1_removed.sql")

		require.Error(t, err)
	})
}

// TestMigrationsIntegration verifies locking, rollback and repeat startup.
func TestMigrationsIntegration(t *testing.T) {
	t.Parallel()

	dsn := os.Getenv("GARAGE_WEBUI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set GARAGE_WEBUI_TEST_DATABASE_URL to run migration integration tests")
	}

	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(admin.Close)
	schema := pgx.Identifier{fmt.Sprintf("webui_migration_test_%d", time.Now().UnixNano())}.Sanitize()
	_, err = admin.Exec(ctx, "CREATE SCHEMA "+schema)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE") })
	config, err := pgxpool.ParseConfig(dsn)
	require.NoError(t, err)
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	store := New(pool)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- store.Migrate(ctx, logger)
		}()
	}

	wg.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}

	require.NoError(t, store.Migrate(ctx, logger))
	migrations, err := loadMigrations(migrationFiles)
	require.NoError(t, err)
	var count int
	require.NoError(t, pool.QueryRow(ctx, "SELECT count(*) FROM webui_schema_migrations").Scan(&count))
	assert.Equal(t, len(migrations), count)
	appliedBeforeFailure := count

	files := fstest.MapFS{
		"migrations/001_initial.sql": {Data: []byte("SELECT 1")},
		"migrations/008_valid.sql":   {Data: []byte("CREATE TABLE migration_rollback_test (id integer)")},
		"migrations/009_invalid.sql": {Data: []byte("THIS IS INVALID SQL")},
	}
	require.Error(t, store.migrate(ctx, logger, files))
	var tableExists bool
	require.NoError(t, pool.QueryRow(ctx, "SELECT to_regclass('migration_rollback_test') IS NOT NULL").Scan(&tableExists))
	assert.False(t, tableExists)
	require.NoError(t, pool.QueryRow(ctx, "SELECT count(*) FROM webui_schema_migrations").Scan(&count))
	assert.Equal(t, appliedBeforeFailure, count)
}
