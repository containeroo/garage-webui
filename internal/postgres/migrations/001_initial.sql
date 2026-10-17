  CREATE TABLE IF NOT EXISTS webui_users (
    username text PRIMARY KEY,
    password_hash text NOT NULL
  );

  ALTER TABLE webui_users
  ADD COLUMN IF NOT EXISTS enabled boolean NOT NULL DEFAULT true;

  ALTER TABLE webui_users
  ADD COLUMN IF NOT EXISTS role text NOT NULL DEFAULT 'admin';

  ALTER TABLE webui_users
  ADD COLUMN IF NOT EXISTS email text NOT NULL DEFAULT '';

  ALTER TABLE webui_users
  ADD COLUMN IF NOT EXISTS display_name text NOT NULL DEFAULT '';

  ALTER TABLE webui_users
  ADD COLUMN IF NOT EXISTS email_overridden boolean NOT NULL DEFAULT false;

  ALTER TABLE webui_users
  ADD COLUMN IF NOT EXISTS display_name_overridden boolean NOT NULL DEFAULT false;

  ALTER TABLE webui_users
  ADD COLUMN IF NOT EXISTS local_credential_enabled boolean NOT NULL DEFAULT true;

  CREATE TABLE IF NOT EXISTS webui_groups (name text PRIMARY KEY);

  CREATE TABLE IF NOT EXISTS webui_group_members (
    username text NOT NULL REFERENCES webui_users (username) ON DELETE CASCADE,
    group_name text NOT NULL REFERENCES webui_groups (name) ON DELETE CASCADE,
    PRIMARY KEY (username, group_name)
  );

  CREATE TABLE IF NOT EXISTS webui_oidc_group_mappings (
    external_group text NOT NULL,
    group_name text NOT NULL REFERENCES webui_groups (name) ON DELETE CASCADE,
    PRIMARY KEY (external_group, group_name)
  );

  CREATE TABLE IF NOT EXISTS webui_oidc_group_members (
    username text NOT NULL REFERENCES webui_users (username) ON DELETE CASCADE,
    group_name text NOT NULL REFERENCES webui_groups (name) ON DELETE CASCADE,
    PRIMARY KEY (username, group_name)
  );

  CREATE TABLE IF NOT EXISTS webui_sessions (
    token_hash bytea PRIMARY KEY,
    username text NOT NULL,
    expires_at timestamptz NOT NULL,
    mode text NOT NULL DEFAULT ''
  );

  ALTER TABLE webui_sessions
  ADD COLUMN IF NOT EXISTS mode text NOT NULL DEFAULT '';

  ALTER TABLE webui_sessions
  ADD COLUMN IF NOT EXISTS external_admin boolean NOT NULL DEFAULT false;

  CREATE TABLE IF NOT EXISTS webui_oidc_states (
    state_hash bytea PRIMARY KEY,
    verifier text NOT NULL,
    expires_at timestamptz NOT NULL
  );

  CREATE TABLE IF NOT EXISTS webui_identities (
    provider text NOT NULL,
    issuer text NOT NULL,
    subject text NOT NULL,
    username text NOT NULL REFERENCES webui_users (username) ON DELETE CASCADE,
    PRIMARY KEY (provider, issuer, subject)
  );

  ALTER TABLE webui_identities
  ADD COLUMN IF NOT EXISTS provider_username text NOT NULL DEFAULT '';

  ALTER TABLE webui_identities
  ADD COLUMN IF NOT EXISTS email text NOT NULL DEFAULT '';

  ALTER TABLE webui_identities
  ADD COLUMN IF NOT EXISTS display_name text NOT NULL DEFAULT '';

  ALTER TABLE webui_identities
  ADD COLUMN IF NOT EXISTS last_seen timestamptz;

  CREATE TABLE IF NOT EXISTS webui_retired_identities (
    provider text NOT NULL,
    issuer text NOT NULL,
    subject text NOT NULL,
    PRIMARY KEY (provider, issuer, subject)
  );

  CREATE TABLE IF NOT EXISTS webui_pending_identities (
    issuer text NOT NULL,
    subject text NOT NULL,
    username text NOT NULL,
    PRIMARY KEY (issuer, subject)
  );

  ALTER TABLE webui_pending_identities
  ADD COLUMN IF NOT EXISTS status text NOT NULL DEFAULT 'pending';

  CREATE TABLE IF NOT EXISTS webui_auth_settings (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    allow_registration boolean NOT NULL DEFAULT true
  );

  INSERT INTO
    webui_auth_settings (singleton)
  VALUES
    (true)
  ON CONFLICT DO NOTHING;

  CREATE TABLE IF NOT EXISTS webui_auth_migrations (name text PRIMARY KEY);

  WITH
    applied AS (
      INSERT INTO
        webui_auth_migrations (name)
      VALUES
        ('external_account_binding')
      ON CONFLICT DO NOTHING
      RETURNING
        name
    )
  DELETE FROM webui_sessions
  WHERE
    mode = 'oidc'
    AND EXISTS (
      SELECT
        1
      FROM
        applied
    );
