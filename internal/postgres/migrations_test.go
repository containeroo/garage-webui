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
	files := fstest.MapFS{
		"migrations/002_second.sql": {Data: []byte("SELECT 2")},
		"migrations/001_first.sql":  {Data: []byte("SELECT 1")},
	}
	migrations, err := loadMigrations(files)
	require.NoError(t, err)
	require.Len(t, migrations, 2)
	assert.Equal(t, int64(1), migrations[0].version)
	assert.Equal(t, int64(2), migrations[1].version)

	for name, files := range map[string]fstest.MapFS{
		"invalid name": {"migrations/bad.sql": {}},
		"zero version": {"migrations/000_first.sql": {}},
		"duplicate": {
			"migrations/001_first.sql": {},
			"migrations/1_second.sql":  {},
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := loadMigrations(files)
			require.Error(t, err)
		})
	}
}

// TestMigrationsIntegration verifies locking, rollback and repeat startup.
func TestMigrationsIntegration(t *testing.T) {
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

	_, err = pool.Exec(ctx, `
  CREATE TABLE webui_users (
    username text PRIMARY KEY,
    password_hash text NOT NULL
  );

  INSERT INTO webui_users (username, password_hash)
  VALUES ('existing', 'existing-hash');
`)
	require.NoError(t, err)

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
	var count int
	require.NoError(t, pool.QueryRow(ctx, "SELECT count(*) FROM webui_schema_migrations").Scan(&count))
	assert.Equal(t, 8, count)
	var passwordHash string
	require.NoError(t, pool.QueryRow(ctx, "SELECT password_hash FROM webui_users WHERE username = $1", "existing").Scan(&passwordHash))
	assert.Equal(t, "existing-hash", passwordHash)

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
	assert.Equal(t, 8, count)
}
