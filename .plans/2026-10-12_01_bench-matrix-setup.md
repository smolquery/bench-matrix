# bench-matrix: set up the repo and the first two benches

## Goal

Run two benches every time new code lands on `smolquery/smolquery` `main`.
Commit the results back into this repo as history.

1. **load** — load the rows that the query suite needs, and time it.
2. **query** — run the query suite against the loaded data, and time each query.

## Suite: ClickBench

- 43 queries over one flat `hits` table (105 columns). No joins.
- `smolquery/random-bench` already passes 43/43 at 100M rows.
- Reuse from random-bench:
  - `scripts/clickbench/queries.sql` (the DuckDB-dialect queries)
  - `scripts/clickbench/load-file.sh` (Parquet → NDJSON → insert route)
  - `schemas/hits_v1.smolquery.json` (schema and clustering)
  - `tools/cbquery/main.go` (per-query JSON: `first_ms`, `hot_ms`, tries, errors)
- ClickBench publishes `duckdb-datalake*` and `clickhouse-datalake*` numbers.
  This gives an outside reference point.

## Trigger

- `smolquery/smolquery` `release.yml` builds `ghcr.io/chasers/smolquery:sha-<sha>` on every push to `main`.
- Add a final step to `release.yml`: send a `repository_dispatch` (`smolquery-main`) to `smolquery/bench-matrix` with the commit SHA.
  - Needs a token with write access to `bench-matrix`, stored as a secret in `smolquery/smolquery`.
- `bench-matrix` also has `workflow_dispatch` with a `sha` input for manual runs and backfill.
- Use a `concurrency` group so that runs queue and do not overlap.

## Workflow shape (`.github/workflows/bench.yml`)

1. Resolve the SHA and the image ref.
2. Pull the image. Start smolquery with a fresh `/data` volume.
   - Set `SMOLQUERY_JOB_MEMORY_LIMIT` high enough for Q23, Q34, Q35.
3. Fetch the `hits_N.parquet` files (cache them with `actions/cache`).
4. **load** bench: create the table, insert all rows, wait until the rows are queryable. Record wall time and rows/s.
5. **query** bench: run the 43 queries, 3 tries each. Record cold and hot times.
6. Write results. Commit and push to `main` with the `GITHUB_TOKEN` (`contents: write`).

## Results layout

```
results/
  runs/<YYYY-MM-DD>_<short-sha>/
    meta.json      # sha, image digest, runner, scale, timestamps
    load.json
    query.json
  history.jsonl    # one line per run, summary numbers, append-only
README.md          # latest run + trend table, regenerated each run
```

## Decisions

- [x] Runner: GitHub-hosted `ubuntu-latest` to start. Change `runs-on` later for a dedicated box.
- [x] Scale: 10M rows (`hits_0..9`).
- [x] Repo visibility: public.
- [x] Image access: `ghcr.io/chasers/smolquery` is public. Tags are `sha-<full sha>`.
- [x] Object store: RustFS (`rustfs/rustfs:1.0.0`) for the sealed tier (user request).
- [x] Catalog: Postgres 17. The default SQLite catalog wedged on `database is locked` during the first seal (1M rows, image `df8fe23`).
- [x] Harness language: Go (`cmd/bench`), ported from random-bench `upload1brc` and `cbquery`.

## Tasks

- [x] `git init`, create `smolquery/bench-matrix` on GitHub, push.
- [x] Port the ClickBench assets from random-bench.
- [x] Write the load bench (`bench load`).
- [x] Write the query bench (`bench query`).
- [x] Write the results writer and the README generator (`bench publish`).
- [x] Write `bench.yml` with `repository_dispatch`, `workflow_dispatch` and a daily catch-up `schedule`.
- [x] Smoke test at 1M rows locally: load 17.0 s, seal 69 s, 43/43 queries.
- [x] Full 10M-row local run (Docker Desktop, 8 vCPU): ingest 164 s (60.9k rows/s), seal 28 s, 43/43, hot sum 67.2 s (default distributed) or 45.1 s (`distributed: false`).
  - Fixed: the seal merge engine OOMed at its 2 GB fallback with no cgroup limit. The container now has `mem_limit: 12g`.
- [x] Add the dispatch step to `smolquery/smolquery` `release.yml` and docs: smolquery/smolquery#473.
- [ ] Add the `BENCH_MATRIX_TOKEN` secret to `smolquery/smolquery`, then merge #473 (user).
- [ ] Do a manual run against the current `main` SHA (started: actions run 38267769769). Check that the results commit lands.
- [x] Write the README: what runs, when, where results live, how to run it locally.

## Later

- A second query pass with `distributed: false`, to track the single-engine path and the scatter overhead.

- A third bench for the target workload: OTel logs ingest and query (random-bench has the schema and bodies).
