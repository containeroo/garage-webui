package postgres

import (
	"context"
	"crypto/sha256"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

const loginAttemptsPerMinute = 30

// AllowLogin shares a per-peer login-attempt budget across application replicas.
// The peer address is hashed and client-supplied forwarding headers are ignored.
func (s *Store) AllowLogin(ctx context.Context, peer string) (bool, error) {
	if peer == "" {
		return false, errors.New("missing login peer")
	}

	fingerprint := sha256.Sum256([]byte(peer))
	start := time.Now().UTC().Truncate(time.Minute)
	var attempts int
	err := s.db.QueryRow(ctx, `
        INSERT INTO webui_login_attempts (peer_hash, window_start, attempts)
        VALUES ($1,$2,1)
        ON CONFLICT (peer_hash, window_start) DO UPDATE
        SET attempts = webui_login_attempts.attempts + 1
        WHERE webui_login_attempts.attempts < $3
        RETURNING attempts`, fingerprint[:], start, loginAttemptsPerMinute).Scan(&attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}

	if err != nil {
		return false, err
	}

	return attempts <= loginAttemptsPerMinute, nil
}

// CleanupExpiredLoginAttempts prunes expired per-peer counters.
func (s *Store) CleanupExpiredLoginAttempts(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `DELETE FROM webui_login_attempts WHERE window_start < $1`, time.Now().UTC().Add(-2*time.Minute))
	return err
}
