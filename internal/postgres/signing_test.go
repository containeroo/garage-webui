package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSigningKeyCoordination checks serialization and recovery across connections.
func TestSigningKeyCoordination(t *testing.T) {
	dsn := os.Getenv("GARAGE_WEBUI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set GARAGE_WEBUI_TEST_DATABASE_URL for PostgreSQL integration tests")
	}

	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(admin.Close)
	schema := pgx.Identifier{fmt.Sprintf("webui_signing_%d", time.Now().UnixNano())}.Sanitize()
	_, err = admin.Exec(ctx, "CREATE SCHEMA "+schema)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE") })
	config, err := pgxpool.ParseConfig(dsn)
	require.NoError(t, err)
	config.ConnConfig.RuntimeParams["search_path"] = schema
	config.ConnConfig.RuntimeParams["application_name"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	_, err = pool.Exec(ctx, `CREATE TABLE webui_upload_keys (
		bucket_id text PRIMARY KEY,
		access_key_id text UNIQUE,
		read_access_key_id text UNIQUE,
		copy_access_key_id text UNIQUE
	)`)
	require.NoError(t, err)
	store := New(pool)
	entered := make(chan struct{})
	release := make(chan struct{})
	first := make(chan error, 1)
	go func() {
		first <- store.WithBucketSigningKey(ctx, "bucket", func(_ context.Context, id string) (string, error) {
			close(entered)
			<-release
			return "key", nil
		})
	}()
	<-entered
	var transactions int
	err = admin.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity
		WHERE application_name = $1 AND state = 'idle in transaction'`, schema).Scan(&transactions)
	require.NoError(t, err)
	assert.Zero(t, transactions)

	waitCtx, cancelWait := context.WithTimeout(ctx, 100*time.Millisecond)
	err = store.WithBucketCopyKey(waitCtx, "bucket", func(_ context.Context, _ string) (string, error) {
		return "unexpected", nil
	})
	cancelWait()
	require.Error(t, err)

	var mu sync.Mutex
	var seen string
	second := make(chan error, 1)
	go func() {
		second <- store.WithBucketSigningKey(ctx, "bucket", func(_ context.Context, id string) (string, error) {
			mu.Lock()
			seen = id
			mu.Unlock()
			return id, nil
		})
	}()
	close(release)
	require.NoError(t, <-first)
	require.NoError(t, <-second)
	mu.Lock()
	assert.Equal(t, "key", seen)
	mu.Unlock()

	canceled, cancel := context.WithCancel(ctx)
	want := errors.New("provisioning interrupted")
	err = store.WithBucketReadKey(canceled, "bucket", func(_ context.Context, _ string) (string, error) {
		cancel()
		return "read-key", want
	})
	require.ErrorIs(t, err, want)
	var id string
	require.NoError(t, pool.QueryRow(ctx, `SELECT read_access_key_id FROM webui_upload_keys WHERE bucket_id = $1`, "bucket").Scan(&id))
	assert.Equal(t, "read-key", id)
	require.NoError(t, store.WithBucketReadKey(ctx, "bucket", func(_ context.Context, id string) (string, error) {
		assert.Equal(t, "read-key", id)
		return id, nil
	}))

	// The new session lock also coordinates with replicas using the old transaction lock.
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	_, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(748403197, hashtext($1))`, "legacy-bucket")
	require.NoError(t, err)
	waitCtx, cancelWait = context.WithTimeout(ctx, 100*time.Millisecond)
	err = store.WithBucketSigningKey(waitCtx, "legacy-bucket", func(_ context.Context, _ string) (string, error) {
		return "unexpected", nil
	})
	cancelWait()
	require.Error(t, err)
	require.NoError(t, tx.Rollback(ctx))
	require.NoError(t, store.WithBucketSigningKey(ctx, "legacy-bucket", func(_ context.Context, _ string) (string, error) {
		return "legacy-key", nil
	}))
}
