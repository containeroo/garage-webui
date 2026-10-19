CREATE TABLE webui_sessions (
 token_hash bytea PRIMARY KEY,
 credential bytea NOT NULL,
 expires_at timestamptz NOT NULL,
 theme text NOT NULL DEFAULT 'Light'
);
CREATE INDEX ON webui_sessions (expires_at);
CREATE TABLE webui_upload_keys (
 bucket_id text PRIMARY KEY,
 access_key_id text,
 read_access_key_id text,
 copy_access_key_id text
);
