-- Separate read-only keys are used for listing and downloading objects.
ALTER TABLE webui_upload_keys
  ALTER COLUMN access_key_id DROP NOT NULL,
  ADD COLUMN read_access_key_id text UNIQUE;
