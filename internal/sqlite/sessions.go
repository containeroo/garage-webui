package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/containeroo/garage-webui/internal/storage"
)

// SaveSession persists encrypted Garage credentials under the hashed session token.
func (s *Store) SaveSession(ctx context.Context, hash, credential []byte, expires time.Time) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO webui_sessions(token_hash,credential,expires_at) VALUES(?,?,?)`, hash, credential, expires.UnixMilli())
	return err
}

// LoadSession retrieves only a live session and hides SQL-driver errors from auth.
func (s *Store) LoadSession(ctx context.Context, hash []byte) (credential []byte, theme string, err error) {
	err = s.db.QueryRowContext(ctx, `SELECT credential,theme FROM webui_sessions WHERE token_hash=? AND expires_at>?`, hash, time.Now().UnixMilli()).Scan(&credential, &theme)
	if errors.Is(err, sql.ErrNoRows) {
		err = storage.ErrSessionNotFound
	}
	return
}

// DeleteSession revokes one browser session.
func (s *Store) DeleteSession(ctx context.Context, hash []byte) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM webui_sessions WHERE token_hash=?`, hash)
	return err
}

// SetSessionTheme changes appearance only for a live session.
func (s *Store) SetSessionTheme(ctx context.Context, hash []byte, theme string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE webui_sessions SET theme=? WHERE token_hash=? AND expires_at>?`, theme, hash, time.Now().UnixMilli())
	return err
}

// CleanupExpiredAuth removes expired encrypted credential sessions.
func (s *Store) CleanupExpiredAuth(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM webui_sessions WHERE expires_at<=?`, time.Now().UnixMilli())
	return err
}
