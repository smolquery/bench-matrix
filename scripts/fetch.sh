#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

files=${FILES:-10}
duckdb_version=${DUCKDB_VERSION:-v1.5.4}
base=https://datasets.clickhouse.com/hits_compatible/athena_partitioned

mkdir -p bin data/hits

if [ ! -x bin/duckdb ]; then
  case "$(uname -s)-$(uname -m)" in
    Linux-x86_64) asset=duckdb_cli-linux-amd64.zip ;;
    Linux-aarch64) asset=duckdb_cli-linux-arm64.zip ;;
    Darwin-*) asset=duckdb_cli-osx-universal.zip ;;
    *) echo "no duckdb build for $(uname -s)-$(uname -m)"; exit 1 ;;
  esac
  curl -fsSL -o bin/duckdb.zip "https://github.com/duckdb/duckdb/releases/download/$duckdb_version/$asset"
  unzip -oq bin/duckdb.zip -d bin
  rm bin/duckdb.zip
  chmod +x bin/duckdb
fi
bin/duckdb --version

seq 0 $((files - 1)) | xargs -P 8 -I{} sh -c '
  f=data/hits/hits_{}.parquet
  [ -f "$f" ] || { curl -fsSL -o "$f.part" "'"$base"'/hits_{}.parquet" && mv "$f.part" "$f"; }
'
echo "$(ls data/hits/*.parquet | wc -l | tr -d ' ') parquet files, $(du -sh data/hits | cut -f1)"
