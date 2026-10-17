-- Use a single installation-wide signing key instead of a key per bucket.
-- Previous keys remain in Garage; administrators can revoke them after upgrade.
DROP TABLE IF EXISTS webui_upload_keys;

CREATE TABLE webui_upload_signer (
  singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
  access_key_id text NOT NULL
);
