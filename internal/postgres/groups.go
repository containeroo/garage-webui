// Package postgres implements PostgreSQL persistence for account management.
package postgres

import (
	"context"

	kit "github.com/containeroo/appkit/auth"
	"github.com/gi8lino/garage-webui/internal/accounts"
	"github.com/jackc/pgx/v5"
)

// GroupMappings lists configured OIDC group mappings.
func (s *Store) GroupMappings(ctx context.Context) ([]accounts.GroupMapping, error) {
	rows, err := s.db.Query(ctx, `
  SELECT
    external_group,
    group_name
  FROM
    webui_oidc_group_mappings
  ORDER BY
    external_group,
    group_name
`)
	if err != nil {
		return nil, err
	}

	defer rows.Close()
	result := []accounts.GroupMapping{}
	for rows.Next() {
		var mapping accounts.GroupMapping
		if err := rows.Scan(&mapping.ExternalGroup, &mapping.Group); err != nil {
			return nil, err
		}

		result = append(result, mapping)
	}

	return result, rows.Err()
}

// SetGroupMapping creates or removes an OIDC group mapping.
func (s *Store) SetGroupMapping(ctx context.Context, mapping accounts.GroupMapping, remove bool) error {
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

	var query string
	if remove {
		query = `
  DELETE FROM webui_oidc_group_mappings
  WHERE
    external_group = $1
    AND group_name = $2
`
	} else {
		query = `
  INSERT INTO
    webui_oidc_group_mappings (external_group, group_name)
  SELECT
    $1,
    name
  FROM
    webui_groups
  WHERE
    name = $2
  ON CONFLICT DO NOTHING
`
	}

	result, err := tx.Exec(ctx, query, mapping.ExternalGroup, mapping.Group)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		if remove {
			return accounts.ErrNotFound
		}

		return accounts.ErrConflict
	}

	if remove {
		if _, err = tx.Exec(ctx, `
  DELETE FROM webui_oidc_group_members
  WHERE
    group_name = $1
`, mapping.Group); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

// SyncOIDCGroups replaces provider-derived memberships after a verified login.
func (s *Store) SyncOIDCGroups(ctx context.Context, username string, groups []string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}

	defer tx.Rollback(ctx) // nolint:errcheck
	if err = lockActor(ctx, tx, username); err != nil {
		return err
	}

	if _, err = tx.Exec(ctx, `
  DELETE FROM webui_oidc_group_members
  WHERE
    username = $1
`, username); err != nil {
		return err
	}

	if _, err = tx.Exec(ctx, `
  INSERT INTO
    webui_oidc_group_members (username, group_name)
  SELECT
    $1,
    group_name
  FROM
    webui_oidc_group_mappings
  WHERE
    external_group = ANY ($2)
  GROUP BY
    group_name
`, username, groups); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// Groups lists local group names.
func (s *Store) Groups(ctx context.Context) ([]string, error) {
	rows, err := s.db.Query(ctx, `
  SELECT
    name
  FROM
    webui_groups
  ORDER BY
    name
`)
	if err != nil {
		return nil, err
	}

	defer rows.Close()
	groups := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}

		groups = append(groups, name)
	}

	return groups, rows.Err()
}

// CreateGroup creates a local group.
func (s *Store) CreateGroup(ctx context.Context, name string) error {
	result, err := s.db.Exec(ctx, `
  INSERT INTO
    webui_groups (name)
  VALUES
    ($1)
  ON CONFLICT DO NOTHING
`, name)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return accounts.ErrConflict
	}

	return nil
}

// DeleteGroup removes a group and its memberships.
func (s *Store) DeleteGroup(ctx context.Context, name string) error {
	result, err := s.db.Exec(ctx, `
  DELETE FROM webui_groups
  WHERE
    name = $1
`, name)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return accounts.ErrNotFound
	}

	return nil
}

// SetAccountGroups atomically replaces one account's local memberships.
func (s *Store) SetAccountGroups(ctx context.Context, actor, username string, groups []string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}

	defer tx.Rollback(ctx) // nolint:errcheck
	if err = lockActor(ctx, tx, actor); err != nil {
		return err
	}

	var exists bool
	if err = tx.QueryRow(ctx, `
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
		return accounts.ErrNotFound
	}

	var count int
	if err = tx.QueryRow(ctx, `
  SELECT
    count(*)
  FROM
    webui_groups
  WHERE
    name = ANY ($1)
`, groups).Scan(&count); err != nil {
		return err
	}

	if count != len(groups) {
		return accounts.ErrInvalid
	}

	if _, err = tx.Exec(ctx, `
  DELETE FROM webui_group_members
  WHERE
    username = $1
`, username); err != nil {
		return err
	}

	for _, group := range groups {
		if _, err = tx.Exec(ctx, `
  INSERT INTO
    webui_group_members (username, group_name)
  VALUES
    ($1, $2)
`, username, group); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

// lockActor serializes account mutations and verifies the acting account.
func lockActor(ctx context.Context, tx pgx.Tx, actor string) error {
	if _, err := tx.Exec(ctx, `
  SELECT
    pg_advisory_xact_lock(748403197613)
`); err != nil {
		return err
	}

	var enabled bool
	if err := tx.QueryRow(ctx, `
  SELECT
    enabled
  FROM
    webui_users
  WHERE
    username = $1
`, actor).Scan(&enabled); err != nil || !enabled {
		return kit.ErrUnauthenticated
	}

	return nil
}
