# Garage Web UI

Garage Web UI is a web interface for managing [Garage](https://garagehq.deuxfleurs.fr/) buckets, access keys, users, and objects.

## Quick start

With Docker Compose, Go, and npm installed, run the local development stack from the repository root:

```sh
make run
```

Open [http://localhost:8080](http://localhost:8080) and create the first administrator. The development stack uses local-only credentials and is not a production deployment.

## Documentation

- [Configuration](configuration/index.md) — flags, environment variables, authentication, and object uploads.
- [Authentication](authentication/index.md) — local, trusted proxy, and OIDC sign-in and recovery.
- [Deployment](deployment/index.md) — run the local stack and understand production prerequisites.
- [Development](development/index.md) — build, test, format, and work on this site.
