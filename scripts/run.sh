#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

: "${SMOLQUERY_IMAGE:?set SMOLQUERY_IMAGE to the image under test}"
files=${FILES:-10}
api=http://127.0.0.1:${SMOLQUERY_PORT:-4000}
export SMOLQUERY_API_KEY=bench

FILES=$files scripts/fetch.sh
go build -o bin/bench ./cmd/bench
scripts/up.sh
rm -rf out
bin/bench load -api "$api" -files "$files" -out out/load.json
bin/bench query -api "$api" -out out/query.json
