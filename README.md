# Garage Web UI

Web interface for managing [Garage](https://garagehq.deuxfleurs.fr/) buckets, access keys, users, and objects.

## Quick Start

Run locally (Docker Compose, Go, and npm required):

```sh
make run
```

Open [http://localhost:8080](http://localhost:8080) and create the first administrator.

Or run the full stack in Docker:

```sh
docker compose -p garage-webui-dev -f dev/compose/compose.yaml up --build -d
```

For deployable Compose and Kubernetes examples, including persistent storage and
credential setup, see [Deployment](docs/content/deployment/index.md).

## Flags

| Flag                          | Description                                               |
| ----------------------------- | --------------------------------------------------------- |
| `--database-url`              | PostgreSQL connection URL (required)                      |
| `--garage-url`                | Garage Admin API URL (required)                           |
| `--garage-token`              | Garage Admin API token (required)                         |
| `--auth-mode`                 | `local`, `proxy`, or `oidc`                               |
| `--listen-address`            | Web UI listen address (default `127.0.0.1:8080`)          |
| `--management-listen-address` | Health endpoint listen address (default `127.0.0.1:8081`) |
| `--public-url`                | Public origin (default `http://localhost:8080`)           |
| `--route-prefix`              | Optional URL path prefix                                  |
| `--s3-public-url`             | Browser-accessible Garage S3 endpoint for objects         |
| `--s3-region`                 | S3 region (default `garage`)                              |
| `--local-login`               | Enable local recovery login with external authentication  |
| `--trusted-proxy-cidrs`       | Trusted proxy networks                                    |
| `--trusted-admin-group`       | Proxy administrator group                                 |
| `--oidc-issuer`               | OIDC issuer URL                                           |
| `--oidc-client-id`            | OIDC client ID                                            |
| `--oidc-client-secret`        | OIDC client secret                                        |
| `--oidc-admin-group`          | OIDC administrator group                                  |
| `--oidc-group-claim`          | OIDC group claim                                          |
| `--log-format`                | `json` or `text`                                          |
| `--debug`                     | Enable debug logging                                      |
| `--access-log`                | Enable HTTP access logging                                |

Flags are also available as environment variables prefixed with `GARAGE_WEBUI__`, for example `GARAGE_WEBUI__ROUTE_PREFIX=/garage`. Run `garage-webui --help` for the complete list.

## License

No license has been specified in the current repository snapshot.
