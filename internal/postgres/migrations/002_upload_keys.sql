-- Remember only the Garage key ID. Secret material stays in Garage and is fetched
-- over the authenticated Admin API when an authorized upload is requested.
CREATE TABLE webui_upload_keys (
  bucket_id text PRIMARY KEY,
  access_key_id text NOT NULL
);
