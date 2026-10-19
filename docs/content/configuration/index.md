# Configuration

Flags are also available as environment variables prefixed with `GARAGE_WEBUI__`, with hyphens replaced by underscores. For example, `--route-prefix` becomes `GARAGE_WEBUI__ROUTE_PREFIX`. Run `garage-webui --help` for the complete list.

| Flag | Description |
| --- | --- |
| `--database-url` | PostgreSQL connection URL (required) |
| `--session-secret` | Base64-encoded 32-byte key encrypting credential sessions (required) |
| `--garage-url` | Garage Admin API URL (required) |
| `--listen-address` | Web UI listen address (default `127.0.0.1:8080`) |
| `--management-listen-address` | Health endpoint listen address (default `127.0.0.1:8081`) |
| `--public-url` | Public origin (default `http://localhost:8080`) |
| `--route-prefix` | Optional URL path prefix |
| `--s3-public-url` | Browser-accessible Garage S3 endpoint for objects |
| `--s3-region` | S3 region (default `garage`) |
| `--log-format` | `json` or `text` |
| `--debug` | Enable debug logging |
| `--access-log` | Enable HTTP access logging |

## Authentication

Sign in using a Garage admin token or S3 access key and secret. Configure `--session-secret` using `openssl rand -base64 32` and keep the same key on all replicas. The Web UI has no shared Garage admin token or local account database. See the [authentication guide](../authentication/index.md).

## Object access

`--s3-public-url` is the browser-accessible S3 API endpoint, not the Garage Admin API. Object operations use PostgreSQL session advisory locks to coordinate signing keys across replicas. Connect directly to PostgreSQL or use session pooling; transaction and statement pooling are not supported.
