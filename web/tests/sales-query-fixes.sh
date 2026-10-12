#!/usr/bin/env bash
# Fresh disposable database; logs/screenshots are retained for inspection.
set -euo pipefail
repo_dir="$(cd "$(dirname "$0")/../.." && pwd)"
run_dir="$(mktemp -d /tmp/sales-query-fixes.XXXXXX)"
api_port="${SALES_API_PORT:-18179}"
export APP_DATABASE__URL="$run_dir/acceptance.db"
export APP_FILE__STORAGE_ROOT="$run_dir/files"
export APP_HTTP__ADDRESS="127.0.0.1:$api_port"
export PHASE5_API_URL="http://127.0.0.1:$api_port"
export SELECTION_API_URL="$PHASE5_API_URL"
export SALES_SCREENSHOT_DIR="$run_dir/screenshots"
cd "$repo_dir"
task db:migrate:sqlite > "$run_dir/migrate.log" 2>&1
setsid task api:sqlite > "$run_dir/api.log" 2>&1 &
api_pid=$!
trap 'kill -TERM -- -"$api_pid" 2>/dev/null || true' EXIT
for attempt in {1..100}; do
  if curl -fsS "$PHASE5_API_URL/health" > /dev/null 2>&1; then break; fi
  if ! kill -0 "$api_pid" 2>/dev/null; then cat "$run_dir/api.log"; exit 1; fi
  sleep 0.2
done
cd web
printf 'Artifacts: %s\n' "$run_dir"
for script in ${SALES_ACCEPTANCE_SCRIPTS:-issue66-sale-query sales-query-fixes master-data-selection phase5-document-query phase5-sale-post}; do
  node "tests/$script.mjs" > "$run_dir/$script.log" 2>&1 || { cat "$run_dir/$script.log"; exit 1; }
  cat "$run_dir/$script.log"
done
printf 'Artifacts: %s\n' "$run_dir"
