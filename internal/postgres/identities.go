package postgres

import (
	"context"
	"errors"
	"fmt"

	kit "github.com/containeroo/appkit/auth"
	"github.com/jackc/pgx/v5"
)

// LinkIdentity associates an external identity with an existing local account.
func (s *Store) LinkIdentity(ctx context.Context, username string, id Identity) error {
	return s.linkIdentity(ctx, username, id, false)
}

// LinkPendingIdentity links an identity only if it is awaiting review.
func (s *Store) LinkPendingIdentity(ctx context.Context, username string, id Identity) error {
	return s.linkIdentity(ctx, username, id, true)
}

// linkIdentity atomically validates and binds the identity.
func (s *Store) linkIdentity(ctx context.Context, username string, id Identity, pending bool) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}

	defer tx.Rollback(ctx) // nolint:errcheck
	if _, err = tx.Exec(ctx, `
  SELECT
    pg_advisory_xact_lock(748403197613)
`); err != nil {
		return err
	}
	if pending {
		var exists bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM webui_pending_identities WHERE issuer = $1 AND subject = $2 AND status = 'pending')`, id.Issuer, id.Subject).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrUserNotFound
		}
	}

	result, err := tx.Exec(ctx, `
  INSERT INTO
    webui_identities (provider, issuer, subject, username)
  SELECT
    $1,
    $2,
    $3,
    username
  FROM
    webui_users
  WHERE
    username = $4
  ON CONFLICT DO NOTHING
`, id.Provider, id.Issuer, id.Subject, username)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrUserConflict
	}

	if _, err = tx.Exec(ctx, `
  DELETE FROM webui_retired_identities
  WHERE
    provider = $1
    AND issuer = $2
    AND subject = $3
`, id.Provider, id.Issuer, id.Subject); err != nil {
		return err
	}

	if id.Provider == "oidc" {
		if _, err = tx.Exec(ctx, `
  DELETE FROM webui_pending_identities
  WHERE
    issuer = $1
    AND subject = $2
`, id.Issuer, id.Subject); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

// PendingIdentities lists verified OIDC identities awaiting an account link.
func (s *Store) PendingIdentities(ctx context.Context) ([]PendingIdentity, error) {
	rows, err := s.db.Query(ctx, `
  SELECT
    issuer,
    subject,
    username,
    status
  FROM
    webui_pending_identities
  ORDER BY
    username,
    issuer,
    subject
`)
	if err != nil {
		return nil, err
	}

	defer rows.Close()
	identities := []PendingIdentity{}
	for rows.Next() {
		identity := PendingIdentity{Identity: Identity{Provider: "oidc"}}
		if err := rows.Scan(&identity.Issuer, &identity.Subject, &identity.Username, &identity.Status); err != nil {
			return nil, err
		}

		identities = append(identities, identity)
	}

	return identities, rows.Err()
}

// RejectPendingIdentity retires an unlinked OIDC identity.
func (s *Store) RejectPendingIdentity(ctx context.Context, id Identity) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}

	defer tx.Rollback(ctx) // nolint:errcheck
	if _, err = tx.Exec(ctx, `
  SELECT
    pg_advisory_xact_lock(748403197613)
`); err != nil {
		return err
	}

	result, err := tx.Exec(ctx, `
  UPDATE webui_pending_identities
  SET
    status = 'rejected'
  WHERE
    issuer = $1
    AND subject = $2
`, id.Issuer, id.Subject)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}

	if _, err = tx.Exec(ctx, `
  INSERT INTO
    webui_retired_identities (provider, issuer, subject)
  VALUES
    ('oidc', $1, $2)
  ON CONFLICT DO NOTHING
`, id.Issuer, id.Subject); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// ReopenPendingIdentity allows an administrator to review a rejected identity.
func (s *Store) ReopenPendingIdentity(ctx context.Context, id Identity) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}

	defer tx.Rollback(ctx) // nolint:errcheck
	if _, err = tx.Exec(ctx, `
  SELECT
    pg_advisory_xact_lock(748403197613)
`); err != nil {
		return err
	}

	result, err := tx.Exec(ctx, `
  UPDATE webui_pending_identities
  SET
    status = 'pending'
  WHERE
    issuer = $1
    AND subject = $2
    AND status = 'rejected'
`, id.Issuer, id.Subject)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}

	if _, err = tx.Exec(ctx, `
  DELETE FROM webui_retired_identities
  WHERE
    provider = 'oidc'
    AND issuer = $1
    AND subject = $2
`, id.Issuer, id.Subject); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// ApprovePendingIdentity creates a viewer account for a verified OIDC identity.
func (s *Store) ApprovePendingIdentity(ctx context.Context, id Identity, username string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}

	defer tx.Rollback(ctx) // nolint:errcheck
	if _, err = tx.Exec(ctx, `
  SELECT
    pg_advisory_xact_lock(748403197613)
`); err != nil {
		return err
	}

	var pending bool
	if err = tx.QueryRow(ctx, `
  SELECT
    EXISTS (
      SELECT
        1
      FROM
        webui_pending_identities
      WHERE
        issuer = $1
        AND subject = $2
        AND status = 'pending'
    )
`, id.Issuer, id.Subject).Scan(&pending); err != nil {
		return err
	}

	if !pending {
		return ErrUserNotFound
	}

	result, err := tx.Exec(ctx, `
  INSERT INTO
    webui_users (username, password_hash, role)
  VALUES
    ($1, '', 'viewer')
  ON CONFLICT DO NOTHING
`, username)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrUserConflict
	}

	if _, err = tx.Exec(ctx, `
  INSERT INTO
    webui_identities (provider, issuer, subject, username)
  VALUES
    ('oidc', $1, $2, $3)
`, id.Issuer, id.Subject, username); err != nil {
		return err
	}

	if _, err = tx.Exec(ctx, `
  DELETE FROM webui_pending_identities
  WHERE
    issuer = $1
    AND subject = $2
`, id.Issuer, id.Subject); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// UnlinkIdentity removes a binding and prevents automatic re-registration.
func (s *Store) UnlinkIdentity(ctx context.Context, username string, id Identity) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}

	defer tx.Rollback(ctx) // nolint:errcheck
	if _, err = tx.Exec(ctx, `
  SELECT
    pg_advisory_xact_lock(748403197613)
`); err != nil {
		return err
	}

	result, err := tx.Exec(ctx, `
  DELETE FROM webui_identities
  WHERE
    provider = $1
    AND issuer = $2
    AND subject = $3
    AND username = $4
`, id.Provider, id.Issuer, id.Subject, username)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}

	if _, err = tx.Exec(ctx, `
  INSERT INTO
    webui_retired_identities (provider, issuer, subject)
  VALUES
    ($1, $2, $3)
  ON CONFLICT DO NOTHING
`, id.Provider, id.Issuer, id.Subject); err != nil {
		return err
	}

	if _, err = tx.Exec(ctx, `
  DELETE FROM webui_sessions
  WHERE
    username = $1
    AND mode = $2
`, username, id.Provider); err != nil {
		return err
	}

	if id.Provider == "oidc" {
		if _, err = tx.Exec(ctx, `
  DELETE FROM webui_oidc_group_members
  WHERE
    username = $1
`, username); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

// RevokeAccountSessions signs an account out of all browser sessions.
func (s *Store) RevokeAccountSessions(ctx context.Context, username string) error {
	result, err := s.db.Exec(ctx, `
  DELETE FROM webui_sessions
  WHERE
    username = $1
    AND EXISTS (
      SELECT
        1
      FROM
        webui_users
      WHERE
        username = $1
    )
`, username)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		var exists bool
		if err := s.db.QueryRow(ctx, `
  SELECT
    EXISTS (
      SELECT
        1
      FROM
        webui_users
      WHERE
        username = $1
    )
`, username).Scan(&exists); err != nil {
			return err
		}

		if !exists {
			return ErrUserNotFound
		}
	}

	return nil
}

// RelinkProxyIdentity replaces a trusted-proxy identity without changing its account.
func (s *Store) RelinkProxyIdentity(ctx context.Context, username, oldSubject, newSubject string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}

	defer tx.Rollback(ctx) // nolint:errcheck
	if _, err = tx.Exec(ctx, `
  SELECT
    pg_advisory_xact_lock(748403197613)
`); err != nil {
		return err
	}

	var exists bool
	if err = tx.QueryRow(ctx, `
  SELECT
    EXISTS (
      SELECT
        1
      FROM
        webui_identities
      WHERE
        provider = 'proxy'
        AND issuer = ''
        AND subject = $1
    )
`, newSubject).Scan(&exists); err != nil {
		return err
	}

	if exists {
		return ErrUserConflict
	}

	result, err := tx.Exec(ctx, `
  UPDATE webui_identities
  SET
    subject = $3,
    provider_username = $3
  WHERE
    provider = 'proxy'
    AND issuer = ''
    AND subject = $1
    AND username = $2
`, oldSubject, username, newSubject)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}

	if _, err = tx.Exec(ctx, `
  INSERT INTO
    webui_retired_identities (provider, issuer, subject)
  VALUES
    ('proxy', '', $1)
  ON CONFLICT DO NOTHING
`, oldSubject); err != nil {
		return err
	}

	if _, err = tx.Exec(ctx, `
  DELETE FROM webui_retired_identities
  WHERE
    provider = 'proxy'
    AND issuer = ''
    AND subject = $1
`, newSubject); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// resolveExternal resolves a verified identity without implicitly linking existing accounts.
func (s *Store) ResolveExternal(ctx context.Context, provider, issuer, subject, username string, registrationOverride *bool) (string, error) {
	id := Identity{
		Provider: provider,
		Issuer:   issuer,
		Subject:  subject,
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", err
	}

	defer tx.Rollback(ctx) // nolint:errcheck
	if _, err = tx.Exec(ctx, `
  SELECT
    pg_advisory_xact_lock(748403197613)
`); err != nil {
		return "", err
	}

	var user string
	var enabled bool
	err = tx.QueryRow(ctx, `
  SELECT
    u.username,
    u.enabled
  FROM
    webui_identities i
    JOIN webui_users u ON u.username = i.username
  WHERE
    i.provider = $1
    AND i.issuer = $2
    AND i.subject = $3
`, provider, issuer, subject).Scan(&user, &enabled)
	if err == nil {
		if !enabled {
			return "", kit.ErrUnauthenticated
		}

		return user, tx.Commit(ctx)
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}

	var allowed, retired, pending bool
	if err = tx.QueryRow(ctx, `
  SELECT
    COALESCE($4::boolean, allow_registration),
    EXISTS (
      SELECT
        1
      FROM
        webui_retired_identities
      WHERE
        provider = $1
        AND issuer = $2
        AND subject = $3
    ),
    EXISTS (
      SELECT
        1
      FROM
        webui_pending_identities
      WHERE
        issuer = $2
        AND subject = $3
        AND $1 = 'oidc'
    )
  FROM
    webui_auth_settings
  WHERE
    singleton
`, provider, issuer, subject, registrationOverride).Scan(&allowed, &retired, &pending); err != nil {
		return "", err
	}

	if retired {
		return "", kit.ErrUnauthenticated
	}

	if !allowed || pending {
		if provider == "oidc" {
			if err := recordPending(ctx, tx, id, username); err != nil {
				return "", err
			}

			if err := tx.Commit(ctx); err != nil {
				return "", err
			}
		}

		return "", kit.ErrUnauthenticated
	}

	result, err := tx.Exec(ctx, `
  INSERT INTO
    webui_users (username, password_hash, role)
  VALUES
    ($1, '', 'viewer')
  ON CONFLICT DO NOTHING
`, username)
	if err != nil {
		return "", err
	}

	if result.RowsAffected() == 0 {
		if provider == "oidc" {
			if err := recordPending(ctx, tx, id, username); err != nil {
				return "", err
			}

			if err := tx.Commit(ctx); err != nil {
				return "", err
			}
		}

		return "", fmt.Errorf("identity requires administrator linking: %w", ErrUserConflict)
	}

	if _, err = tx.Exec(ctx, `
  INSERT INTO
    webui_identities (provider, issuer, subject, username)
  VALUES
    ($1, $2, $3, $4)
`, provider, issuer, subject, username); err != nil {
		return "", err
	}

	return username, tx.Commit(ctx)
}

// recordPending keeps an OIDC identity for an administrator to link or reject.
func recordPending(ctx context.Context, tx pgx.Tx, id Identity, username string) error {
	_, err := tx.Exec(ctx, `
  INSERT INTO
    webui_pending_identities (issuer, subject, username)
  VALUES
    ($1, $2, $3)
  ON CONFLICT (issuer, subject) DO UPDATE
  SET
    username = excluded.username
`, id.Issuer, id.Subject, username)
	return err
}
