package postgres

import (
	"context"
	"errors"

	kit "github.com/containeroo/appkit/auth"
	auth "github.com/gi8lino/garage-webui/internal/application/authentication"
	"github.com/jackc/pgx/v5"
)

// Account is the account representation returned by the store.
type Account = auth.Account

// Identity identifies an external account binding.
type Identity = auth.Identity

// PendingIdentity is an external identity awaiting review.
type PendingIdentity = auth.PendingIdentity

// ErrUserConflict indicates a conflicting account or identity.
var ErrUserConflict = auth.ErrUserConflict

// ErrInvalidUser indicates invalid account data.
var ErrInvalidUser = auth.ErrInvalidUser

// ErrUserNotFound indicates a missing account or identity.
var ErrUserNotFound = auth.ErrUserNotFound

// Accounts lists accounts and their external identity bindings.
func (s *Store) Accounts(ctx context.Context) ([]Account, error) {
	rows, err := s.db.Query(ctx, `
  SELECT
    u.username,
    u.role,
    u.email,
    u.display_name,
    u.email_overridden,
    u.display_name_overridden,
    u.enabled,
    u.password_hash <> '',
    u.local_credential_enabled,
    ARRAY(
      SELECT
        group_name
      FROM
        webui_group_members
      WHERE
        username = u.username
      ORDER BY
        group_name
    ),
    ARRAY(
      SELECT
        group_name
      FROM
        webui_oidc_group_members
      WHERE
        username = u.username
      ORDER BY
        group_name
    ),
    i.provider,
    i.issuer,
    i.subject
  FROM
    webui_users u
    LEFT JOIN webui_identities i ON i.username = u.username
  ORDER BY
    u.username,
    i.provider,
    i.issuer,
    i.subject
`)
	if err != nil {
		return nil, err
	}

	defer rows.Close()
	accounts := []Account{}
	for rows.Next() {
		var user Account
		var provider, issuer, subject *string
		if err := rows.Scan(
			&user.Username,
			&user.Role,
			&user.Email,
			&user.DisplayName,
			&user.EmailOverridden,
			&user.DisplayNameOverridden,
			&user.Enabled,
			&user.HasPassword,
			&user.LocalCredentialEnabled,
			&user.Groups,
			&user.ExternalGroups,
			&provider,
			&issuer,
			&subject,
		); err != nil {
			return nil, err
		}

		if len(accounts) == 0 || accounts[len(accounts)-1].Username != user.Username {
			user.Identities = []Identity{}
			accounts = append(accounts, user)
		}

		if provider != nil {
			accounts[len(accounts)-1].Identities = append(accounts[len(accounts)-1].Identities, Identity{
				Provider: *provider,
				Issuer:   *issuer,
				Subject:  *subject,
			})
		}
	}

	return accounts, rows.Err()
}

// SetLocalCredentialEnabled changes whether a local password can authenticate.
func (s *Store) SetLocalCredentialEnabled(ctx context.Context, actor, username string, enabled bool) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}

	defer tx.Rollback(ctx) // nolint:errcheck
	if err = lockActor(ctx, tx, actor); err != nil {
		return err
	}

	result, err := tx.Exec(ctx, `
  UPDATE webui_users
  SET
    local_credential_enabled = $2
  WHERE
    username = $1
`, username, enabled)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}

	if !enabled {
		if _, err = tx.Exec(ctx, `
  DELETE FROM webui_sessions
  WHERE
    username = $1
    AND mode = 'local'
`, username); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

// Role returns the enabled account's authorization role.
func (s *Store) Role(ctx context.Context, username string) (string, error) {
	var role string
	err := s.db.QueryRow(ctx, `
  SELECT
    role
  FROM
    webui_users
  WHERE
    username = $1
    AND enabled
`, username).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", kit.ErrUnauthenticated
	}
	return role, err
}

// SetAccountProfile updates an account's role and profile under administrator lock.
func (s *Store) SetAccountProfile(ctx context.Context, actor, username, role, email, displayName string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}

	defer tx.Rollback(ctx) // nolint:errcheck
	if err = lockActor(ctx, tx, actor); err != nil {
		return err
	}

	result, err := tx.Exec(ctx, `
  UPDATE webui_users
  SET role = $2,
  email = $3,
  display_name = $4,
  email_overridden = true,
  display_name_overridden = true
  WHERE
    username = $1
`, username, role, email, displayName)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}

	return tx.Commit(ctx)
}

// Registration reports whether unknown external identities may register.
func (s *Store) Registration(ctx context.Context) (bool, error) {
	var enabled bool
	err := s.db.QueryRow(ctx, `
  SELECT
    allow_registration
  FROM
    webui_auth_settings
  WHERE
    singleton
`).Scan(&enabled)
	return enabled, err
}

// SetRegistration changes external identity registration policy.
func (s *Store) SetRegistration(ctx context.Context, enabled bool) error {
	_, err := s.db.Exec(ctx, `
  UPDATE webui_auth_settings
  SET
    allow_registration = $1
  WHERE
    singleton
`, enabled)
	return err
}

// CreateAccount creates a local account with an optional password.
func (s *Store) CreateAccount(ctx context.Context, username, hash string) error {
	result, err := s.db.Exec(ctx, `
  INSERT INTO
    webui_users (username, password_hash, role)
  VALUES
    ($1, $2, 'viewer')
  ON CONFLICT DO NOTHING
`, username, hash)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrUserConflict
	}

	return nil
}

// UpdateAccount changes account status and optionally its local password.
func (s *Store) UpdateAccount(ctx context.Context, actor, username string, enabled bool, hash string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}

	defer tx.Rollback(ctx) // nolint:errcheck
	if err := lockActor(ctx, tx, actor); err != nil {
		return err
	}

	result, err := tx.Exec(ctx, `
  UPDATE webui_users
  SET
    enabled = $2,
    password_hash = CASE
      WHEN $3 = '' THEN password_hash
      ELSE $3
    END
  WHERE
    username = $1
`, username, enabled, hash)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}

	if !enabled || hash != "" {
		if _, err := tx.Exec(ctx, `
  DELETE FROM webui_sessions
  WHERE
    username = $1
`, username); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

// DeleteAccount removes an account and retires its linked identities.
func (s *Store) DeleteAccount(ctx context.Context, actor, username string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}

	defer tx.Rollback(ctx) // nolint:errcheck
	if err = lockActor(ctx, tx, actor); err != nil {
		return err
	}

	if _, err = tx.Exec(ctx, `
  INSERT INTO
    webui_retired_identities
  SELECT
    provider,
    issuer,
    subject
  FROM
    webui_identities
  WHERE
    username = $1
  ON CONFLICT DO NOTHING
`, username); err != nil {
		return err
	}

	if _, err = tx.Exec(ctx, `
  DELETE FROM webui_sessions
  WHERE
    username = $1
`, username); err != nil {
		return err
	}

	result, err := tx.Exec(ctx, `
  DELETE FROM webui_users
  WHERE
    username = $1
`, username)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}

	return tx.Commit(ctx)
}
