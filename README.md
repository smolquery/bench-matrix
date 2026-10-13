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
  pushes the image `ghcr.io/smolquery/smolquery:sha-<sha>`. For commits up to
  `df8fe23`, the workflow uses the old name, `ghcr.io/chasers/smolquery`.
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

1. Pick an image: `export SMOLQUERY_IMAGE=ghcr.io/smolquery/smolquery:sha-<full sha>`.
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

[`bdbcba4`](https://github.com/smolquery/smolquery/commit/bdbcba43dabc4d1dddfc437c0fdbc12af2a0b67a) on 2026-10-13: **ok** ([workflow run](https://github.com/smolquery/bench-matrix/actions/runs/38419951440))

### History

| date | commit | status | load s | rows/s | seal s | passed | first sum s | hot sum s | hot geomean ms |
|---|---|---|---:|---:|---:|---:|---:|---:|---:|
| 2026-10-13 | [`bdbcba4`](https://github.com/smolquery/smolquery/commit/bdbcba43dabc4d1dddfc437c0fdbc12af2a0b67a) | ok | 155.0 | 64511 | 64.2 | 43/43 | 89.30 | 85.19 | 1661 |
| 2026-10-13 | [`a8a561a`](https://github.com/smolquery/smolquery/commit/a8a561acada383ac4290324eb7c57203b5d8e5f3) | ok | 139.9 | 71492 | 15.7 | 43/43 | 78.78 | 70.66 | 1378 |
| 2026-10-13 | [`df8fe23`](https://github.com/smolquery/smolquery/commit/df8fe2317d7bdd35252b8a44d6d7a6e26ffd3fd9) | ok | 157.0 | 63683 | 70.1 | 43/43 | 88.08 | 84.80 | 1650 |
| 2026-10-12 | [`df8fe23`](https://github.com/smolquery/smolquery/commit/df8fe2317d7bdd35252b8a44d6d7a6e26ffd3fd9) | ok | 237.1 | 42173 | 71.6 | 43/43 | 134.25 | 127.36 | 2804 |

<details><summary>Per-query times, latest run</summary>

| query | first ms | hot ms | previous hot ms | change |
|---|---:|---:|---:|---:|
| Q1 | 364 | 346 | 286 | +21% |
| Q2 | 1619 | 1544 | 1268 | +22% |
| Q3 | 1585 | 1566 | 1296 | +21% |
| Q4 | 1591 | 1557 | 1253 | +24% |
| Q5 | 2603 | 2502 | 2035 | +23% |
| Q6 | 2816 | 2626 | 2304 | +14% |
| Q7 | 1546 | 1477 | 1270 | +16% |
| Q8 | 1526 | 1491 | 1217 | +22% |
| Q9 | 1947 | 1991 | 1623 | +23% |
| Q10 | 2118 | 2068 | 1677 | +23% |
| Q11 | 1600 | 1585 | 1315 | +21% |
| Q12 | 1582 | 1599 | 1326 | +21% |
| Q13 | 2040 | 1979 | 1581 | +25% |
| Q14 | 2467 | 2394 | 1906 | +26% |
| Q15 | 2141 | 2034 | 1705 | +19% |
| Q16 | 1877 | 1773 | 1480 | +20% |
| Q17 | 2450 | 2478 | 1988 | +25% |
| Q18 | 2327 | 2316 | 1871 | +24% |
| Q19 | 3198 | 3138 | 2485 | +26% |
| Q20 | 691 | 599 | 537 | +12% |
| Q21 | 2418 | 2082 | 1711 | +22% |
| Q22 | 2068 | 2013 | 1692 | +19% |
| Q23 | 2887 | 2695 | 2298 | +17% |
| Q24 | 2407 | 2007 | 1669 | +20% |
| Q25 | 811 | 728 | 667 | +9% |
| Q26 | 743 | 797 | 608 | +31% |
| Q27 | 765 | 690 | 572 | +21% |
| Q28 | 2110 | 2004 | 1663 | +21% |
| Q29 | 9359 | 7907 | 6667 | +19% |
| Q30 | 1647 | 1594 | 1305 | +22% |
| Q31 | 2116 | 2109 | 1762 | +20% |
| Q32 | 2381 | 2297 | 1977 | +16% |
| Q33 | 5022 | 4994 | 4087 | +22% |
| Q34 | 3455 | 3414 | 2898 | +18% |
| Q35 | 3272 | 3317 | 2765 | +20% |
| Q36 | 2161 | 2024 | 1636 | +24% |
| Q37 | 2222 | 2020 | 1793 | +13% |
| Q38 | 1862 | 1898 | 1537 | +24% |
| Q39 | 690 | 699 | 575 | +22% |
| Q40 | 883 | 889 | 712 | +25% |
| Q41 | 692 | 628 | 534 | +17% |
| Q42 | 623 | 631 | 564 | +12% |
| Q43 | 612 | 693 | 543 | +28% |

</details>

<!-- results:end -->
