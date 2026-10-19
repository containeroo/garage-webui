#!/bin/sh
set -eu
root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
temp_dir=$(mktemp -d "${TMPDIR:-/tmp}/garage-webui-e2e.XXXXXXXX")
project="garage-webui-e2e-$(basename "$temp_dir" | tr '[:upper:].' '[:lower:]-')"
app_pid=''
compose() { docker compose --project-name "$project" --file "$root/dev/compose/compose.e2e.yaml" "$@"; }
cleanup() {
  status=$?
  trap - EXIT HUP INT TERM
  if [ -n "$app_pid" ]; then
    kill "$app_pid" 2>/dev/null || true
    wait "$app_pid" 2>/dev/null || true
  fi
  if [ "$status" -ne 0 ] && [ -f "$temp_dir/app.log" ]; then tail -n 60 "$temp_dir/app.log" >&2; fi
  if ! compose down --volumes --remove-orphans; then status=1; fi
  rm -r -- "$temp_dir"
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' HUP TERM
build_app() {
  make frontend
  if [ -z "${E2E_CHROMIUM_PATH:-}" ]; then (cd web && npx playwright install chromium); fi
  go build -o "$temp_dir/garage-webui" ./cmd/garage-webui
}
allocate_ports() {
  ports=$(
    node <<'NODE'
const net = require("node:net");
(async()=>{const servers=[];for(let i=0;i<2;i++){const server=net.createServer();await new Promise((resolve,reject)=>{server.once("error",reject);server.listen(0,"127.0.0.1",resolve)});servers.push(server)}console.log(servers.map(server=>server.address().port).join(" "));await Promise.all(servers.map(server=>new Promise(resolve=>server.close(resolve))))})().catch(error=>{console.error(error);process.exitCode=1});
NODE
  )
  # shellcheck disable=SC2086
  set -- $ports
  app_port=$1 management_port=$2
}
start_app() {
  compose up -d --wait garage postgres
  compose run --rm --no-deps garage-ready
  garage_port=$(compose port garage 3903)
  s3_port=$(compose port garage 3900)
  database_port=$(compose port postgres 5432)
  export E2E_GARAGE_URL="http://127.0.0.1:${garage_port##*:}" E2E_ADMIN_TOKEN=local-garage-admin-token
  export GARAGE_WEBUI__DATABASE_URL="postgres://garage_webui:local-database-password@127.0.0.1:${database_port##*:}/garage_webui?sslmode=disable"
  export GARAGE_WEBUI__GARAGE_URL="$E2E_GARAGE_URL" GARAGE_WEBUI__SESSION_SECRET=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=
  export GARAGE_WEBUI__PUBLIC_URL="http://127.0.0.1:$app_port" GARAGE_WEBUI__S3_PUBLIC_URL="http://127.0.0.1:${s3_port##*:}"
  export GARAGE_WEBUI__ROUTE_PREFIX=/e2e E2E_BASE_URL="http://127.0.0.1:$app_port/e2e/"
  "$temp_dir/garage-webui" --listen-address "127.0.0.1:$app_port" --management-listen-address "127.0.0.1:$management_port" >"$temp_dir/app.log" 2>&1 &
  app_pid=$!
  attempt=0
  until curl --silent --fail "${E2E_BASE_URL}api/session" >/dev/null; do
    if ! kill -0 "$app_pid" 2>/dev/null || [ "$attempt" -ge 60 ]; then
      printf 'Application did not become ready\n' >&2
      exit 1
    fi
    attempt=$((attempt + 1))
    sleep 1
  done
}
cd "$root"
build_app
allocate_ports
start_app
if [ "${CAPTURE_SCREENSHOTS:-false}" = true ]; then
  mkdir -p "$temp_dir/screenshots"
  SCREENSHOT_URL="$E2E_BASE_URL" SCREENSHOT_DIR="$temp_dir/screenshots" node web/capture-screenshots.mjs
  mkdir -p docs/content/assets/screenshots
  cp "$temp_dir/screenshots/"*.png docs/content/assets/screenshots/
else
  (cd web && npm run test:e2e)
fi
