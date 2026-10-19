# Development

From the repository root:

```sh
make build
make test
make fmt
make check-format
```

Development requires Node.js 24 or newer for the frontend build and tests. `make build` installs frontend dependencies and builds the frontend and Go binary. `make test` builds the frontend, runs frontend and Go tests, and runs `go vet`. The formatting targets are `fmt-go`, `fmt-web`, and `fmt-md`; install frontend dependencies with `npm --prefix web ci` before running the frontend or Markdown formatter.

PostgreSQL integration tests run when `GARAGE_WEBUI_TEST_DATABASE_URL` is set; otherwise they are skipped. Use a dedicated test database whose user can create and drop schemas. For the local development database started by `make dev-up`:

```sh
GARAGE_WEBUI_TEST_DATABASE_URL='postgres://garage_webui:local-database-password@localhost:5432/garage_webui?sslmode=disable' make test
```

CI supplies a PostgreSQL service and runs these integration tests automatically.

## Browser tests

Run all local-authentication, trusted-proxy and Keycloak OIDC Playwright tests with:

```sh
make test-e2e
```

This requires Docker, Go, Node.js 24 or newer, and `curl`. It builds the frontend and application, installs Playwright's Chromium, starts a fresh Docker Compose project with its own Garage and PostgreSQL volumes and a disposable Keycloak instance, and runs all browser tests. It stops its application processes and removes that project's containers, networks and volumes on success, failure or interruption. It does not use or delete the development stack. Playwright retains traces on failure, which can include application data.

The suite includes 257 MiB multipart completion and cancellation tests; set `E2E_LARGE_UPLOAD=0` to skip them. Services use automatically assigned loopback ports. To use an already-installed Chromium binary, set `E2E_CHROMIUM_PATH` to its absolute path. CI runs the same target.

To run against an independently managed **fresh, isolated** application instead, set `E2E_BASE_URL` to its URL including the route prefix and run `make test-e2e-existing`. This lower-level target does not start or clean up services. The tests create accounts, a bucket and signing credentials; never point them at production or shared development data.

To run the proxy tests separately, start an isolated proxy-mode instance with registration enabled, trusted proxy CIDR `127.0.0.1/32`, admin group `garage-admins`, public URL `http://127.0.0.1:18083` and route prefix `/proxy-e2e`. Start another instance against the same isolated database with a CIDR that excludes loopback, and then run:

```sh
cd web
E2E_BASE_URL=http://localhost:8080/e2e/ \
E2E_PROXY_URL=http://127.0.0.1:18083/ \
E2E_PROXY_UPSTREAM=http://127.0.0.1:18082/ \
E2E_PROXY_UNTRUSTED_UPSTREAM=http://127.0.0.1:18084/ \
npx playwright test e2e/proxy.spec.js
```

The fixture injects test identities and must only be used on loopback; it is not an authentication proxy suitable for deployment.

### Keycloak OIDC tests

`make oidc-provider-up` starts a disposable Keycloak container on `127.0.0.1:18090` with a preconfigured test realm. Its data is ephemeral; `make oidc-provider-down` removes the container and its test data. The realm has `oidc-admin` and `oidc-viewer` accounts, both with password `local-test-password`. These credentials and development-mode settings must never be used in production.

For manual testing or the OIDC browser suite, start Garage and PostgreSQL, create a fresh dedicated database, and run the Web UI with the following settings (the URLs match the test realm's registered callback):

```sh
make dev-up frontend oidc-provider-up
docker exec garage-webui-dev-postgres-1 createdb -U garage_webui garage_webui_oidc_e2e
GARAGE_WEBUI__DATABASE_URL='postgres://garage_webui:local-database-password@localhost:5432/garage_webui_oidc_e2e?sslmode=disable' \
GARAGE_WEBUI__GARAGE_URL='http://localhost:3903' \
GARAGE_WEBUI__GARAGE_TOKEN='local-garage-admin-token' \
GARAGE_WEBUI__AUTH_MODE='oidc' GARAGE_WEBUI__ALLOW_USER_REGISTRATION='true' \
GARAGE_WEBUI__PUBLIC_URL='http://127.0.0.1:18086' GARAGE_WEBUI__ROUTE_PREFIX='/oidc-e2e' \
GARAGE_WEBUI__OIDC_ISSUER='http://127.0.0.1:18090/realms/garage-webui' \
GARAGE_WEBUI__OIDC_CLIENT_ID='garage-webui-test' GARAGE_WEBUI__OIDC_CLIENT_SECRET='local-test-client-secret' \
GARAGE_WEBUI__OIDC_ADMIN_GROUP='garage-admins' \
go run ./cmd/garage-webui --listen-address 127.0.0.1:18086 --management-listen-address 127.0.0.1:18186
```

Wait for Keycloak's discovery endpoint to become available before starting the Web UI. In another terminal, open `http://127.0.0.1:18086/oidc-e2e/` or run:

```sh
cd web
E2E_BASE_URL=http://localhost:8080/e2e/ E2E_OIDC_URL=http://127.0.0.1:18086/oidc-e2e/ npx playwright test e2e/oidc.spec.js
```

Stop the application before removing the dedicated test database. Do not point these tests at a shared database.

## Documentation screenshots

Run `make screenshots` to rebuild the screenshots used by the README and documentation. It starts a disposable Garage and PostgreSQL stack, captures the interface with Playwright, and removes the stack and its data afterward. Docker, Go, Node.js 24 or newer, and `curl` are required. Set `E2E_CHROMIUM_PATH` to use an existing Chromium executable.

## Documentation site

The site uses the pinned [Kumbuka CLI](https://github.com/kumbuka-me/cli). Build it with:

```sh
make site
make site-serve
```

`make site-serve` rebuilds the site for a local URL, serves it, and opens a browser. Edit Markdown under `docs/content` and site settings in `docs/site.toml`. Generated output is written to `docs/site`. GitHub Actions builds pull requests and deploys changes on `main` to GitHub Pages. Run `make help` to see available development and tagging targets.

Run `make favicon` after changing `web/public/logo.svg` to regenerate the Web UI and documentation icons. This requires `rsvg-convert` from librsvg.
