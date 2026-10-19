CREATE TABLE webui_sessions (
    token_hash BLOB PRIMARY KEY NOT NULL,
    credential BLOB NOT NULL,
    expires_at INTEGER NOT NULL,
    theme TEXT NOT NULL DEFAULT 'Light'
);
CREATE INDEX webui_sessions_expiry_idx ON webui_sessions (expires_at);
CREATE TABLE webui_upload_keys (
    bucket_id TEXT PRIMARY KEY NOT NULL,
    access_key_id TEXT,
    read_access_key_id TEXT,
    copy_access_key_id TEXT
);
CREATE TABLE webui_login_attempts (
    peer_hash BLOB NOT NULL,
    window_start INTEGER NOT NULL,
    attempts INTEGER NOT NULL CHECK (attempts > 0),
    PRIMARY KEY (peer_hash, window_start)
);
CREATE INDEX webui_login_attempts_window_idx ON webui_login_attempts (window_start);
