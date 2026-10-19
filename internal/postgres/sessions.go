package postgres

import (
	"context"
	"time"
)

// SaveSession persists encrypted credentials under a hashed random session token.
func (s *Store) SaveSession(ctx context.Context, hash, credential []byte, expires time.Time) error {
	_, err := s.db.Exec(ctx, `INSERT INTO webui_sessions (token_hash,credential,expires_at) VALUES ($1,$2,$3)`, hash, credential, expires)
	return err
}

// LoadSession loads only an unexpired session.
func (s *Store) LoadSession(ctx context.Context, hash []byte) (credential []byte, theme string, err error) {
	err = s.db.QueryRow(ctx, `SELECT credential,theme FROM webui_sessions WHERE token_hash=$1 AND expires_at>now()`, hash).Scan(&credential, &theme)
	return
}

// DeleteSession revokes a browser session on every replica.
func (s *Store) DeleteSession(ctx context.Context, hash []byte) error {
	_, err := s.db.Exec(ctx, `DELETE FROM webui_sessions WHERE token_hash=$1`, hash)
	return err
}

// SetSessionTheme updates the current session's appearance preference.
func (s *Store) SetSessionTheme(ctx context.Context, hash []byte, theme string) error {
	_, err := s.db.Exec(ctx, `UPDATE webui_sessions SET theme=$2 WHERE token_hash=$1 AND expires_at>now()`, hash, theme)
	return err
}

// CleanupExpiredAuth removes expired credential sessions.
func (s *Store) CleanupExpiredAuth(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `DELETE FROM webui_sessions WHERE expires_at<=now()`)
	return err
}
