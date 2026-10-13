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

[`0e1641c`](https://github.com/smolquery/smolquery/commit/0e1641c879bb27afea6e222cab36054f9da816da) on 2026-10-13: **ok** ([workflow run](https://github.com/smolquery/bench-matrix/actions/runs/38446341514))

### History

| date | commit | status | load s | rows/s | seal s | passed | first sum s | hot sum s | hot geomean ms |
|---|---|---|---:|---:|---:|---:|---:|---:|---:|
| 2026-10-13 | [`0e1641c`](https://github.com/smolquery/smolquery/commit/0e1641c879bb27afea6e222cab36054f9da816da) | ok | 203.1 | 49229 | 65.6 | 43/43 | 96.64 | 90.93 | 1831 |
| 2026-10-13 | [`bdbcba4`](https://github.com/smolquery/smolquery/commit/bdbcba43dabc4d1dddfc437c0fdbc12af2a0b67a) | ok | 155.0 | 64511 | 64.2 | 43/43 | 89.30 | 85.19 | 1661 |
| 2026-10-13 | [`a8a561a`](https://github.com/smolquery/smolquery/commit/a8a561acada383ac4290324eb7c57203b5d8e5f3) | ok | 139.9 | 71492 | 15.7 | 43/43 | 78.78 | 70.66 | 1378 |
| 2026-10-13 | [`df8fe23`](https://github.com/smolquery/smolquery/commit/df8fe2317d7bdd35252b8a44d6d7a6e26ffd3fd9) | ok | 157.0 | 63683 | 70.1 | 43/43 | 88.08 | 84.80 | 1650 |
| 2026-10-12 | [`df8fe23`](https://github.com/smolquery/smolquery/commit/df8fe2317d7bdd35252b8a44d6d7a6e26ffd3fd9) | ok | 237.1 | 42173 | 71.6 | 43/43 | 134.25 | 127.36 | 2804 |

<details><summary>Per-query times, latest run</summary>

| query | first ms | hot ms | previous hot ms | change |
|---|---:|---:|---:|---:|
| Q1 | 395 | 427 | 346 | +23% |
| Q2 | 1775 | 1510 | 1544 | -2% |
| Q3 | 1611 | 1553 | 1566 | -1% |
| Q4 | 1694 | 1566 | 1557 | +1% |
| Q5 | 2732 | 2708 | 2502 | +8% |
| Q6 | 3028 | 2758 | 2626 | +5% |
| Q7 | 1625 | 1516 | 1477 | +3% |
| Q8 | 1486 | 1508 | 1491 | +1% |
| Q9 | 1969 | 1975 | 1991 | -1% |
| Q10 | 2040 | 2036 | 2068 | -2% |
| Q11 | 1621 | 1570 | 1585 | -1% |
| Q12 | 1578 | 1576 | 1599 | -1% |
| Q13 | 1985 | 2074 | 1979 | +5% |
| Q14 | 2533 | 2469 | 2394 | +3% |
| Q15 | 2219 | 2144 | 2034 | +5% |
| Q16 | 1871 | 1796 | 1773 | +1% |
| Q17 | 2567 | 2439 | 2478 | -2% |
| Q18 | 2496 | 2386 | 2316 | +3% |
| Q19 | 3308 | 3189 | 3138 | +2% |
| Q20 | 1086 | 911 | 599 | +52% |
| Q21 | 2928 | 2216 | 2082 | +6% |
| Q22 | 2182 | 2163 | 2013 | +7% |
| Q23 | 3351 | 3110 | 2695 | +15% |
| Q24 | 4944 | 2557 | 2007 | +27% |
| Q25 | 951 | 905 | 728 | +24% |
| Q26 | 985 | 1016 | 797 | +27% |
| Q27 | 924 | 965 | 690 | +40% |
| Q28 | 2174 | 2185 | 2004 | +9% |
| Q29 | 7561 | 7907 | 7907 | -0% |
| Q30 | 1621 | 1622 | 1594 | +2% |
| Q31 | 2169 | 2148 | 2109 | +2% |
| Q32 | 2499 | 2338 | 2297 | +2% |
| Q33 | 5278 | 5237 | 4994 | +5% |
| Q34 | 3741 | 3598 | 3414 | +5% |
| Q35 | 3835 | 3833 | 3317 | +16% |
| Q36 | 1998 | 2013 | 2024 | -1% |
| Q37 | 2622 | 2320 | 2020 | +15% |
| Q38 | 2090 | 2031 | 1898 | +7% |
| Q39 | 1011 | 962 | 699 | +38% |
| Q40 | 1229 | 1114 | 889 | +25% |
| Q41 | 1083 | 846 | 628 | +35% |
| Q42 | 935 | 858 | 631 | +36% |
| Q43 | 910 | 873 | 693 | +26% |

</details>

<!-- results:end -->
