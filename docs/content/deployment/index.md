# Deployment

## Local development

Run Garage and PostgreSQL in Docker, with the Web UI running locally:

```sh
make run
```

To serve it under `/garage`:

```sh
make run ROUTE_PREFIX=/garage
```

Alternatively run the full development stack in Docker:

```sh
docker compose -p garage-webui-dev -f dev/compose/compose.yaml up --build -d
```

Open `http://localhost:8080` and sign in with the Garage admin token. Use `local-garage-admin-token` for the development stack. The Compose file contains development-only credentials; do not use them in production.

## Deployment considerations

This refactor requires a fresh Web UI database and the new `session-secret`. Previous Web UI account schemas and authentication flags are unsupported. Garage data is unaffected.

The application requires PostgreSQL, a Garage Admin API URL, and a session encryption key. Set a public origin and, for browser object access, a browser-reachable S3 API endpoint. Keep the database credentials, session encryption key private. For replica coordination, use a direct PostgreSQL connection or session pooling, not transaction or statement pooling. See [Configuration](../configuration/index.md) for the available settings.

## Docker Compose

The example in `deploy/compose.yaml` starts Garage, PostgreSQL and the Web UI with persistent named volumes. Create `deploy/.env` with unique secrets before starting it (the generated PostgreSQL password is hexadecimal so it is safe in the connection URL):

```sh
umask 077
printf 'GARAGE_ADMIN_TOKEN=%s\nGARAGE_RPC_SECRET=%s\nPOSTGRES_PASSWORD=%s\nSESSION_SECRET=%s\n' \
  "$(openssl rand -hex 32)" "$(openssl rand -hex 32)" "$(openssl rand -hex 32)" "$(openssl rand -base64 32)" > deploy/.env
docker compose --env-file deploy/.env -f deploy/compose.yaml up -d
```

Open `http://localhost:8080` and sign in with the Garage admin token. The UI and S3 ports bind to loopback by default. For remote access, configure `WEBUI_BIND_ADDRESS`, `S3_BIND_ADDRESS`, `PUBLIC_URL` and `S3_PUBLIC_URL` in `deploy/.env`; the S3 URL must be reachable by the user's browser. The host ports can also be changed with `WEBUI_PORT` and `S3_PORT`; update the public URLs to match. Use HTTPS through a reverse proxy for remote access. The Admin API and PostgreSQL are not published to the host. Set `GARAGE_WEBUI_IMAGE` to a specific release tag when deploying. Keep the volumes and secrets backed up; changing the PostgreSQL password in the environment does not change an existing database password.

## Kubernetes

The Kustomize manifests in `deploy/kubernetes` provide single-node Garage and PostgreSQL StatefulSets, persistent volume claims, and a Web UI Deployment. They require a default StorageClass (or a storage class specified on the claims). Create the secrets in the target namespace before applying the manifests:

```sh
kubectl create secret generic garage-webui-secrets \
  --from-literal=garage-admin-token="$(openssl rand -hex 32)" \
  --from-literal=garage-rpc-secret="$(openssl rand -hex 32)" \
  --from-literal=session-secret="$(openssl rand -base64 32)" \
  --from-literal=postgres-password="$(openssl rand -hex 32)"
kubectl apply -k deploy/kubernetes
kubectl rollout status statefulset/garage
kubectl rollout status statefulset/garage-postgres
kubectl rollout status deployment/garage-webui
```

Use `-n <namespace>` consistently if deploying outside the current namespace. The PostgreSQL password must be URL-safe because the Web UI includes it in a connection URL. For a local test, forward both services in separate terminals:

```sh
kubectl port-forward service/garage-webui 8080:8080
kubectl port-forward service/garage 3900:3900
```

Open `http://localhost:8080` and sign in with the Garage admin token. For remote access, expose the Web UI and Garage S3 services through your ingress or gateway, and update `GARAGE_WEBUI__PUBLIC_URL` and `GARAGE_WEBUI__S3_PUBLIC_URL` in `webui.yaml` to their browser-accessible HTTPS origins. Do not expose the Garage Admin API or PostgreSQL publicly. Pin the Web UI image to a release, size the claims for your workload, and configure backups. These single-node examples are starting points; they do not provide Garage or PostgreSQL high availability.

For buckets used with multipart uploads or copies, configure an abort-incomplete-multipart-upload lifecycle rule supported by the Garage deployment. The browser attempts to abort failed transfers, but cannot guarantee cleanup after a lost connection or a closed tab. The Web UI reports when an attempted abort fails; an interrupted browser may be unable to report it.
