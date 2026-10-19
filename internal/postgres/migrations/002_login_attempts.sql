-- Share login throttling across Web UI replicas without storing client addresses.
CREATE TABLE webui_login_attempts (
    peer_hash bytea NOT NULL,
    window_start timestamptz NOT NULL,
    attempts integer NOT NULL CHECK (attempts > 0),
    PRIMARY KEY (peer_hash, window_start)
);
CREATE INDEX webui_login_attempts_window_idx ON webui_login_attempts (window_start);
