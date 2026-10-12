#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

: "${SMOLQUERY_IMAGE:?set SMOLQUERY_IMAGE to the image under test}"

docker compose down -v --remove-orphans >/dev/null 2>&1 || true
docker compose pull --quiet
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

echo "waiting for smolquery"
for _ in $(seq 1 120); do
  if curl -fs http://127.0.0.1:${SMOLQUERY_PORT:-4000}/healthz >/dev/null; then
    echo "smolquery healthy"
    exit 0
  fi
  sleep 1
done
echo "smolquery did not become healthy"
docker compose logs smolquery | tail -100
exit 1
