package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"time"
)

const loginAttemptsPerMinute = 30

// AllowLogin atomically enforces the same per-peer budget as PostgreSQL.
func (s *Store) AllowLogin(ctx context.Context, peer string) (bool, error) {
	return s.allowLoginAt(ctx, peer, time.Now())
}

// allowLoginAt enforces the peer login attempt budget at the supplied time.
func (s *Store) allowLoginAt(ctx context.Context, peer string, now time.Time) (bool, error) {
	if peer == "" {
		return false, errors.New("missing login peer")
	}
	fingerprint := sha256.Sum256([]byte(peer))
	start := now.UTC().Truncate(time.Minute).Unix()
	var attempts int
	err := s.db.QueryRowContext(ctx, `
 INSERT INTO webui_login_attempts(peer_hash,window_start,attempts) VALUES(?,?,1)
 ON CONFLICT(peer_hash,window_start) DO UPDATE SET attempts=webui_login_attempts.attempts+1
 WHERE webui_login_attempts.attempts<? RETURNING attempts`, fingerprint[:], start, loginAttemptsPerMinute).Scan(&attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil && attempts <= loginAttemptsPerMinute, err
}

// CleanupExpiredLoginAttempts removes expired counters without retaining peer addresses.
func (s *Store) CleanupExpiredLoginAttempts(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM webui_login_attempts WHERE window_start<?`, time.Now().Add(-2*time.Minute).Unix())
	return err
}
