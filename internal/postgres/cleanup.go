package postgres

import (
	"context"
	"fmt"
)

const cleanupBatchSize = 1000

// CleanupExpiredAuth removes up to one batch of expired records per table.
func (s *Store) CleanupExpiredAuth(ctx context.Context) error {
	if _, err := s.db.Exec(ctx, `
		DELETE FROM webui_sessions WHERE ctid IN (
			SELECT ctid FROM webui_sessions WHERE expires_at <= now()
			ORDER BY expires_at LIMIT $1 FOR UPDATE SKIP LOCKED
		)`, cleanupBatchSize); err != nil {
		return fmt.Errorf("delete expired sessions: %w", err)
	}
	if _, err := s.db.Exec(ctx, `
		DELETE FROM webui_oidc_states WHERE ctid IN (
			SELECT ctid FROM webui_oidc_states WHERE expires_at <= now()
			ORDER BY expires_at LIMIT $1 FOR UPDATE SKIP LOCKED
		)`, cleanupBatchSize); err != nil {
		return fmt.Errorf("delete expired OIDC states: %w", err)
	}
	return nil
}
