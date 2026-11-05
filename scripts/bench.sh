#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

: "${SMOLQUERY_IMAGE:?set SMOLQUERY_IMAGE to the image under test}"
system=${1:?usage: bench.sh smolquery|clickhouse|victorialogs}
files=${FILES:-10}
out=out/$system
export SMOLQUERY_API_KEY=bench

case "$system" in
  smolquery) port=${SMOLQUERY_PORT:-4000} ;;
  clickhouse) port=${CLICKHOUSE_PORT:-8123} ;;
  victorialogs) port=${VICTORIALOGS_PORT:-9428} ;;
  *) echo "unknown system $system"; exit 2 ;;
esac
api=http://127.0.0.1:$port

mkdir -p "$out"

finish() {
  docker compose logs --no-color > "$out/compose.log" 2>&1 || true
  scripts/down.sh >/dev/null 2>&1 || true
}
trap finish EXIT

scripts/up.sh "$system"
bin/bench load -system "$system" -api "$api" -files "$files" -out "$out/load.json"
bin/bench query -system "$system" -api "$api" -out "$out/query.json"
