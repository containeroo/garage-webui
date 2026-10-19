package postgres

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/containeroo/garage-webui/internal/signing"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const bucketAdvisoryLockNamespace = 748403197

// bucketLock retains the PostgreSQL connection that owns a session advisory lock.
type bucketLock struct {
	// conn owns the PostgreSQL session advisory lock.
	conn *pgxpool.Conn
	// bucketID identifies the locked Garage bucket.
	bucketID string
	// once ensures the connection is released or discarded only once.
	once sync.Once
	// closeErr retains any failure when releasing the advisory lock.
	closeErr error
}

// LockBucket serializes signing-key work for a bucket across all Web UI replicas.
// Lock acquisition uses the request context; Close releases the lock independently.
func (s *Store) LockBucket(ctx context.Context, bucketID string) (signing.BucketLock, error) {
	if bucketID == "" {
		return nil, errors.New("bucket ID is required")
	}

	conn, err := s.db.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire bucket signing connection: %w", err)
	}

	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1, hashtext($2))`, bucketAdvisoryLockNamespace, bucketID); err != nil {
		// Cancellation may occur after the server acquires the lock but before
		// the reply arrives. Never return this connection to the pool.
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = conn.Hijack().Close(cleanup)

		return nil, fmt.Errorf("lock bucket signing keys: %w", err)
	}

	return &bucketLock{conn: conn, bucketID: bucketID}, nil
}

// Close releases the session lock, discarding the connection if unlock is uncertain.
// Closing an already closed handle is harmless.
func (l *bucketLock) Close() error {
	l.once.Do(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var unlocked bool
		err := l.conn.QueryRow(cleanup, `SELECT pg_advisory_unlock($1, hashtext($2))`, bucketAdvisoryLockNamespace, l.bucketID).Scan(&unlocked)
		if err != nil || !unlocked {
			l.closeErr = fmt.Errorf("release bucket signing lock: %w", errOrUnlockFailure(err))
			_ = l.conn.Hijack().Close(cleanup)

			return
		}

		l.conn.Release()
	})
	return l.closeErr
}

// errOrUnlockFailure distinguishes a failed query from a missing held lock.
func errOrUnlockFailure(err error) error {
	if err != nil {
		return err
	}

	return errors.New("advisory lock was not held")
}

// signingKeyColumn maps a fixed key purpose to its PostgreSQL column.
func signingKeyColumn(purpose signing.Purpose) (string, error) {
	switch purpose {
	case signing.Write:
		return "access_key_id", nil
	case signing.Read:
		return "read_access_key_id", nil
	case signing.Copy:
		return "copy_access_key_id", nil
	default:
		return "", fmt.Errorf("unknown bucket signing key purpose %q", purpose)
	}
}

// KeyID returns the saved Garage key ID for a purpose, or empty when absent.
func (l *bucketLock) KeyID(ctx context.Context, purpose signing.Purpose) (string, error) {
	column, err := signingKeyColumn(purpose)
	if err != nil {
		return "", err
	}

	var id string
	err = l.conn.QueryRow(ctx, `SELECT COALESCE(`+column+`, '') FROM webui_upload_keys WHERE bucket_id=$1`, l.bucketID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}

	if err != nil {
		return "", fmt.Errorf("load bucket signing key: %w", err)
	}

	return id, nil
}

// SaveKeyID records a created key even if the original request was canceled.
// This makes partial Garage provisioning recoverable on the next request.
func (l *bucketLock) SaveKeyID(ctx context.Context, purpose signing.Purpose, id string) error {
	column, err := signingKeyColumn(purpose)
	if err != nil {
		return err
	}

	if id == "" {
		return errors.New("cannot persist an empty Garage key ID")
	}

	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_, err = l.conn.Exec(saveCtx, `
		INSERT INTO webui_upload_keys (bucket_id, `+column+`)
		VALUES ($1, $2)
		ON CONFLICT (bucket_id) DO UPDATE SET `+column+` = EXCLUDED.`+column,
		l.bucketID, id)
	if err != nil {
		return fmt.Errorf("save bucket signing key: %w", err)
	}

	return nil
}

// KeyIDs returns distinct stored Garage key IDs for a bucket.
func (l *bucketLock) KeyIDs(ctx context.Context) ([]string, error) {
	var writeID, readID, copyID string
	err := l.conn.QueryRow(ctx, `
		SELECT COALESCE(access_key_id,''), COALESCE(read_access_key_id,''), COALESCE(copy_access_key_id,'')
		FROM webui_upload_keys WHERE bucket_id=$1`, l.bucketID).Scan(&writeID, &readID, &copyID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("load managed signing keys: %w", err)
	}

	ids := make([]string, 0, 3)
	for _, id := range []string{writeID, readID, copyID} {
		if id != "" && !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}

	return ids, nil
}

// DeleteKeyReferences removes persisted key references after Garage cleanup succeeds.
func (l *bucketLock) DeleteKeyReferences(ctx context.Context) error {
	_, err := l.conn.Exec(ctx, `DELETE FROM webui_upload_keys WHERE bucket_id=$1`, l.bucketID)
	if err != nil {
		return fmt.Errorf("remove bucket signing key references: %w", err)
	}

	return nil
}
