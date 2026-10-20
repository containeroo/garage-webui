# Garage Web UI

Web interface for managing [Garage](https://garagehq.deuxfleurs.fr/) buckets, access keys, admin tokens, and objects.

![Garage Web UI overview showing cluster health and bucket storage](docs/content/assets/screenshots/overview.png)

## Quick Start

Run locally (Docker Compose, Go, and npm required):

```sh
make run
```

Open [http://localhost:8080](http://localhost:8080) and sign in with the development admin token `local-garage-admin-token`.

Or run the full stack in Docker:

```sh
docker compose -p garage-webui-dev -f dev/compose/compose.yaml up --build -d
```

For deployable Compose and Kubernetes examples, including persistent storage and credential setup, see [Deployment](docs/content/deployment/index.md).

## Flags

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

Flags are also available as environment variables prefixed with `GARAGE_WEBUI__`, for example `GARAGE_WEBUI__ROUTE_PREFIX=/garage`. Run `garage-webui --help` for the complete list.

For a single instance, use `GARAGE_WEBUI__DATABASE_URL=sqlite:///data/garage-webui.db` with a persistent, writable `/data` directory. PostgreSQL remains available for multiple replicas. See [SQLite configuration](docs/content/configuration/index.md#sqlite) for details.

## Authentication

Sign in with a Garage admin token for administration, or an S3 access key and secret for bucket access. Garage controls permissions. Admins can create, edit, and revoke named admin tokens according to their scopes. PostgreSQL or SQLite stores revocable sessions with encrypted credentials.

## License

No license has been specified in the current repository snapshot.
