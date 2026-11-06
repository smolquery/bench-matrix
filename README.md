# bench-matrix

bench-matrix benchmarks every commit that lands on the `main` branch of
[smolquery/smolquery](https://github.com/smolquery/smolquery). Each run also
benches [ClickHouse](https://github.com/ClickHouse/ClickHouse) and
[VictoriaLogs](https://github.com/VictoriaMetrics/VictoriaLogs) on the same
machine. Each run commits its results to this repository, so the history stays
in git.

## What runs

A run benches three systems, one at a time, in this order: smolquery,
ClickHouse, VictoriaLogs. Each system runs two benches on the
[ClickBench](https://github.com/ClickHouse/ClickBench) `hits` dataset. The
harness removes each stack and its volumes before the next system starts.

| bench | what it does | what it measures |
|---|---|---|
| **load** | Creates the table (105 columns), inserts 10M rows through the NDJSON insert route of the system, then waits for background work to stop. | Ingest time and rows/s until the last insert is acknowledged. Seal time until background work stops. An exact row count check. The bench fails if the count is wrong or the background work does not stop in 20 minutes. |
| **query** | Runs the 43 ClickBench queries, 3 tries each, over HTTP. | The first try and the best of the other two tries ("hot"), per query. The sums and the geometric mean of the hot times. |

### smolquery

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
machine. That conversion shares the CPU with the system under test during the
load.

The seal time for smolquery ends when the hot tier is empty. smolquery does this
work after it acknowledges the inserts.

### ClickHouse and VictoriaLogs

Each peer is one container on a local Docker volume, with the same 12 GB limit
as smolquery. The peers use default settings, plus the settings in this table.
`compose.yaml` pins each version, so a peer number changes only with the runner
or a deliberate version change. Thus the peers also show the runner noise.

| system | image | insert route | harness waits until | settings |
|---|---|---|---|---|
| ClickHouse | `clickhouse/clickhouse-server:26.10.2.103` | `FORMAT JSONEachRow` over HTTP | `system.merges` is empty | The ClickBench table: `MergeTree`, `fsync_after_insert = 1`. |
| VictoriaLogs | `victoriametrics/victoria-logs:v1.53.0` | `/insert/jsonline` | `vl_active_merges` is 0 after a flush | `-retentionPeriod=20y`, `-search.maxQueryDuration=5m`. |

All systems get the same NDJSON: 24 MB bodies over 8 concurrent requests. The
peer table and queries come from ClickBench commit
[`ff15af5`](https://github.com/ClickHouse/ClickBench/tree/ff15af5d8aae5ce5ce79b8e479379582f9050fdc).

The peers merge in the background during the load, so the merge time is in their
load time. The latest-run table leaves the seal cell blank for a peer, because no
seal step exists to measure. The harness still waits for merges to stop, which
keeps them out of the query timings. To compare the systems, use `total s`: the
load time plus the seal time.

To change a peer version, change its image in `compose.yaml`. The results show
the version of each peer.

### Limits of the comparison

- **Storage:** smolquery reads from object storage through a file cache. The
  peers read from a local volume.
- **Memory:** the 12 GB limit does not include the Postgres and RustFS
  containers that smolquery needs.
- **Answers:** the harness does not compare answers in a run. A manual check at
  10M rows found the differences in the next list.
- **Ingest:** NDJSON is the common route, not the fastest route. ClickBench
  loads ClickHouse from Parquet directly.

The VictoriaLogs queries are LogsQL translations, not the same SQL. The manual
check found these real differences in the answers:

| query | system | difference |
|---|---|---|
| Q4 | ClickHouse | `AVG(UserID)` returns about -6.5e11, not 2.5e18. The `Int64` sum overflows. DuckDB on the source Parquet gives 2.5e18. |
| Q6 | VictoriaLogs | `count_uniq` does not count the empty string, so the result is 1 lower. |
| Q23, Q26 | VictoriaLogs | `min` and sort use a different string order, so some strings differ. |
| Q29 | smolquery | `.` does not match a newline in the regular expression, so some counts are lower. 266 `Referer` values hold a newline. |

Other queries can return different rows when rows tie under `LIMIT`, or when a
query has no `ORDER BY` (Q18). The time formats also differ.

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
  `query.json` for one run. The `clickhouse/` and `victorialogs/` directories
  hold `load.json` and `query.json` for each peer.
- The section below: the latest run for each system, the last 30 runs, and
  per-query times against the previous run and the peers. `bench publish`
  rewrites it after each run.
- Each workflow run also uploads `out/` with the container logs of each system
  as an artifact.

A peer failure does not stop the publish. The workflow publishes all results,
then fails. The status column of the latest run shows which system failed.

## Run it locally

You need Docker, Go 1.26, `curl` and `unzip`.

1. Pick an image: `export SMOLQUERY_IMAGE=ghcr.io/smolquery/smolquery:sha-<full sha>`.
2. Run the benches: `FILES=10 scripts/run.sh`. The results go to
   `out/<system>/`. The harness stops each stack when its benches finish.

`FILES` sets the number of 1M-row parquet files (default 10). `SYSTEMS` sets the
systems to bench, for example `SYSTEMS=clickhouse scripts/run.sh`. The ingest
variables in `compose.yaml` read from the environment, so you can override
them for a local run. If a port is in use, set `SMOLQUERY_PORT` (4000),
`RUSTFS_PORT` (9000), `CLICKHOUSE_PORT` (8123) or `VICTORIALOGS_PORT` (9428).

## Layout

| path | contents |
|---|---|
| `cmd/bench` | The Go harness: `bench load`, `bench query`, `bench publish`. `system.go` holds the adapter for each system. |
| `suites/clickbench` | The smolquery table definition and queries. `clickhouse/` and `victorialogs/` hold the peer versions. |
| `scripts` | `fetch.sh` (data and DuckDB CLI), `up.sh`, `down.sh`, `bench.sh` (one system), `run.sh` (all systems). |
| `compose.yaml` | Postgres, RustFS, smolquery, ClickHouse and VictoriaLogs. |
| `.github/workflows/bench.yml` | The CI workflow. |

## Results

<!-- results:start -->
### Latest run

[`5189c5b`](https://github.com/smolquery/smolquery/commit/5189c5bc2aef12a1e2fb5dfd866bf0fcb1afb368) on 2026-11-06: **ok** ([workflow run](https://github.com/smolquery/bench-matrix/actions/runs/42265343156))

| system | version | status | load s | rows/s | seal s | total s | passed | first sum s | hot sum s | hot geomean ms |
|---|---|---|---:|---:|---:|---:|---:|---:|---:|---:|
| smolquery | [`5189c5b`](https://github.com/smolquery/smolquery/commit/5189c5bc2aef12a1e2fb5dfd866bf0fcb1afb368) | ok | 120.9 | 82703 | 69.5 | 190.4 | 43/43 | 33.54 | 33.35 | 633 |
| ClickHouse | 26.10.2.103 | ok | 82.2 | 121726 |  | 82.2 | 43/43 | 4.19 | 3.31 | 32 |
| VictoriaLogs | v1.53.0 | ok | 145.9 | 68552 |  | 145.9 | 43/43 | 28.34 | 27.27 | 147 |

### History

| date | commit | status | load s | rows/s | seal s | passed | first sum s | hot sum s | hot geomean ms | ClickHouse hot sum s | VictoriaLogs hot sum s |
|---|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| 2026-11-06 | [`5189c5b`](https://github.com/smolquery/smolquery/commit/5189c5bc2aef12a1e2fb5dfd866bf0fcb1afb368) | ok | 120.9 | 82703 | 69.5 | 43/43 | 33.54 | 33.35 | 633 | 3.31 | 27.27 |
| 2026-11-05 | [`4a72dc3`](https://github.com/smolquery/smolquery/commit/4a72dc3b4a81318fe9ef5d94fa13fb2ee91b9ecc) | ok | 136.8 | 73100 | 13.3 | 43/43 | 91.94 | 89.64 | 1772 | 3.90 | 33.64 |
| 2026-11-02 | [`4a72dc3`](https://github.com/smolquery/smolquery/commit/4a72dc3b4a81318fe9ef5d94fa13fb2ee91b9ecc) | ok | 168.2 | 59471 | 20.0 | 43/43 | 109.39 | 107.30 | 2142 |  |  |
| 2026-11-01 | [`9e15395`](https://github.com/smolquery/smolquery/commit/9e153953c61e06847301103db4b70ebdb0dcca23) | ok | 174.0 | 57485 | 56.4 | 43/43 | 115.11 | 112.43 | 2269 |  |  |
| 2026-10-30 | [`d675422`](https://github.com/smolquery/smolquery/commit/d6754225a32fdeb4e7188263728fe2476bbe28cf) | load_failed | 0.0 | 0 | 0.0 | 0/0 | 0.00 | 0.00 | 0 |  |  |
| 2026-10-29 | [`5dc2144`](https://github.com/smolquery/smolquery/commit/5dc21443fa37ae2ab8cef41e0e9a6cf0f1bf00b0) | load_failed | 0.0 | 0 | 0.0 | 0/0 | 0.00 | 0.00 | 0 |  |  |
| 2026-10-29 | [`8930223`](https://github.com/smolquery/smolquery/commit/8930223fa562009fe73c41f658aad90da6e715ba) | load_failed | 0.0 | 0 | 0.0 | 0/0 | 0.00 | 0.00 | 0 |  |  |
| 2026-10-29 | [`f3b0d5d`](https://github.com/smolquery/smolquery/commit/f3b0d5d32e6dde598f54e6a1eaba65a42949e432) | ok | 169.6 | 58957 | 26.3 | 43/43 | 53.64 | 52.86 | 1013 |  |  |
| 2026-10-28 | [`d12dc63`](https://github.com/smolquery/smolquery/commit/d12dc630b4d1e25c3812432f0010d2d8952853da) | ok | 119.2 | 83869 | 18.2 | 43/43 | 74.66 | 73.46 | 1455 |  |  |
| 2026-10-28 | [`88ebd11`](https://github.com/smolquery/smolquery/commit/88ebd112cde29ec5d9aa99f48b357b84eab8aa1c) | ok | 149.9 | 66693 | 64.4 | 43/43 | 99.02 | 96.71 | 1891 |  |  |
| 2026-10-26 | [`77155b0`](https://github.com/smolquery/smolquery/commit/77155b07e1b359cc60ceac53d54f75fbae4de75f) | ok | 155.8 | 64188 | 65.2 | 43/43 | 111.06 | 107.86 | 2113 |  |  |
| 2026-10-26 | [`309425e`](https://github.com/smolquery/smolquery/commit/309425e8ee69198a531074e21398b6136b0054bf) | ok | 131.2 | 76223 | 64.2 | 43/43 | 88.61 | 86.13 | 1680 |  |  |
| 2026-10-24 | [`75584d6`](https://github.com/smolquery/smolquery/commit/75584d61160a7dab5bfe88b5dc8eca7696ab9a5a) | ok | 120.8 | 82754 | 68.4 | 43/43 | 85.42 | 83.35 | 1620 |  |  |
| 2026-10-24 | [`e607d60`](https://github.com/smolquery/smolquery/commit/e607d60c25050fe60085121e6edf31c4f1fa4024) | ok | 121.4 | 82340 | 62.7 | 43/43 | 84.81 | 82.60 | 1618 |  |  |
| 2026-10-24 | [`d3977f4`](https://github.com/smolquery/smolquery/commit/d3977f40be0295e5485d4b89cfe7a0644d9ba015) | ok | 153.2 | 65253 | 13.2 | 43/43 | 100.20 | 97.58 | 1921 |  |  |
| 2026-10-24 | [`19d0758`](https://github.com/smolquery/smolquery/commit/19d0758b48fca9103fc63cc7318d0b77cd9ccd52) | ok | 159.0 | 62902 | 58.3 | 43/43 | 101.20 | 98.48 | 1914 |  |  |
| 2026-10-24 | [`d171282`](https://github.com/smolquery/smolquery/commit/d17128269fb3a9a0fb47e224fb1b9851120337a4) | ok | 152.0 | 65801 | 64.1 | 43/43 | 98.93 | 98.27 | 1930 |  |  |
| 2026-10-24 | [`521fece`](https://github.com/smolquery/smolquery/commit/521feceb478e86d87c1f2d29954e621f9df4bd05) | ok | 157.1 | 63663 | 13.0 | 43/43 | 103.34 | 100.25 | 1968 |  |  |
| 2026-10-23 | [`2434d95`](https://github.com/smolquery/smolquery/commit/2434d95501910f86b8b2b2b0a0a4772e5f564795) | ok | 155.9 | 64128 | 64.7 | 43/43 | 100.26 | 96.45 | 1895 |  |  |
| 2026-10-23 | [`4b1e422`](https://github.com/smolquery/smolquery/commit/4b1e4220327f5db42b63d6e4a042ef7f24b879c9) | ok | 123.3 | 81079 | 68.3 | 43/43 | 87.44 | 81.68 | 1573 |  |  |
| 2026-10-23 | [`6f09296`](https://github.com/smolquery/smolquery/commit/6f092961fd8c3bcd6bf216064c46563ba9d7677b) | ok | 144.8 | 69054 | 64.2 | 43/43 | 90.61 | 88.70 | 1706 |  |  |
| 2026-10-23 | [`e7913f1`](https://github.com/smolquery/smolquery/commit/e7913f17469cbd3a5d395bd73ab8f346f221f751) | ok | 141.7 | 70595 | 64.0 | 43/43 | 88.81 | 86.73 | 1652 |  |  |
| 2026-10-20 | [`6aa484b`](https://github.com/smolquery/smolquery/commit/6aa484bcb527b0bdee9cc01df1b01d613b7f714e) | ok | 154.8 | 64585 | 94.9 | 43/43 | 45.51 | 43.25 | 824 |  |  |
| 2026-10-19 | [`8b2fe19`](https://github.com/smolquery/smolquery/commit/8b2fe1904d17d3f8bedf37108f6e33778dfcb294) | ok | 143.8 | 69549 | 30.6 | 43/43 | 38.90 | 36.37 | 704 |  |  |
| 2026-10-19 | [`e97db80`](https://github.com/smolquery/smolquery/commit/e97db80e9fb8cda91ab39a67d89dc37eeb8e477c) | ok | 119.9 | 83437 | 26.3 | 43/43 | 76.37 | 71.54 | 1386 |  |  |
| 2026-10-18 | [`e88220c`](https://github.com/smolquery/smolquery/commit/e88220c6c8fcd2a971fb946a6587ec5095e5b4bc) | load_failed | 0.0 | 0 | 0.0 | 0/0 | 0.00 | 0.00 | 0 |  |  |
| 2026-10-18 | [`c1d88b0`](https://github.com/smolquery/smolquery/commit/c1d88b02c47263ed2317551a876741b6ecb632a0) | ok | 156.3 | 63972 | 25.3 | 43/43 | 96.24 | 91.54 | 1772 |  |  |
| 2026-10-18 | [`76cea1a`](https://github.com/smolquery/smolquery/commit/76cea1a524f7d04ef2cf9d7310f0d076a4aeb11e) | ok | 140.0 | 71453 | 64.8 | 43/43 | 93.12 | 87.97 | 1700 |  |  |
| 2026-10-18 | [`761787f`](https://github.com/smolquery/smolquery/commit/761787fdcd3bfed79d502a5cddca4b50f4cfb3bc) | ok | 147.6 | 67770 | 30.8 | 43/43 | 38.99 | 36.98 | 716 |  |  |
| 2026-10-18 | [`83c63ad`](https://github.com/smolquery/smolquery/commit/83c63ad0427ad528f52ec879e6b0d9026b02c733) | ok | 145.8 | 68565 | 64.9 | 43/43 | 91.16 | 87.44 | 1709 |  |  |

<details><summary>Per-query times, latest run</summary>

| query | first ms | hot ms | previous hot ms | change | ClickHouse hot ms | VictoriaLogs hot ms |
|---|---:|---:|---:|---:|---:|---:|
| Q1 | 306 | 295 | 342 | -14% | 1 | 1 |
| Q2 | 360 | 312 | 1662 | -81% | 1 | 1 |
| Q3 | 334 | 318 | 1627 | -80% | 8 | 44 |
| Q4 | 363 | 373 | 1724 | -78% | 9 | 49 |
| Q5 | 414 | 367 | 2776 | -87% | 38 | 210 |
| Q6 | 497 | 513 | 2898 | -82% | 120 | 254 |
| Q7 | 294 | 275 | 1615 | -83% | 3 | 5 |
| Q8 | 314 | 300 | 1712 | -82% | 6 | 1 |
| Q9 | 416 | 420 | 2010 | -79% | 69 | 369 |
| Q10 | 465 | 477 | 2094 | -77% | 85 | 741 |
| Q11 | 331 | 347 | 1758 | -80% | 18 | 67 |
| Q12 | 378 | 362 | 1765 | -79% | 22 | 84 |
| Q13 | 487 | 496 | 2205 | -77% | 87 | 295 |
| Q14 | 580 | 556 | 2413 | -77% | 108 | 611 |
| Q15 | 508 | 457 | 2216 | -79% | 102 | 358 |
| Q16 | 384 | 404 | 1832 | -78% | 42 | 291 |
| Q17 | 595 | 584 | 2424 | -76% | 157 | 937 |
| Q18 | 565 | 589 | 2283 | -74% | 7 | 1054 |
| Q19 | 753 | 792 | 2971 | -73% | 228 | 1996 |
| Q20 | 637 | 673 | 770 | -13% | 2 | 4 |
| Q21 | 867 | 925 | 2459 | -62% | 60 | 249 |
| Q22 | 909 | 982 | 2529 | -61% | 22 | 245 |
| Q23 | 1556 | 1586 | 3470 | -54% | 196 | 298 |
| Q24 | 2780 | 2884 | 3113 | -7% | 52 | 291 |
| Q25 | 842 | 770 | 895 | -14% | 6 | 120 |
| Q26 | 739 | 706 | 859 | -18% | 47 | 99 |
| Q27 | 764 | 815 | 876 | -7% | 10 | 127 |
| Q28 | 930 | 931 | 2458 | -62% | 36 | 235 |
| Q29 | 4117 | 4099 | 7813 | -48% | 384 | 3660 |
| Q30 | 351 | 333 | 1727 | -81% | 14 | 1914 |
| Q31 | 537 | 555 | 2141 | -74% | 63 | 645 |
| Q32 | 679 | 692 | 2310 | -70% | 66 | 930 |
| Q33 | 908 | 920 | 4000 | -77% | 178 | 6825 |
| Q34 | 1202 | 1192 | 3838 | -69% | 398 | 1367 |
| Q35 | 1239 | 1178 | 3798 | -69% | 390 | 1527 |
| Q36 | 445 | 408 | 1921 | -79% | 42 | 922 |
| Q37 | 883 | 794 | 954 | -17% | 47 | 85 |
| Q38 | 768 | 723 | 923 | -22% | 26 | 33 |
| Q39 | 827 | 847 | 921 | -8% | 24 | 24 |
| Q40 | 1046 | 1113 | 1205 | -8% | 110 | 241 |
| Q41 | 766 | 673 | 804 | -16% | 12 | 19 |
| Q42 | 688 | 670 | 737 | -9% | 9 | 18 |
| Q43 | 716 | 644 | 788 | -18% | 7 | 17 |

</details>

<!-- results:end -->
