package postgres

import (
	"context"

	auth "github.com/gi8lino/garage-webui/internal/application/authentication"
)

// Profile contains the stored account profile.
type Profile = auth.Profile

// Profile returns one enabled user's profile.
func (s *Store) Profile(ctx context.Context, username string) (Profile, error) {
	var profile Profile
	err := s.db.QueryRow(ctx, `
  SELECT
    username,
    email,
    display_name,
    theme,
    local_credential_enabled AND password_hash <> ''
  FROM
    webui_users
  WHERE
    username = $1
    AND enabled
`, username).Scan(&profile.Username, &profile.Email, &profile.DisplayName, &profile.Theme, &profile.LocalCredentialEnabled)
	return profile, err
}

// UpdateOwnProfile changes the caller's locally managed profile fields.
func (s *Store) UpdateOwnProfile(ctx context.Context, username, email, displayName string) error {
	result, err := s.db.Exec(ctx, `
  UPDATE webui_users
  SET
    email = $2,
    display_name = $3,
    email_overridden = true,
    display_name_overridden = true
  WHERE
    username = $1
    AND enabled
`, username, email, displayName)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}

	return nil
}

// UpdateOwnTheme saves a preference without altering profile fields.
func (s *Store) UpdateOwnTheme(ctx context.Context, username, theme string) error {
	result, err := s.db.Exec(ctx, `UPDATE webui_users SET theme = $2 WHERE username = $1 AND enabled`, username, theme)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

// ChangeOwnPassword atomically replaces the verified credential and revokes sessions.
func (s *Store) ChangeOwnPassword(ctx context.Context, username, existing, hash string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}

	defer tx.Rollback(ctx) // nolint:errcheck
	if err = lockActor(ctx, tx, username); err != nil {
		return err
	}

	result, err := tx.Exec(ctx, `
  UPDATE webui_users
  SET
    password_hash = $2
  WHERE
    username = $1
    AND password_hash = $3
    AND local_credential_enabled
`, username, hash, existing)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrInvalidUser
	}

	if _, err = tx.Exec(ctx, `
  DELETE FROM webui_sessions
  WHERE
    username = $1
    AND mode = 'local'
`, username); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// RenameAccount changes a local username while preserving credentials and links.
func (s *Store) RenameAccount(ctx context.Context, actor, username, newUsername string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}

	defer tx.Rollback(ctx) // nolint:errcheck
	if err = lockActor(ctx, tx, actor); err != nil {
		return err
	}

	var proxyBound bool
	if err = tx.QueryRow(ctx, `
  SELECT
    EXISTS (
      SELECT
        1
      FROM
        webui_identities
      WHERE
        username = $1
        AND provider = 'proxy'
    )
`, username).Scan(&proxyBound); err != nil {
		return err
	}

	if proxyBound {
		return ErrUserConflict
	}

	result, err := tx.Exec(ctx, `
  INSERT INTO
    webui_users (
      username,
      password_hash,
      enabled,
      role,
      email,
      display_name,
      theme,
      email_overridden,
      display_name_overridden,
      local_credential_enabled
    )
  SELECT
    $2,
    password_hash,
    enabled,
    role,
    email,
    display_name,
    theme,
    email_overridden,
    display_name_overridden,
    local_credential_enabled
  FROM
    webui_users
  WHERE
    username = $1
  ON CONFLICT DO NOTHING
`, username, newUsername)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrUserConflict
	}

	for _, query := range []string{`
  UPDATE webui_identities
  SET
    username = $2
  WHERE
    username = $1
`, `
  UPDATE webui_group_members
  SET
    username = $2
  WHERE
    username = $1
`, `
  UPDATE webui_oidc_group_members
  SET
    username = $2
  WHERE
    username = $1
`, `
  UPDATE webui_sessions
  SET
    username = $2
  WHERE
    username = $1
`} {
		if _, err = tx.Exec(ctx, query, username, newUsername); err != nil {
			return err
		}
	}

	if _, err = tx.Exec(ctx, `
  DELETE FROM webui_users
  WHERE
    username = $1
`, username); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// refreshExternalProfile records provider fields without overriding local edits.
func (s *Store) RefreshExternalProfile(ctx context.Context, user string, id Identity, username, email, displayName string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}

	defer tx.Rollback(ctx) // nolint:errcheck
	if err = lockActor(ctx, tx, user); err != nil {
		return err
	}

	result, err := tx.Exec(ctx, `
  UPDATE webui_identities
  SET
    provider_username = $5,
    email = $6,
    display_name = $7,
    last_seen = now()
  WHERE
    username = $1
    AND provider = $2
    AND issuer = $3
    AND subject = $4
`, user, id.Provider, id.Issuer, id.Subject, username, email, displayName)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}

	if _, err = tx.Exec(ctx, `
  UPDATE webui_users
  SET
    email = CASE
      WHEN email_overridden THEN email
      ELSE $2
    END,
    display_name = CASE
      WHEN display_name_overridden THEN display_name
      ELSE $3
    END
  WHERE
    username = $1
`, user, email, displayName); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// RevertProfile restores provider-owned fields for a linked identity.
func (s *Store) RevertProfile(ctx context.Context, actor, username string, email, displayName bool) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}

	defer tx.Rollback(ctx) // nolint:errcheck
	if err = lockActor(ctx, tx, actor); err != nil {
		return err
	}

	result, err := tx.Exec(ctx, `
  UPDATE webui_users u
  SET
    email = CASE
      WHEN $2 THEN i.email
      ELSE u.email
    END,
    display_name = CASE
      WHEN $3 THEN i.display_name
      ELSE u.display_name
    END,
    email_overridden = CASE
      WHEN $2 THEN false
      ELSE u.email_overridden
    END,
    display_name_overridden = CASE
      WHEN $3 THEN false
      ELSE u.display_name_overridden
    END
  FROM
    LATERAL (
      SELECT
        email,
        display_name
      FROM
        webui_identities
      WHERE
        username = $1
        AND last_seen IS NOT NULL
      ORDER BY
        last_seen DESC
      LIMIT
        1
    ) i
  WHERE
    u.username = $1
`, username, email, displayName)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}

	return tx.Commit(ctx)
}
