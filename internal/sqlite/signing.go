package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/containeroo/garage-webui/internal/signing"
)

// bucketGate tracks holders and waiters without holding a SQLite transaction over Garage calls.
type bucketGate struct {
	// token provides the single permit for this bucket gate.
	token chan struct{}
	// references counts the holders and waiters using this gate.
	references int
}

// bucketLock owns an acquired bucket provisioning lock until release.
type bucketLock struct {
	// store owns the database and per-bucket gates.
	store *Store
	// bucketID identifies the locked bucket.
	bucketID string
	// gate holds the acquired per-bucket permit.
	gate *bucketGate
	// once ensures the lock is released only once.
	once sync.Once
	// closed reports whether the lock has been released.
	closed atomic.Bool
}

// LockBucket serializes provisioning and deletion for one Garage bucket.
func (s *Store) LockBucket(ctx context.Context, bucketID string) (signing.BucketLock, error) {
	if bucketID == "" {
		return nil, errors.New("bucket ID is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	gate := s.buckets[bucketID]
	if gate == nil {
		gate = &bucketGate{token: make(chan struct{}, 1)}
		gate.token <- struct{}{}
		s.buckets[bucketID] = gate
	}
	gate.references++
	s.mu.Unlock()
	select {
	case <-ctx.Done():
		s.releaseReference(bucketID, gate)
		return nil, ctx.Err()
	case <-gate.token:
		return &bucketLock{store: s, bucketID: bucketID, gate: gate}, nil
	}
}

// releaseReference frees unused gates once both holders and waiters are gone.
func (s *Store) releaseReference(bucketID string, gate *bucketGate) {
	s.mu.Lock()
	defer s.mu.Unlock()
	gate.references--
	if gate.references == 0 {
		delete(s.buckets, bucketID)
	}
}

// Close releases the bucket even when its request context was canceled.
func (l *bucketLock) Close() error {
	l.once.Do(func() {
		l.closed.Store(true)
		l.gate.token <- struct{}{}
		l.store.releaseReference(l.bucketID, l.gate)
	})
	return nil
}

// signingKeyColumn accepts only fixed SQL column names for known purposes.
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

// KeyID reads the reference saved by the current or a previous process.
func (l *bucketLock) KeyID(ctx context.Context, purpose signing.Purpose) (string, error) {
	if l.closed.Load() {
		return "", errors.New("bucket lock is closed")
	}
	column, err := signingKeyColumn(purpose)
	if err != nil {
		return "", err
	}
	var id string
	err = l.store.db.QueryRowContext(ctx, `SELECT COALESCE(`+column+`,'') FROM webui_upload_keys WHERE bucket_id=?`, l.bucketID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}

// SaveKeyID commits immediately, including after cancellation, so created Garage keys remain recoverable.
func (l *bucketLock) SaveKeyID(ctx context.Context, purpose signing.Purpose, id string) error {
	if l.closed.Load() {
		return errors.New("bucket lock is closed")
	}
	column, err := signingKeyColumn(purpose)
	if err != nil {
		return err
	}
	if id == "" {
		return errors.New("cannot persist an empty Garage key ID")
	}
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_, err = l.store.db.ExecContext(saveCtx, `INSERT INTO webui_upload_keys(bucket_id,`+column+`) VALUES(?,?) ON CONFLICT(bucket_id) DO UPDATE SET `+column+`=excluded.`+column, l.bucketID, id)
	return err
}

// KeyIDs returns distinct managed keys so deletion revokes each one once.
func (l *bucketLock) KeyIDs(ctx context.Context) ([]string, error) {
	if l.closed.Load() {
		return nil, errors.New("bucket lock is closed")
	}
	var writeID, readID, copyID string
	err := l.store.db.QueryRowContext(ctx, `SELECT COALESCE(access_key_id,''),COALESCE(read_access_key_id,''),COALESCE(copy_access_key_id,'') FROM webui_upload_keys WHERE bucket_id=?`, l.bucketID).Scan(&writeID, &readID, &copyID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for _, id := range []string{writeID, readID, copyID} {
		if id != "" && !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// DeleteKeyReferences clears references only after the caller's Garage cleanup succeeds.
func (l *bucketLock) DeleteKeyReferences(ctx context.Context) error {
	if l.closed.Load() {
		return errors.New("bucket lock is closed")
	}
	_, err := l.store.db.ExecContext(ctx, `DELETE FROM webui_upload_keys WHERE bucket_id=?`, l.bucketID)
	return err
}
