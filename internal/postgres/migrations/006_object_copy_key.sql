-- A separate bucket-limited read/write key is used for server-side copies.
ALTER TABLE webui_upload_keys ADD COLUMN copy_access_key_id text UNIQUE;
