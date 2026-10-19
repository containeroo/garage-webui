# Configuration

Flags are also available as environment variables prefixed with `GARAGE_WEBUI__`, with hyphens replaced by underscores. For example, `--route-prefix` becomes `GARAGE_WEBUI__ROUTE_PREFIX`. Run `garage-webui --help` for the complete list.

| Flag | Description |
| --- | --- |
| `--database-url` | PostgreSQL connection URL or SQLite file URL (required) |
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

`--s3-public-url` is the browser-accessible S3 API endpoint, not the Garage Admin API. With PostgreSQL, object operations use session advisory locks to coordinate signing keys across replicas. Connect directly to PostgreSQL or use session pooling; transaction and statement pooling are not supported.

## SQLite

Use `--database-url sqlite:///data/garage-webui.db` (or `GARAGE_WEBUI__DATABASE_URL=sqlite:///data/garage-webui.db`) to use SQLite instead of PostgreSQL. Relative paths such as `sqlite:./data/garage-webui.db` also work. The database and schema are created automatically. No PostgreSQL server is needed.

SQLite supports one Web UI process per database on a local filesystem. The application holds a process lock and refuses a second instance using the same file. Mount the containing directory as persistent storage, writable by the container user (UID 65532). Keep the directory private: it contains encrypted sessions and signing-key references. The session encryption secret remains required and must stay stable across restarts.

PostgreSQL is the option for multiple replicas. Connection-pool settings apply only to PostgreSQL and are rejected with SQLite. Switching database URLs does not transfer sessions or signing-key references between backends.

SQLite uses WAL mode. Stop the application before copying its database for backup, or use a SQLite-aware online backup tool; copying only the main file while running can omit pending writes. Persist the entire directory, including WAL files. Do not share the SQLite directory between pods or place it on a network filesystem.
