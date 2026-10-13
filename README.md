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

[`03e51fa`](https://github.com/smolquery/smolquery/commit/03e51faf724811af16e0b9889c1a0562984a8291) on 2026-10-13: **ok** ([workflow run](https://github.com/smolquery/bench-matrix/actions/runs/38448710609))

### History

| date | commit | status | load s | rows/s | seal s | passed | first sum s | hot sum s | hot geomean ms |
|---|---|---|---:|---:|---:|---:|---:|---:|---:|
| 2026-10-13 | [`03e51fa`](https://github.com/smolquery/smolquery/commit/03e51faf724811af16e0b9889c1a0562984a8291) | ok | 141.3 | 70769 | 64.0 | 43/43 | 66.94 | 62.50 | 1215 |
| 2026-10-13 | [`0e1641c`](https://github.com/smolquery/smolquery/commit/0e1641c879bb27afea6e222cab36054f9da816da) | ok | 203.1 | 49229 | 65.6 | 43/43 | 96.64 | 90.93 | 1831 |
| 2026-10-13 | [`bdbcba4`](https://github.com/smolquery/smolquery/commit/bdbcba43dabc4d1dddfc437c0fdbc12af2a0b67a) | ok | 155.0 | 64511 | 64.2 | 43/43 | 89.30 | 85.19 | 1661 |
| 2026-10-13 | [`a8a561a`](https://github.com/smolquery/smolquery/commit/a8a561acada383ac4290324eb7c57203b5d8e5f3) | ok | 139.9 | 71492 | 15.7 | 43/43 | 78.78 | 70.66 | 1378 |
| 2026-10-13 | [`df8fe23`](https://github.com/smolquery/smolquery/commit/df8fe2317d7bdd35252b8a44d6d7a6e26ffd3fd9) | ok | 157.0 | 63683 | 70.1 | 43/43 | 88.08 | 84.80 | 1650 |
| 2026-10-12 | [`df8fe23`](https://github.com/smolquery/smolquery/commit/df8fe2317d7bdd35252b8a44d6d7a6e26ffd3fd9) | ok | 237.1 | 42173 | 71.6 | 43/43 | 134.25 | 127.36 | 2804 |

<details><summary>Per-query times, latest run</summary>

| query | first ms | hot ms | previous hot ms | change |
|---|---:|---:|---:|---:|
| Q1 | 233 | 239 | 427 | -44% |
| Q2 | 1010 | 1025 | 1510 | -32% |
| Q3 | 1068 | 999 | 1553 | -36% |
| Q4 | 1078 | 1013 | 1566 | -35% |
| Q5 | 1907 | 1945 | 2708 | -28% |
| Q6 | 2187 | 2014 | 2758 | -27% |
| Q7 | 1062 | 992 | 1516 | -35% |
| Q8 | 1021 | 1004 | 1508 | -33% |
| Q9 | 1411 | 1391 | 1975 | -30% |
| Q10 | 1447 | 1434 | 2036 | -30% |
| Q11 | 1045 | 1031 | 1570 | -34% |
| Q12 | 1095 | 1088 | 1576 | -31% |
| Q13 | 1489 | 1406 | 2074 | -32% |
| Q14 | 1791 | 1801 | 2469 | -27% |
| Q15 | 1563 | 1491 | 2144 | -30% |
| Q16 | 1183 | 1224 | 1796 | -32% |
| Q17 | 1767 | 1845 | 2439 | -24% |
| Q18 | 1759 | 1686 | 2386 | -29% |
| Q19 | 2378 | 2272 | 3189 | -29% |
| Q20 | 544 | 503 | 911 | -45% |
| Q21 | 2245 | 1618 | 2216 | -27% |
| Q22 | 1600 | 1484 | 2163 | -31% |
| Q23 | 2414 | 2165 | 3110 | -30% |
| Q24 | 3221 | 1830 | 2557 | -28% |
| Q25 | 549 | 583 | 905 | -36% |
| Q26 | 615 | 567 | 1016 | -44% |
| Q27 | 503 | 581 | 965 | -40% |
| Q28 | 1545 | 1506 | 2185 | -31% |
| Q29 | 5819 | 5316 | 7907 | -33% |
| Q30 | 1040 | 1043 | 1622 | -36% |
| Q31 | 1496 | 1468 | 2148 | -32% |
| Q32 | 1689 | 1580 | 2338 | -32% |
| Q33 | 3931 | 3938 | 5237 | -25% |
| Q34 | 3062 | 2690 | 3598 | -25% |
| Q35 | 2835 | 2812 | 3833 | -27% |
| Q36 | 1461 | 1428 | 2013 | -29% |
| Q37 | 1627 | 1485 | 2320 | -36% |
| Q38 | 1344 | 1312 | 2031 | -35% |
| Q39 | 582 | 523 | 962 | -46% |
| Q40 | 738 | 713 | 1114 | -36% |
| Q41 | 592 | 496 | 846 | -41% |
| Q42 | 487 | 492 | 858 | -43% |
| Q43 | 510 | 463 | 873 | -47% |

</details>

<!-- results:end -->
