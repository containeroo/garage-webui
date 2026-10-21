CREATE TABLE webui_deletion_jobs (
  id text PRIMARY KEY,
  owner_hash bytea NOT NULL,
  credential bytea,
  bucket text NOT NULL,
  targets text NOT NULL,
  status text NOT NULL DEFAULT 'pending',
  deleted bigint NOT NULL DEFAULT 0,
  error text NOT NULL DEFAULT '',
  cancel_requested boolean NOT NULL DEFAULT false,
  worker text NOT NULL DEFAULT '',
  lease_until timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX webui_deletion_owner_idx
  ON webui_deletion_jobs (owner_hash, created_at DESC);

CREATE INDEX webui_deletion_queue_idx
  ON webui_deletion_jobs (status, lease_until);
