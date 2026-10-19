package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/containeroo/garage-webui/internal/signing"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSigningKeyColumns rejects unsupported purposes before SQL is constructed.
func TestSigningKeyColumns(t *testing.T) {
	for _, purpose := range []signing.Purpose{signing.Write, signing.Read, signing.Copy} {
		column, err := signingKeyColumn(purpose)
		require.NoError(t, err)
		assert.NotEmpty(t, column)
	}
	_, err := signingKeyColumn("unknown")
	require.Error(t, err)
}

// TestSigningKeyCoordination verifies explicit bucket locks, cancellation and recovery.
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

	first, err := store.LockBucket(ctx, "bucket")
	require.NoError(t, err)
	other, err := store.LockBucket(ctx, "other-bucket")
	require.NoError(t, err)
	require.NoError(t, other.Close())

	var transactions int
	err = admin.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity
		WHERE application_name = $1 AND state = 'idle in transaction'`, schema).Scan(&transactions)
	require.NoError(t, err)
	assert.Zero(t, transactions)

	waitCtx, cancelWait := context.WithTimeout(ctx, 100*time.Millisecond)
	_, err = store.LockBucket(waitCtx, "bucket")
	cancelWait()
	require.Error(t, err)

	require.NoError(t, first.SaveKeyID(ctx, signing.Write, "key"))
	id, err := first.KeyID(ctx, signing.Write)
	require.NoError(t, err)
	assert.Equal(t, "key", id)
	require.NoError(t, first.Close())
	require.NoError(t, first.Close())

	second, err := store.LockBucket(ctx, "bucket")
	require.NoError(t, err)
	id, err = second.KeyID(ctx, signing.Write)
	require.NoError(t, err)
	assert.Equal(t, "key", id)

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	// A canceled HTTP request must not lose a key already created in Garage.
	require.NoError(t, second.SaveKeyID(canceled, signing.Read, "read-key"))
	require.NoError(t, second.SaveKeyID(ctx, signing.Copy, "read-key"))
	_, err = second.KeyID(ctx, "unknown")
	require.Error(t, err)
	ids, err := second.KeyIDs(ctx)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"key", "read-key"}, ids)
	require.NoError(t, second.DeleteKeyReferences(ctx))
	ids, err = second.KeyIDs(ctx)
	require.NoError(t, err)
	assert.Empty(t, ids)
	require.NoError(t, second.Close())

	// The session lock coordinates with the old transaction advisory lock.
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	_, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1, hashtext($2))`, bucketAdvisoryLockNamespace, "legacy-bucket")
	require.NoError(t, err)
	waitCtx, cancelWait = context.WithTimeout(ctx, 100*time.Millisecond)
	_, err = store.LockBucket(waitCtx, "legacy-bucket")
	cancelWait()
	require.Error(t, err)
	require.NoError(t, tx.Rollback(ctx))
	legacy, err := store.LockBucket(ctx, "legacy-bucket")
	require.NoError(t, err)
	require.NoError(t, legacy.SaveKeyID(ctx, signing.Write, "legacy-key"))
	require.NoError(t, legacy.Close())

	_, err = store.LockBucket(ctx, "")
	require.Error(t, err)
	assert.False(t, errors.Is(err, context.Canceled))
}
