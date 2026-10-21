# Authentication

Sign in with credentials managed by Garage. The Web UI does not create application accounts, local passwords, groups, identity links, or registration policies.

## Admin tokens

Choose **Admin token** and enter a Garage administration API token. The Web UI validates it with `GetCurrentAdminTokenInfo` and checks its current scope and validity on each authenticated request. Each admin API request uses the signed-in token. There is no shared privileged token in the Web UI deployment.

Create individual named tokens in **Admin tokens**. The screen supports creation, name and scope changes, optional expiry, and revocation, according to the calling token's scopes. New secrets are shown once. Save them before dismissing the dialog. Garage documents `CreateAdminToken` and `UpdateAdminToken` as effectively full administration privileges because they can grant new permissions.

A token may also be bootstrapped with the Garage CLI:

```sh
garage admin-token create --scope '*' alice
```

Garage's master admin token can be used to bootstrap named tokens. Keep one named token per administrator for independent rotation and revocation. Token names identify credentials rather than verified people.

## S3 credentials

Choose **S3 credentials**, then enter an access key ID and secret access key. Authentication uses a signed S3 ListBuckets request. The UI shows the buckets returned for that key and object browsing, with no cluster, access-key, or admin-token management.

Object requests are signed with the supplied key. Garage enforces read/write permissions on every S3 operation, including short-lived signed URLs. A read-only key cannot upload or delete objects even if it requests an upload URL. Revoking a key or changing its permissions takes effect in Garage immediately.

Configure `--s3-url` for S3 sign-in, browsing, and streamed object transfers. It falls back to `--s3-public-url` when unset. Normal operations pass through the Go backend using the signed-in credentials and do not require browser CORS. Admin sessions provision bucket-scoped keys; S3 sessions use their supplied key and never expand permissions.

`--s3-public-url` enables the optional **Upload directly to S3** and **Download directly from S3** actions. Direct uploads bypass the Web UI and require bucket CORS allowing the Web UI origin, required methods and headers, and an exposed ETag for multipart uploads. Admin sessions prepare CORS only for these optional direct transfers; S3-key sessions require CORS to be configured by a bucket administrator.

## Sessions

The configured database stores encrypted credentials and session expiry. Generate a session encryption key:

```sh
openssl rand -base64 32
```

Set it as `GARAGE_WEBUI__SESSION_SECRET` on every replica. Treat the key and database as secrets. Changing the key invalidates existing sessions. Use a fresh database with this refactor; there is no migration from the former application-account schema.

Sessions expire after eight hours. Browser cookies contain only a random session identifier and use HttpOnly, SameSite=Lax, a deployment-relative path, and Secure on HTTPS deployments. Logout revokes the session across replicas. The credential is verified again for authenticated requests; expired or revoked Garage tokens lose access.

Sign-in attempts are limited per immediate network peer on each process. Use ingress rate limiting where clients share a proxy. Control API mutations require JSON and an Origin matching `--public-url`. Streamed file writes accept raw bodies and enforce the same origin check.

## Object access with scoped admin tokens

An admin token used for object operations needs `GetBucketInfo`, `GetKeyInfo`, `CreateKey`, `AllowBucketKey`, `DenyBucketKey`. Optional direct transfers additionally require `UpdateBucket` for CORS provisioning. The Web UI checks these scopes before using managed signing keys. Bucket deletion also needs `DeleteBucket` and `DeleteKey` so the Web UI can revoke its managed keys. Direct S3 sign-in uses the key's own Garage permissions instead.

Login rate limits are persisted in the configured database (shared across replicas with PostgreSQL), using a hash of the connecting network peer. The Web UI does not trust unconfigured forwarded-IP headers. Behind a reverse proxy, enable appropriate ingress rate limiting so clients sharing its source address do not share the application's allowance.
