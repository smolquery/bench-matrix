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

[`75776ec`](https://github.com/smolquery/smolquery/commit/75776ecb5a96042def8a7bc5c6f9ff45e42d5f5b) on 2026-10-16: **ok** ([workflow run](https://github.com/smolquery/bench-matrix/actions/runs/38832639073))

### History

| date | commit | status | load s | rows/s | seal s | passed | first sum s | hot sum s | hot geomean ms |
|---|---|---|---:|---:|---:|---:|---:|---:|---:|
| 2026-10-16 | [`75776ec`](https://github.com/smolquery/smolquery/commit/75776ecb5a96042def8a7bc5c6f9ff45e42d5f5b) | ok | 146.5 | 68241 | 65.0 | 43/43 | 92.22 | 87.73 | 1716 |
| 2026-10-15 | [`4fb926b`](https://github.com/smolquery/smolquery/commit/4fb926bcd677161c130be6806c69f154cdc6fbf8) | ok | 170.9 | 58515 | 67.8 | 43/43 | 91.97 | 85.81 | 1682 |
| 2026-10-15 | [`e82d2b2`](https://github.com/smolquery/smolquery/commit/e82d2b2bea18987e8efb09d46ebeeed363849a98) | ok | 144.8 | 69064 | 25.4 | 43/43 | 90.16 | 86.40 | 1665 |
| 2026-10-13 | [`cab9cc7`](https://github.com/smolquery/smolquery/commit/cab9cc7211931ecbd2be1ae540b77b3e25bfc86c) | ok | 101.7 | 98296 | 29.8 | 43/43 | 57.02 | 53.51 | 1046 |
| 2026-10-13 | [`03e51fa`](https://github.com/smolquery/smolquery/commit/03e51faf724811af16e0b9889c1a0562984a8291) | ok | 141.3 | 70769 | 64.0 | 43/43 | 66.94 | 62.50 | 1215 |
| 2026-10-13 | [`0e1641c`](https://github.com/smolquery/smolquery/commit/0e1641c879bb27afea6e222cab36054f9da816da) | ok | 203.1 | 49229 | 65.6 | 43/43 | 96.64 | 90.93 | 1831 |
| 2026-10-13 | [`bdbcba4`](https://github.com/smolquery/smolquery/commit/bdbcba43dabc4d1dddfc437c0fdbc12af2a0b67a) | ok | 155.0 | 64511 | 64.2 | 43/43 | 89.30 | 85.19 | 1661 |
| 2026-10-13 | [`a8a561a`](https://github.com/smolquery/smolquery/commit/a8a561acada383ac4290324eb7c57203b5d8e5f3) | ok | 139.9 | 71492 | 15.7 | 43/43 | 78.78 | 70.66 | 1378 |
| 2026-10-13 | [`df8fe23`](https://github.com/smolquery/smolquery/commit/df8fe2317d7bdd35252b8a44d6d7a6e26ffd3fd9) | ok | 157.0 | 63683 | 70.1 | 43/43 | 88.08 | 84.80 | 1650 |
| 2026-10-12 | [`df8fe23`](https://github.com/smolquery/smolquery/commit/df8fe2317d7bdd35252b8a44d6d7a6e26ffd3fd9) | ok | 237.1 | 42173 | 71.6 | 43/43 | 134.25 | 127.36 | 2804 |

<details><summary>Per-query times, latest run</summary>

| query | first ms | hot ms | previous hot ms | change |
|---|---:|---:|---:|---:|
| Q1 | 363 | 369 | 365 | +1% |
| Q2 | 1697 | 1601 | 1482 | +8% |
| Q3 | 1687 | 1637 | 1517 | +8% |
| Q4 | 1642 | 1618 | 1499 | +8% |
| Q5 | 2694 | 2525 | 2425 | +4% |
| Q6 | 3018 | 2720 | 2711 | +0% |
| Q7 | 1689 | 1554 | 1522 | +2% |
| Q8 | 1730 | 1288 | 1222 | +5% |
| Q9 | 2111 | 2098 | 1977 | +6% |
| Q10 | 2189 | 2193 | 2035 | +8% |
| Q11 | 1694 | 1675 | 1603 | +5% |
| Q12 | 1727 | 1471 | 1643 | -10% |
| Q13 | 2125 | 2104 | 2074 | +1% |
| Q14 | 2510 | 2429 | 2432 | -0% |
| Q15 | 2139 | 2138 | 2054 | +4% |
| Q16 | 1858 | 1982 | 1727 | +15% |
| Q17 | 2508 | 2500 | 2356 | +6% |
| Q18 | 2525 | 2463 | 2288 | +8% |
| Q19 | 3104 | 3124 | 3132 | -0% |
| Q20 | 824 | 660 | 635 | +4% |
| Q21 | 2529 | 2156 | 2129 | +1% |
| Q22 | 2281 | 2065 | 2098 | -2% |
| Q23 | 3251 | 2746 | 2833 | -3% |
| Q24 | 2940 | 2183 | 2338 | -7% |
| Q25 | 750 | 755 | 788 | -4% |
| Q26 | 761 | 757 | 700 | +8% |
| Q27 | 760 | 738 | 791 | -7% |
| Q28 | 2151 | 2012 | 2074 | -3% |
| Q29 | 8382 | 7999 | 7510 | +7% |
| Q30 | 1768 | 1735 | 1572 | +10% |
| Q31 | 2189 | 2174 | 2010 | +8% |
| Q32 | 2306 | 2386 | 2213 | +8% |
| Q33 | 5017 | 4974 | 5075 | -2% |
| Q34 | 3447 | 3409 | 3494 | -2% |
| Q35 | 3459 | 3467 | 3606 | -4% |
| Q36 | 2166 | 2129 | 2040 | +4% |
| Q37 | 2318 | 2249 | 2275 | -1% |
| Q38 | 1995 | 1997 | 1780 | +12% |
| Q39 | 797 | 773 | 754 | +2% |
| Q40 | 996 | 908 | 1033 | -12% |
| Q41 | 719 | 679 | 660 | +3% |
| Q42 | 689 | 627 | 640 | -2% |
| Q43 | 717 | 660 | 691 | -4% |

</details>

<!-- results:end -->
