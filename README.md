# bench-matrix

bench-matrix benchmarks every commit that lands on the `main` branch of
[smolquery/smolquery](https://github.com/smolquery/smolquery). Each run
commits its results to this repository, so the history stays in git.

## What runs

A run starts the smolquery image for one commit and runs two benches on the
[ClickBench](https://github.com/ClickHouse/ClickBench) `hits` dataset.

| bench | what it does | what it measures |
|---|---|---|
| **load** | Creates `bench.hits` (105 columns), inserts 10M rows through the NDJSON insert route, then waits until the hot tier seals to object storage. | Ingest time and rows/s until the last insert is acknowledged. Seal time until the hot tier is empty. An exact `count(*)` check. The bench fails if the count is wrong or the seal does not finish in 20 minutes. |
| **query** | Runs the 43 ClickBench queries, 3 tries each, through `POST /v1/queries`. | The first try and the best of the other two tries ("hot"), per query. The sums and the geometric mean of the hot times. |

The stack is one smolquery node, configured like a production node:

- **Catalog:** Postgres 17 (`SMOLQUERY_CATALOG`).
- **Sealed tier:** [RustFS](https://github.com/rustfs/rustfs), an S3-compatible
  object store (`SMOLQUERY_S3_*`).
- **Query file cache:** on (`SMOLQUERY_QUERY_FILE_CACHE_DIR`).
- **Memory:** a 12 GB container limit (`SMOLQUERY_MEM_LIMIT`). smolquery sizes
  its storage engines from the cgroup limit, as it does in a Kubernetes pod.
- **Job memory:** `SMOLQUERY_JOB_MEMORY_LIMIT=6GB`.
- **Ingest:** 24 MB NDJSON bodies over 8 concurrent requests. The insert body
  cap is 32 MB (`SMOLQUERY_INSERT_MAX_NDJSON_BYTES`). The group-commit flush
  trigger is 32 MB (`SMOLQUERY_FLUSH_MAX_BYTES`), and the buffer ceiling is
  256 MB (`SMOLQUERY_MAX_BUFFERED_BYTES`).

`compose.yaml` holds the full configuration.

The load bench converts Parquet to NDJSON with the DuckDB CLI on the same
machine. That conversion shares the CPU with smolquery during the load.

## When it runs

- **On each push to smolquery `main`.** The smolquery `release.yml` workflow
  sends a `repository_dispatch` (`smolquery-main`) with the commit SHA after it
  pushes the image `ghcr.io/chasers/smolquery:sha-<sha>`.
- **Manually.** Run the `Bench` workflow with an optional `sha` input.
- **Daily at 06:17 UTC.** The run benches the `main` head if no run for that
  commit exists. This catches a lost dispatch.

Runs queue one at a time. The runner is a GitHub-hosted `ubuntu-latest` machine
(4 vCPU, 16 GB). Shared runners add noise, so compare trends, not single runs.

## Where the results are

- `results/history.jsonl`: one summary line per run, append-only.
- `results/runs/<timestamp>_<sha>/`: `meta.json`, `load.json` and
  `query.json` for one run.
- The section below: the latest run, the last 30 runs, and per-query times
  against the previous run. `bench publish` rewrites it after each run.
- Each workflow run also uploads `out/` with the container logs as an artifact.

## Run it locally

You need Docker, Go 1.26, `curl` and `unzip`.

1. Pick an image: `export SMOLQUERY_IMAGE=ghcr.io/chasers/smolquery:sha-<full sha>`.
2. Run the benches: `FILES=10 scripts/run.sh`. The results go to `out/`.
3. Stop the stack: `scripts/down.sh`.

`FILES` sets the number of 1M-row parquet files (default 10). The ingest
variables in `compose.yaml` read from the environment, so you can override
them for a local run. If ports 4000 or
9000 are in use, set `SMOLQUERY_PORT` and `RUSTFS_PORT`.

## Layout

| path | contents |
|---|---|
| `cmd/bench` | The Go harness: `bench load`, `bench query`, `bench publish`. |
| `suites/clickbench` | The table definition with its clustering key, and the 43 queries. |
| `scripts` | `fetch.sh` (data and DuckDB CLI), `up.sh`, `down.sh`, `run.sh`. |
| `compose.yaml` | Postgres, RustFS and smolquery. |
| `.github/workflows/bench.yml` | The CI workflow. |

## Results

<!-- results:start -->
### Latest run

[`df8fe23`](https://github.com/smolquery/smolquery/commit/df8fe2317d7bdd35252b8a44d6d7a6e26ffd3fd9) on 2026-10-12: **ok** ([workflow run](https://github.com/smolquery/bench-matrix/actions/runs/38267769769))

### History

| date | commit | status | load s | rows/s | seal s | passed | first sum s | hot sum s | hot geomean ms |
|---|---|---|---:|---:|---:|---:|---:|---:|---:|
| 2026-10-12 | [`df8fe23`](https://github.com/smolquery/smolquery/commit/df8fe2317d7bdd35252b8a44d6d7a6e26ffd3fd9) | ok | 237.1 | 42173 | 71.6 | 43/43 | 134.25 | 127.36 | 2804 |

<details><summary>Per-query times, latest run</summary>

| query | first ms | hot ms | previous hot ms | change |
|---|---:|---:|---:|---:|
| Q1 | 1378 | 1239 |  |  |
| Q2 | 2581 | 2342 |  |  |
| Q3 | 2502 | 2458 |  |  |
| Q4 | 2740 | 2452 |  |  |
| Q5 | 3981 | 3983 |  |  |
| Q6 | 4445 | 3988 |  |  |
| Q7 | 2617 | 2446 |  |  |
| Q8 | 2413 | 2598 |  |  |
| Q9 | 3220 | 3208 |  |  |
| Q10 | 3102 | 3087 |  |  |
| Q11 | 2658 | 2609 |  |  |
| Q12 | 2486 | 2608 |  |  |
| Q13 | 3176 | 3075 |  |  |
| Q14 | 3818 | 3897 |  |  |
| Q15 | 3297 | 3345 |  |  |
| Q16 | 3031 | 2289 |  |  |
| Q17 | 2940 | 2895 |  |  |
| Q18 | 2970 | 2850 |  |  |
| Q19 | 3675 | 3563 |  |  |
| Q20 | 3192 | 2702 |  |  |
| Q21 | 2848 | 2564 |  |  |
| Q22 | 2557 | 2532 |  |  |
| Q23 | 3674 | 3260 |  |  |
| Q24 | 4042 | 3416 |  |  |
| Q25 | 2742 | 2707 |  |  |
| Q26 | 3123 | 2559 |  |  |
| Q27 | 3039 | 2476 |  |  |
| Q28 | 2561 | 2491 |  |  |
| Q29 | 8619 | 8158 |  |  |
| Q30 | 2051 | 2141 |  |  |
| Q31 | 2700 | 2540 |  |  |
| Q32 | 2903 | 2812 |  |  |
| Q33 | 5457 | 5533 |  |  |
| Q34 | 4009 | 3951 |  |  |
| Q35 | 4007 | 3957 |  |  |
| Q36 | 2530 | 2392 |  |  |
| Q37 | 3977 | 3720 |  |  |
| Q38 | 3515 | 3241 |  |  |
| Q39 | 1992 | 1960 |  |  |
| Q40 | 2082 | 2004 |  |  |
| Q41 | 2180 | 1904 |  |  |
| Q42 | 1840 | 1809 |  |  |
| Q43 | 1581 | 1599 |  |  |

</details>

<!-- results:end -->
