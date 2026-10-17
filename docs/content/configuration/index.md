# Configuration

Flags are also available as environment variables prefixed with `GARAGE_WEBUI__`, with hyphens replaced by underscores. For example, `--route-prefix` becomes `GARAGE_WEBUI__ROUTE_PREFIX`. Run `garage-webui --help` for the complete list.

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

## Authentication

Local mode serves the login route. With proxy or OIDC authentication, `--local-login` enables recovery sign-in at `/login` under the configured route prefix; the account must have an enabled local credential. Restrict trusted proxy networks to your actual proxies, and protect credentials and tokens as secrets.

See the [authentication guide](../authentication/index.md) for setup examples, trusted headers, OIDC callbacks, account registration, and recovery.

## Object access

`--s3-public-url` is the browser-accessible S3 API endpoint, not the Garage Admin API. Object operations use PostgreSQL session advisory locks to coordinate signing keys across replicas. Connect directly to PostgreSQL or use session pooling; transaction and statement pooling are not supported.
