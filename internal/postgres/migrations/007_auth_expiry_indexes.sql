CREATE INDEX IF NOT EXISTS webui_sessions_expires_at_idx ON webui_sessions (expires_at);
CREATE INDEX IF NOT EXISTS webui_oidc_states_expires_at_idx ON webui_oidc_states (expires_at);
