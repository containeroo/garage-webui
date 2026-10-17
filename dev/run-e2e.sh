#!/bin/sh
set -eu

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
temp_dir=$(mktemp -d "${TMPDIR:-/tmp}/garage-webui-e2e.XXXXXXXX")
project="garage-webui-e2e-$(basename "$temp_dir" | tr '[:upper:].' '[:lower:]-')"
compose_file="$root/dev/compose/compose.e2e.yaml"
local_pid='' proxy_pid='' untrusted_pid='' oidc_pid=''

# Compose needs valid values even if setup fails before ports are allocated.
export E2E_KEYCLOAK_PORT=1 E2E_KEYCLOAK_ORIGIN=http://127.0.0.1:1
export E2E_OIDC_REDIRECT_URI=http://127.0.0.1:1/oidc-e2e/auth/callback

compose() {
  docker compose --project-name "$project" --file "$compose_file" "$@"
}

cleanup() {
  status=$?
  trap - EXIT HUP INT TERM
  for pid in "$local_pid" "$proxy_pid" "$untrusted_pid" "$oidc_pid"; do
    if [ -n "$pid" ]; then kill "$pid" 2>/dev/null || true; fi
  done
  for pid in "$local_pid" "$proxy_pid" "$untrusted_pid" "$oidc_pid"; do
    if [ -n "$pid" ]; then wait "$pid" 2>/dev/null || true; fi
  done
  if [ "$status" -ne 0 ]; then
    for log in "$temp_dir"/*.log; do
      if [ -f "$log" ]; then
        printf '\n--- %s ---\n' "$(basename "$log")" >&2
        tail -n 50 "$log" >&2
      fi
    done
  fi
  if ! compose down --volumes --remove-orphans; then
    printf 'Failed to clean up E2E Docker project %s\n' "$project" >&2
    status=1
  fi
  rm -r -- "$temp_dir"
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' HUP TERM

build_app() {
  printf 'Building frontend and installing Chromium...\n'
  make frontend
  if [ -z "${E2E_CHROMIUM_PATH:-}" ]; then
    (cd web && npx playwright install chromium)
  fi
  go build -o "$temp_dir/garage-webui" ./cmd/garage-webui
}

allocate_ports() {
  # Hold all sockets until allocation is complete to avoid duplicate ports.
  ports=$(node <<'NODE'
const net = require("node:net");
(async () => {
  const servers = [];
  for (let i = 0; i < 10; i++) {
    const server = net.createServer();
    await new Promise((resolve, reject) => {
      server.once("error", reject);
      server.listen(0, "127.0.0.1", resolve);
    });
    servers.push(server);
  }
  console.log(servers.map((server) => server.address().port).join(" "));
  await Promise.all(servers.map((server) => new Promise((resolve) => server.close(resolve))));
})().catch((error) => { console.error(error); process.exitCode = 1; });
NODE
  )
  # Split the space-separated numeric ports returned by Node into arguments.
  # shellcheck disable=SC2086
  set -- $ports
  local_port=$1 local_management_port=$2 proxy_port=$3 proxy_front_port=$4 untrusted_port=$5
  oidc_port=$6 E2E_KEYCLOAK_PORT=$7 proxy_management_port=$8 untrusted_management_port=$9
  shift 9
  oidc_management_port=$1
  export E2E_KEYCLOAK_PORT E2E_KEYCLOAK_ORIGIN="http://127.0.0.1:$E2E_KEYCLOAK_PORT"
  export E2E_OIDC_REDIRECT_URI="http://127.0.0.1:$oidc_port/oidc-e2e/auth/callback"
}

start_dependencies() {
  printf 'Starting disposable services (%s)...\n' "$project"
  compose up -d --wait garage postgres
  compose run --rm --no-deps garage-ready
  compose up -d keycloak

  garage_port=$(compose port garage 3903)
  s3_port=$(compose port garage 3900)
  database_port=$(compose port postgres 5432)
  garage_port=${garage_port##*:}
  s3_port=${s3_port##*:}
  database_port=${database_port##*:}
  compose exec -T postgres createdb -U garage_webui garage_webui_proxy_e2e
  compose exec -T postgres createdb -U garage_webui garage_webui_oidc_e2e

  export GARAGE_WEBUI__GARAGE_URL="http://127.0.0.1:$garage_port"
  export GARAGE_WEBUI__GARAGE_TOKEN=local-garage-admin-token
  database_base="postgres://garage_webui:local-database-password@127.0.0.1:$database_port"
}

wait_url() {
  url=$1
  pid=${2:-}
  attempt=0
  while [ "$attempt" -lt 120 ]; do
    if curl --silent --fail "$url" >/dev/null; then return 0; fi
    if [ -n "$pid" ] && ! kill -0 "$pid" 2>/dev/null; then
      printf 'E2E service exited before becoming ready: %s\n' "$url" >&2
      return 1
    fi
    attempt=$((attempt + 1))
    sleep 1
  done
  printf 'Timed out waiting for %s\n' "$url" >&2
  compose logs keycloak >&2 || true
  return 1
}

start_local() {
  GARAGE_WEBUI__DATABASE_URL="$database_base/garage_webui?sslmode=disable" \
  GARAGE_WEBUI__AUTH_MODE=local \
  GARAGE_WEBUI__PUBLIC_URL="http://127.0.0.1:$local_port" \
  GARAGE_WEBUI__S3_PUBLIC_URL="http://127.0.0.1:$s3_port" \
  GARAGE_WEBUI__ROUTE_PREFIX=/e2e \
    "$temp_dir/garage-webui" --listen-address "127.0.0.1:$local_port" \
      --management-listen-address "127.0.0.1:$local_management_port" >"$temp_dir/local.log" 2>&1 &
  local_pid=$!
  wait_url "http://127.0.0.1:$local_port/e2e/api/session" "$local_pid"
}

start_proxy() {
  export GARAGE_WEBUI__DATABASE_URL="$database_base/garage_webui_proxy_e2e?sslmode=disable"
  GARAGE_WEBUI__AUTH_MODE=proxy GARAGE_WEBUI__ALLOW_USER_REGISTRATION=true \
  GARAGE_WEBUI__TRUSTED_ADMIN_GROUP=garage-admins GARAGE_WEBUI__PUBLIC_URL="http://127.0.0.1:$proxy_front_port" \
  GARAGE_WEBUI__ROUTE_PREFIX=/proxy-e2e GARAGE_WEBUI__TRUSTED_PROXY_CIDRS=127.0.0.1/32 \
    "$temp_dir/garage-webui" --listen-address "127.0.0.1:$proxy_port" \
      --management-listen-address "127.0.0.1:$proxy_management_port" >"$temp_dir/proxy.log" 2>&1 &
  proxy_pid=$!
  wait_url "http://127.0.0.1:$proxy_port/proxy-e2e/api/session" "$proxy_pid"

  GARAGE_WEBUI__AUTH_MODE=proxy GARAGE_WEBUI__ALLOW_USER_REGISTRATION=true \
  GARAGE_WEBUI__TRUSTED_ADMIN_GROUP=garage-admins GARAGE_WEBUI__PUBLIC_URL="http://127.0.0.1:$proxy_front_port" \
  GARAGE_WEBUI__ROUTE_PREFIX=/proxy-e2e GARAGE_WEBUI__TRUSTED_PROXY_CIDRS=192.0.2.0/24 \
    "$temp_dir/garage-webui" --listen-address "127.0.0.1:$untrusted_port" \
      --management-listen-address "127.0.0.1:$untrusted_management_port" >"$temp_dir/untrusted.log" 2>&1 &
  untrusted_pid=$!
  wait_url "http://127.0.0.1:$untrusted_port/proxy-e2e/api/session" "$untrusted_pid"
}

start_oidc() {
  wait_url "$E2E_KEYCLOAK_ORIGIN/realms/garage-webui/.well-known/openid-configuration"
  GARAGE_WEBUI__DATABASE_URL="$database_base/garage_webui_oidc_e2e?sslmode=disable" \
  GARAGE_WEBUI__AUTH_MODE=oidc GARAGE_WEBUI__ALLOW_USER_REGISTRATION=true \
  GARAGE_WEBUI__PUBLIC_URL="http://127.0.0.1:$oidc_port" GARAGE_WEBUI__ROUTE_PREFIX=/oidc-e2e \
  GARAGE_WEBUI__OIDC_ISSUER="$E2E_KEYCLOAK_ORIGIN/realms/garage-webui" \
  GARAGE_WEBUI__OIDC_CLIENT_ID=garage-webui-test GARAGE_WEBUI__OIDC_CLIENT_SECRET=local-test-client-secret \
  GARAGE_WEBUI__OIDC_ADMIN_GROUP=garage-admins \
    "$temp_dir/garage-webui" --listen-address "127.0.0.1:$oidc_port" \
      --management-listen-address "127.0.0.1:$oidc_management_port" >"$temp_dir/oidc.log" 2>&1 &
  oidc_pid=$!
  wait_url "http://127.0.0.1:$oidc_port/oidc-e2e/api/session" "$oidc_pid"
}

run_tests() {
  printf 'Running local, proxy and OIDC browser tests...\n'
  cd "$root/web"
  E2E_BASE_URL="http://127.0.0.1:$local_port/e2e/" \
  E2E_PROXY_URL="http://127.0.0.1:$proxy_front_port/" \
  E2E_PROXY_UPSTREAM="http://127.0.0.1:$proxy_port/" \
  E2E_PROXY_UNTRUSTED_UPSTREAM="http://127.0.0.1:$untrusted_port/" \
  E2E_OIDC_URL="http://127.0.0.1:$oidc_port/oidc-e2e/" \
  E2E_LARGE_UPLOAD="${E2E_LARGE_UPLOAD:-1}" npm run test:e2e
}

cd "$root"
build_app
allocate_ports
start_dependencies
start_local
start_proxy
start_oidc
run_tests
