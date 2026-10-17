package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// WithBucketSigningKey serializes write-key provisioning across application replicas.
func (s *Store) WithBucketSigningKey(ctx context.Context, bucketID string, prepare func(context.Context, string) (string, error)) error {
	return s.withBucketKey(ctx, bucketID, "access_key_id", prepare)
}

// WithBucketReadKey serializes read-key provisioning across application replicas.
func (s *Store) WithBucketReadKey(ctx context.Context, bucketID string, prepare func(context.Context, string) (string, error)) error {
	return s.withBucketKey(ctx, bucketID, "read_access_key_id", prepare)
}

// WithBucketCopyKey serializes bucket-scoped read/write copy-key provisioning.
func (s *Store) WithBucketCopyKey(ctx context.Context, bucketID string, prepare func(context.Context, string) (string, error)) error {
	return s.withBucketKey(ctx, bucketID, "copy_access_key_id", prepare)
}

// withBucketKey persists only the Garage key ID, even when later setup fails.
// The column argument comes only from this package's three fixed wrapper methods.
func (s *Store) withBucketKey(
	ctx context.Context,
	bucketID, column string,
	prepare func(context.Context, string) (string, error),
) (result error) {
	conn, err := s.db.Acquire(ctx)
	if err != nil {
		return err
	}

	locked := false
	closed := false
	defer func() {
		if closed {
			return
		}

		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if locked {
			var unlocked bool
			err := conn.QueryRow(cleanup, `SELECT pg_advisory_unlock(748403197, hashtext($1))`, bucketID).Scan(&unlocked)
			if err != nil || !unlocked {
				result = errors.Join(result, errors.New("release bucket signing lock"))
				_ = conn.Hijack().Close(cleanup)
				return
			}
		}

		conn.Release()
	}()

	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock(748403197, hashtext($1))`, bucketID); err != nil {
		// A canceled lock request may have acquired the lock before the response.
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		closed = true
		_ = conn.Hijack().Close(cleanup)
		return fmt.Errorf("lock bucket signing keys: %w", err)
	}

	locked = true

	var keyID string
	err = conn.QueryRow(ctx, `SELECT COALESCE(`+column+`, '') FROM webui_upload_keys WHERE bucket_id = $1`, bucketID).Scan(&keyID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("load bucket signing key: %w", err)
	}

	provisionCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	newID, provisionErr := prepare(provisionCtx, keyID)
	cancel()
	if newID == "" {
		if provisionErr != nil {
			return provisionErr
		}

		return errors.New("bucket signing key provisioning returned an empty key ID")
	}

	if newID != keyID {
		// Persist a known key even if the client disconnected during provisioning.
		saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_, err = conn.Exec(saveCtx, `
			INSERT INTO webui_upload_keys (bucket_id, `+column+`)
			VALUES ($1, $2)
			ON CONFLICT (bucket_id) DO UPDATE SET `+column+` = EXCLUDED.`+column,
			bucketID, newID)
		if err != nil {
			return fmt.Errorf("save bucket signing key: %w", err)
		}
	}

	return provisionErr
}
