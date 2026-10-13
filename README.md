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

[`a8a561a`](https://github.com/smolquery/smolquery/commit/a8a561acada383ac4290324eb7c57203b5d8e5f3) on 2026-10-13: **ok** ([workflow run](https://github.com/smolquery/bench-matrix/actions/runs/38405413514))

### History

| date | commit | status | load s | rows/s | seal s | passed | first sum s | hot sum s | hot geomean ms |
|---|---|---|---:|---:|---:|---:|---:|---:|---:|
| 2026-10-13 | [`a8a561a`](https://github.com/smolquery/smolquery/commit/a8a561acada383ac4290324eb7c57203b5d8e5f3) | ok | 139.9 | 71492 | 15.7 | 43/43 | 78.78 | 70.66 | 1378 |
| 2026-10-13 | [`df8fe23`](https://github.com/smolquery/smolquery/commit/df8fe2317d7bdd35252b8a44d6d7a6e26ffd3fd9) | ok | 157.0 | 63683 | 70.1 | 43/43 | 88.08 | 84.80 | 1650 |
| 2026-10-12 | [`df8fe23`](https://github.com/smolquery/smolquery/commit/df8fe2317d7bdd35252b8a44d6d7a6e26ffd3fd9) | ok | 237.1 | 42173 | 71.6 | 43/43 | 134.25 | 127.36 | 2804 |

<details><summary>Per-query times, latest run</summary>

| query | first ms | hot ms | previous hot ms | change |
|---|---:|---:|---:|---:|
| Q1 | 318 | 286 | 330 | -14% |
| Q2 | 1345 | 1268 | 1514 | -16% |
| Q3 | 1701 | 1296 | 1520 | -15% |
| Q4 | 1563 | 1253 | 1554 | -19% |
| Q5 | 2292 | 2035 | 2355 | -14% |
| Q6 | 2468 | 2304 | 2583 | -11% |
| Q7 | 1326 | 1270 | 1491 | -15% |
| Q8 | 1295 | 1217 | 1542 | -21% |
| Q9 | 1692 | 1623 | 1997 | -19% |
| Q10 | 1645 | 1677 | 2074 | -19% |
| Q11 | 1398 | 1315 | 1574 | -16% |
| Q12 | 1368 | 1326 | 1644 | -19% |
| Q13 | 1703 | 1581 | 1994 | -21% |
| Q14 | 1982 | 1906 | 2415 | -21% |
| Q15 | 1775 | 1705 | 2093 | -19% |
| Q16 | 1484 | 1480 | 1807 | -18% |
| Q17 | 2023 | 1988 | 2384 | -17% |
| Q18 | 1953 | 1871 | 2295 | -18% |
| Q19 | 3317 | 2485 | 3194 | -22% |
| Q20 | 718 | 537 | 605 | -11% |
| Q21 | 2495 | 1711 | 1991 | -14% |
| Q22 | 1688 | 1692 | 2137 | -21% |
| Q23 | 2809 | 2298 | 2775 | -17% |
| Q24 | 3958 | 1669 | 2025 | -18% |
| Q25 | 660 | 667 | 753 | -11% |
| Q26 | 673 | 608 | 724 | -16% |
| Q27 | 606 | 572 | 662 | -14% |
| Q28 | 1778 | 1663 | 2041 | -19% |
| Q29 | 7048 | 6667 | 7607 | -12% |
| Q30 | 1293 | 1305 | 1564 | -17% |
| Q31 | 1801 | 1762 | 2076 | -15% |
| Q32 | 1943 | 1977 | 2335 | -15% |
| Q33 | 4082 | 4087 | 4874 | -16% |
| Q34 | 2878 | 2898 | 3390 | -15% |
| Q35 | 2858 | 2765 | 3372 | -18% |
| Q36 | 1784 | 1636 | 2116 | -23% |
| Q37 | 1819 | 1793 | 2049 | -12% |
| Q38 | 1651 | 1537 | 1959 | -22% |
| Q39 | 837 | 575 | 702 | -18% |
| Q40 | 928 | 712 | 860 | -17% |
| Q41 | 701 | 534 | 628 | -15% |
| Q42 | 553 | 564 | 588 | -4% |
| Q43 | 565 | 543 | 614 | -12% |

</details>

<!-- results:end -->
