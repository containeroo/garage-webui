CREATE TABLE webui_deletion_jobs (
  id TEXT PRIMARY KEY NOT NULL,
  owner_hash BLOB NOT NULL,
  credential BLOB,
  bucket TEXT NOT NULL,
  targets TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  deleted INTEGER NOT NULL DEFAULT 0,
  error TEXT NOT NULL DEFAULT '',
  cancel_requested INTEGER NOT NULL DEFAULT 0,
  worker TEXT NOT NULL DEFAULT '',
  lease_until INTEGER,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);
CREATE INDEX webui_deletion_owner_idx
  ON webui_deletion_jobs (owner_hash, created_at DESC);

CREATE INDEX webui_deletion_queue_idx
  ON webui_deletion_jobs (status, lease_until);
