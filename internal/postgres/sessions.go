package postgres

import (
	"context"
	"errors"
	"fmt"

	kit "github.com/containeroo/appkit/auth"
	auth "github.com/gi8lino/garage-webui/internal/application/authentication"
	"github.com/jackc/pgx/v5"
)

// SetupRequired reports whether any account exists.
func (s *Store) SetupRequired(ctx context.Context) (bool, error) {
	var exists bool
	if err := s.db.QueryRow(ctx, `
  SELECT
    EXISTS (
      SELECT
        1
      FROM
        webui_users
    )
`).Scan(&exists); err != nil {
		return false, fmt.Errorf("check setup status: %w", err)
	}

	return !exists, nil
}

// Setup creates the first account and session atomically.
func (s *Store) Setup(ctx context.Context, username, passwordHash, mode string, hash []byte) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin setup: %w", err)
	}

	defer tx.Rollback(ctx) // nolint:errcheck
	if _, err = tx.Exec(ctx, `
  SELECT
    pg_advisory_xact_lock(748403197612)
`); err != nil {
		return fmt.Errorf("lock setup: %w", err)
	}

	var exists bool
	if err = tx.QueryRow(ctx, `
  SELECT
    EXISTS (
      SELECT
        1
      FROM
        webui_users
    )
`).Scan(&exists); err != nil {
		return fmt.Errorf("check setup: %w", err)
	}

	if exists {
		return auth.ErrSetupComplete
	}

	if _, err = tx.Exec(ctx, `
  INSERT INTO
    webui_users (username, password_hash)
  VALUES
    ($1, $2)
`, username, passwordHash); err != nil {
		return fmt.Errorf("create administrator: %w", err)
	}

	if _, err = tx.Exec(ctx, `
  UPDATE webui_auth_settings
  SET
    allow_registration = false
  WHERE
    singleton
`); err != nil {
		return fmt.Errorf("disable external registration: %w", err)
	}

	if _, err = tx.Exec(ctx, `
  INSERT INTO
    webui_sessions (token_hash, username, expires_at, mode)
  VALUES
    ($1, $2, now() + interval '12 hours', $3)
`, hash, username, mode); err != nil {
		return fmt.Errorf("create setup session: %w", err)
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("complete setup: %w", err)
	}

	return nil
}

// Session returns a valid session's user, role, and external administrator claim.
func (s *Store) Session(ctx context.Context, hash []byte, mode string, localLogin bool) (username, userRole string, externalAdmin bool, err error) {
	var user, role string
	err = s.db.QueryRow(ctx, `
  SELECT
    u.username,
    u.role,
    s.external_admin
  FROM
    webui_sessions s
    JOIN webui_users u ON u.username = s.username
  WHERE
    s.token_hash = $1
    AND s.expires_at > now()
    AND (
      s.mode = $2
      OR (
        s.mode = 'local'
        AND $3
      )
    )
    AND u.enabled
`, hash, mode, localLogin).Scan(&user, &role, &externalAdmin)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", false, kit.ErrUnauthenticated
	}
	return user, role, externalAdmin, err
}

// PasswordHash returns an enabled account's local credential.
func (s *Store) PasswordHash(ctx context.Context, username string) (string, error) {
	var hash string
	err := s.db.QueryRow(ctx, `
  SELECT
    password_hash
  FROM
    webui_users
  WHERE
    username = $1
    AND enabled
    AND local_credential_enabled
`, username).Scan(&hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", kit.ErrUnauthenticated
	}
	return hash, err
}

// DeleteSession revokes a session.
func (s *Store) DeleteSession(ctx context.Context, hash []byte) error {
	_, err := s.db.Exec(ctx, `
  DELETE FROM webui_sessions
  WHERE
    token_hash = $1
`, hash)
	return err
}

// SaveOIDCState stores an authorization-code verifier.
func (s *Store) SaveOIDCState(ctx context.Context, hash []byte, verifier string) error {
	_, err := s.db.Exec(ctx, `
  INSERT INTO
    webui_oidc_states (state_hash, verifier, expires_at)
  VALUES
    ($1, $2, now() + interval '10 minutes')
`, hash, verifier)
	return err
}

// ConsumeOIDCState consumes a valid authorization state once.
func (s *Store) ConsumeOIDCState(ctx context.Context, hash []byte) (string, error) {
	var verifier string
	err := s.db.QueryRow(ctx, `
  DELETE FROM webui_oidc_states
  WHERE
    state_hash = $1
    AND expires_at > now()
  RETURNING
    verifier
`, hash).Scan(&verifier)
	return verifier, err
}

// CreateSession verifies the credential or identity and creates a session atomically.
func (s *Store) CreateSession(
	ctx context.Context,
	user, passwordHash string,
	identity *auth.Identity,
	externalAdmin bool,
	mode string,
	hash []byte,
) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}

	defer tx.Rollback(ctx) // nolint:errcheck
	if err := lockActor(ctx, tx, user); err != nil {
		return err
	}

	var valid bool
	if identity == nil {
		err = tx.QueryRow(ctx, `
  SELECT
    EXISTS (
      SELECT
        1
      FROM
        webui_users
      WHERE
        username = $1
        AND enabled
        AND local_credential_enabled
        AND password_hash = $2
        AND password_hash <> ''
    )
`, user, passwordHash).Scan(&valid)
	} else {
		err = tx.QueryRow(ctx, `
  SELECT
    EXISTS (
      SELECT
        1
      FROM
        webui_identities
      WHERE
        username = $1
        AND provider = $2
        AND issuer = $3
        AND subject = $4
    )
`, user, identity.Provider, identity.Issuer, identity.Subject).Scan(&valid)
	}

	if err != nil {
		return err
	}

	if !valid {
		return kit.ErrUnauthenticated
	}

	if _, err := tx.Exec(ctx, `
  INSERT INTO
    webui_sessions (
      token_hash,
      username,
      expires_at,
      mode,
      external_admin
    )
  VALUES
    ($1, $2, now() + interval '12 hours', $3, $4)
`, hash, user, mode, externalAdmin); err != nil {
		return err
	}

	return tx.Commit(ctx)
}
