#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

: "${SMOLQUERY_IMAGE:?set SMOLQUERY_IMAGE to the image under test}"
system=${1:-smolquery}

docker compose down -v --remove-orphans >/dev/null 2>&1 || true

wait_for() {
  local url=$1
  echo "waiting for $system"
  for _ in $(seq 1 120); do
    if curl -fs --max-time 5 "$url" >/dev/null; then
      echo "$system healthy"
      return 0
    fi
    sleep 1
  done
  echo "$system did not become healthy"
  docker compose logs "$system" | tail -100
  return 1
}

case "$system" in
  clickhouse)
    docker compose pull --quiet clickhouse
    docker compose up -d clickhouse
    wait_for "http://127.0.0.1:${CLICKHOUSE_PORT:-8123}/ping"
    exit
    ;;
  victorialogs)
    docker compose pull --quiet victorialogs
    docker compose up -d victorialogs
    wait_for "http://127.0.0.1:${VICTORIALOGS_PORT:-9428}/health"
    exit
    ;;
  smolquery) ;;
  *) echo "unknown system $system"; exit 2 ;;
esac

docker compose pull --quiet postgres rustfs smolquery
docker compose up -d --wait postgres rustfs

echo "waiting for rustfs"
for _ in $(seq 1 60); do
  status=$(curl -s -o /dev/null -w '%{http_code}' -X PUT \
    --aws-sigv4 "aws:amz:us-east-1:s3" --user bench-access:bench-secret-key \
    http://127.0.0.1:${RUSTFS_PORT:-9000}/smolquery-sealed || true)
  case "$status" in
    200|409) break ;;
  esac
  sleep 1
done
case "$status" in
  200|409) echo "bucket smolquery-sealed ready" ;;
  *) echo "rustfs did not create the bucket (HTTP $status)"; docker compose logs rustfs | tail -50; exit 1 ;;
esac

docker compose up -d smolquery
wait_for "http://127.0.0.1:${SMOLQUERY_PORT:-4000}/healthz"
