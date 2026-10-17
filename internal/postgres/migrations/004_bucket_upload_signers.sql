-- Return to one signing key per bucket, without retaining privileged shared keys.
DROP TABLE IF EXISTS webui_upload_signer;

CREATE TABLE IF NOT EXISTS webui_upload_keys (
  bucket_id text PRIMARY KEY,
  access_key_id text NOT NULL UNIQUE
);
