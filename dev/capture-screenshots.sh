#!/bin/sh
set -eu

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
temp_dir=$(mktemp -d "${TMPDIR:-/tmp}/garage-webui-screenshots.XXXXXXXX")
project="garage-webui-screenshots-$(basename "$temp_dir" | tr '[:upper:].' '[:lower:]-')"
compose_file="$root/dev/compose/compose.e2e.yaml"
app_pid=''
export E2E_KEYCLOAK_PORT=1 E2E_KEYCLOAK_ORIGIN=http://127.0.0.1:1
export E2E_OIDC_REDIRECT_URI=http://127.0.0.1:1/callback

compose() {
  docker compose --project-name "$project" --file "$compose_file" "$@"
}

cleanup() {
  status=$?
  trap - EXIT HUP INT TERM
  if [ -n "$app_pid" ]; then
    kill "$app_pid" 2>/dev/null || true
    wait "$app_pid" 2>/dev/null || true
  fi
  if [ "$status" -ne 0 ] && [ -f "$temp_dir/app.log" ]; then
    tail -n 50 "$temp_dir/app.log" >&2
  fi
  if ! compose down --volumes --remove-orphans; then
    printf 'Failed to clean up screenshot project %s\n' "$project" >&2
    status=1
  fi
  rm -r -- "$temp_dir"
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' HUP TERM

cd "$root"
make frontend
if [ -z "${E2E_CHROMIUM_PATH:-}" ]; then
  (cd web && npx playwright install chromium)
fi
go build -o "$temp_dir/garage-webui" ./cmd/garage-webui

ports=$(node <<'NODE'
const net = require('node:net');
(async () => {
  const servers = await Promise.all(Array.from({ length: 2 }, () => new Promise((resolve, reject) => {
    const server = net.createServer().once('error', reject);
    server.listen(0, '127.0.0.1', () => resolve(server));
  })));
  console.log(servers.map(server => server.address().port).join(' '));
  await Promise.all(servers.map(server => new Promise(resolve => server.close(resolve))));
})().catch(error => { console.error(error); process.exitCode = 1; });
NODE
)
# shellcheck disable=SC2086
set -- $ports
app_port=$1
management_port=$2

compose up -d --wait garage postgres
compose run --rm --no-deps garage-ready
garage_port=$(compose port garage 3903)
s3_port=$(compose port garage 3900)
db_port=$(compose port postgres 5432)
GARAGE_WEBUI__DATABASE_URL="postgres://garage_webui:local-database-password@127.0.0.1:${db_port##*:}/garage_webui?sslmode=disable" \
GARAGE_WEBUI__GARAGE_URL="http://127.0.0.1:${garage_port##*:}" \
GARAGE_WEBUI__GARAGE_TOKEN=local-garage-admin-token \
GARAGE_WEBUI__AUTH_MODE=local GARAGE_WEBUI__PUBLIC_URL="http://127.0.0.1:$app_port" \
GARAGE_WEBUI__S3_PUBLIC_URL="http://127.0.0.1:${s3_port##*:}" \
  "$temp_dir/garage-webui" --listen-address "127.0.0.1:$app_port" \
  --management-listen-address "127.0.0.1:$management_port" >"$temp_dir/app.log" 2>&1 &
app_pid=$!

attempt=0
until curl --silent --fail "http://127.0.0.1:$app_port/api/session" >/dev/null; do
  if ! kill -0 "$app_pid" 2>/dev/null || [ "$attempt" -ge 120 ]; then
    printf 'Screenshot application did not become ready\n' >&2
    exit 1
  fi
  attempt=$((attempt + 1))
  sleep 1
done

mkdir -p "$temp_dir/screenshots"
SCREENSHOT_URL="http://127.0.0.1:$app_port/" SCREENSHOT_DIR="$temp_dir/screenshots" \
  node "$root/web/capture-screenshots.mjs"
mkdir -p "$root/docs/content/assets/screenshots"
cp "$temp_dir/screenshots/"*.png "$root/docs/content/assets/screenshots/"
printf 'Updated documentation screenshots.\n'
