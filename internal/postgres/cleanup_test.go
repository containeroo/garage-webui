package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestCleanupExpiredAuth(t *testing.T) {
	dsn := os.Getenv("GARAGE_WEBUI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set GARAGE_WEBUI_TEST_DATABASE_URL for PostgreSQL integration tests")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(admin.Close)
	schema := pgx.Identifier{fmt.Sprintf("webui_cleanup_test_%d", time.Now().UnixNano())}.Sanitize()
	_, err = admin.Exec(ctx, "CREATE SCHEMA "+schema)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE") })
	config, err := pgxpool.ParseConfig(dsn)
	require.NoError(t, err)
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	require.NoError(t, err)
	defer pool.Close()
	_, err = pool.Exec(ctx, `
		CREATE TABLE webui_sessions (expires_at timestamptz NOT NULL);
		CREATE TABLE webui_oidc_states (expires_at timestamptz NOT NULL);
		INSERT INTO webui_sessions VALUES (now() - interval '1 hour'), (now() + interval '1 hour');
		INSERT INTO webui_oidc_states VALUES (now() - interval '1 hour'), (now() + interval '1 hour');
	`)
	require.NoError(t, err)
	require.NoError(t, New(pool).CleanupExpiredAuth(ctx))
	for _, table := range []string{"webui_sessions", "webui_oidc_states"} {
		var count int
		require.NoError(t, pool.QueryRow(ctx, "SELECT count(*) FROM "+pgx.Identifier{table}.Sanitize()).Scan(&count))
		require.Equal(t, 1, count, table)
		_, err = pool.Exec(ctx, "INSERT INTO "+pgx.Identifier{table}.Sanitize()+" SELECT now() - interval '1 hour' FROM generate_series(1, $1)", cleanupBatchSize+1)
		require.NoError(t, err)
	}
	require.NoError(t, New(pool).CleanupExpiredAuth(ctx))
	for _, table := range []string{"webui_sessions", "webui_oidc_states"} {
		var count int
		require.NoError(t, pool.QueryRow(ctx, "SELECT count(*) FROM "+pgx.Identifier{table}.Sanitize()).Scan(&count))
		require.Equal(t, 2, count, table, "one expired and one active record should remain")
	}
	require.NoError(t, New(pool).CleanupExpiredAuth(ctx))
	for _, table := range []string{"webui_sessions", "webui_oidc_states"} {
		var count int
		require.NoError(t, pool.QueryRow(ctx, "SELECT count(*) FROM "+pgx.Identifier{table}.Sanitize()).Scan(&count))
		require.Equal(t, 1, count, table)
	}
}
