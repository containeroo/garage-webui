# Development

Run Garage and PostgreSQL with the Web UI locally:

```sh
make run
```

Sign in with admin token `local-garage-admin-token`. Development credentials are only for the disposable local stack.

## Checks

```sh
make test
make test-e2e
```

The E2E target builds the application and starts isolated Garage and PostgreSQL containers. Playwright exercises admin-token creation, editing, revocation, scoped permissions, S3 bucket restrictions, read-only enforcement, and object operations. Cleanup removes only the disposable project and volumes. To include large multipart transfers, use `E2E_LARGE_UPLOAD=1 make test-e2e`.

PostgreSQL integration tests require `GARAGE_WEBUI_TEST_DATABASE_URL`. Set it to a test database and run `go test -count=1 ./...` to bypass the test cache. Tests use isolated schemas. CI provides the database automatically.

## Screenshots

```sh
make screenshots
```

This target starts the same isolated stack and captures the real UI with Playwright. Screenshots are written to `docs/content/assets/screenshots` and referenced by the README and documentation.
