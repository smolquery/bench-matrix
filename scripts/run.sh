#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

: "${SMOLQUERY_IMAGE:?set SMOLQUERY_IMAGE to the image under test}"
systems=${SYSTEMS:-smolquery clickhouse victorialogs}

scripts/fetch.sh
go build -o bin/bench ./cmd/bench
rm -rf out

failed=""
for system in $systems; do
  scripts/bench.sh "$system" || failed="$failed $system"
done

if [ -n "$failed" ]; then
  echo "failed:$failed"
  exit 1
fi
