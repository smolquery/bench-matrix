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

[`f3b0d5d`](https://github.com/smolquery/smolquery/commit/f3b0d5d32e6dde598f54e6a1eaba65a42949e432) on 2026-10-29: **ok** ([workflow run](https://github.com/smolquery/bench-matrix/actions/runs/40878718242))

### History

| date | commit | status | load s | rows/s | seal s | passed | first sum s | hot sum s | hot geomean ms |
|---|---|---|---:|---:|---:|---:|---:|---:|---:|
| 2026-10-29 | [`f3b0d5d`](https://github.com/smolquery/smolquery/commit/f3b0d5d32e6dde598f54e6a1eaba65a42949e432) | ok | 169.6 | 58957 | 26.3 | 43/43 | 53.64 | 52.86 | 1013 |
| 2026-10-28 | [`d12dc63`](https://github.com/smolquery/smolquery/commit/d12dc630b4d1e25c3812432f0010d2d8952853da) | ok | 119.2 | 83869 | 18.2 | 43/43 | 74.66 | 73.46 | 1455 |
| 2026-10-28 | [`88ebd11`](https://github.com/smolquery/smolquery/commit/88ebd112cde29ec5d9aa99f48b357b84eab8aa1c) | ok | 149.9 | 66693 | 64.4 | 43/43 | 99.02 | 96.71 | 1891 |
| 2026-10-26 | [`77155b0`](https://github.com/smolquery/smolquery/commit/77155b07e1b359cc60ceac53d54f75fbae4de75f) | ok | 155.8 | 64188 | 65.2 | 43/43 | 111.06 | 107.86 | 2113 |
| 2026-10-26 | [`309425e`](https://github.com/smolquery/smolquery/commit/309425e8ee69198a531074e21398b6136b0054bf) | ok | 131.2 | 76223 | 64.2 | 43/43 | 88.61 | 86.13 | 1680 |
| 2026-10-24 | [`75584d6`](https://github.com/smolquery/smolquery/commit/75584d61160a7dab5bfe88b5dc8eca7696ab9a5a) | ok | 120.8 | 82754 | 68.4 | 43/43 | 85.42 | 83.35 | 1620 |
| 2026-10-24 | [`e607d60`](https://github.com/smolquery/smolquery/commit/e607d60c25050fe60085121e6edf31c4f1fa4024) | ok | 121.4 | 82340 | 62.7 | 43/43 | 84.81 | 82.60 | 1618 |
| 2026-10-24 | [`d3977f4`](https://github.com/smolquery/smolquery/commit/d3977f40be0295e5485d4b89cfe7a0644d9ba015) | ok | 153.2 | 65253 | 13.2 | 43/43 | 100.20 | 97.58 | 1921 |
| 2026-10-24 | [`19d0758`](https://github.com/smolquery/smolquery/commit/19d0758b48fca9103fc63cc7318d0b77cd9ccd52) | ok | 159.0 | 62902 | 58.3 | 43/43 | 101.20 | 98.48 | 1914 |
| 2026-10-24 | [`d171282`](https://github.com/smolquery/smolquery/commit/d17128269fb3a9a0fb47e224fb1b9851120337a4) | ok | 152.0 | 65801 | 64.1 | 43/43 | 98.93 | 98.27 | 1930 |
| 2026-10-24 | [`521fece`](https://github.com/smolquery/smolquery/commit/521feceb478e86d87c1f2d29954e621f9df4bd05) | ok | 157.1 | 63663 | 13.0 | 43/43 | 103.34 | 100.25 | 1968 |
| 2026-10-23 | [`2434d95`](https://github.com/smolquery/smolquery/commit/2434d95501910f86b8b2b2b0a0a4772e5f564795) | ok | 155.9 | 64128 | 64.7 | 43/43 | 100.26 | 96.45 | 1895 |
| 2026-10-23 | [`4b1e422`](https://github.com/smolquery/smolquery/commit/4b1e4220327f5db42b63d6e4a042ef7f24b879c9) | ok | 123.3 | 81079 | 68.3 | 43/43 | 87.44 | 81.68 | 1573 |
| 2026-10-23 | [`6f09296`](https://github.com/smolquery/smolquery/commit/6f092961fd8c3bcd6bf216064c46563ba9d7677b) | ok | 144.8 | 69054 | 64.2 | 43/43 | 90.61 | 88.70 | 1706 |
| 2026-10-23 | [`e7913f1`](https://github.com/smolquery/smolquery/commit/e7913f17469cbd3a5d395bd73ab8f346f221f751) | ok | 141.7 | 70595 | 64.0 | 43/43 | 88.81 | 86.73 | 1652 |
| 2026-10-20 | [`6aa484b`](https://github.com/smolquery/smolquery/commit/6aa484bcb527b0bdee9cc01df1b01d613b7f714e) | ok | 154.8 | 64585 | 94.9 | 43/43 | 45.51 | 43.25 | 824 |
| 2026-10-19 | [`8b2fe19`](https://github.com/smolquery/smolquery/commit/8b2fe1904d17d3f8bedf37108f6e33778dfcb294) | ok | 143.8 | 69549 | 30.6 | 43/43 | 38.90 | 36.37 | 704 |
| 2026-10-19 | [`e97db80`](https://github.com/smolquery/smolquery/commit/e97db80e9fb8cda91ab39a67d89dc37eeb8e477c) | ok | 119.9 | 83437 | 26.3 | 43/43 | 76.37 | 71.54 | 1386 |
| 2026-10-18 | [`e88220c`](https://github.com/smolquery/smolquery/commit/e88220c6c8fcd2a971fb946a6587ec5095e5b4bc) | load_failed | 0.0 | 0 | 0.0 | 0/0 | 0.00 | 0.00 | 0 |
| 2026-10-18 | [`c1d88b0`](https://github.com/smolquery/smolquery/commit/c1d88b02c47263ed2317551a876741b6ecb632a0) | ok | 156.3 | 63972 | 25.3 | 43/43 | 96.24 | 91.54 | 1772 |
| 2026-10-18 | [`76cea1a`](https://github.com/smolquery/smolquery/commit/76cea1a524f7d04ef2cf9d7310f0d076a4aeb11e) | ok | 140.0 | 71453 | 64.8 | 43/43 | 93.12 | 87.97 | 1700 |
| 2026-10-18 | [`761787f`](https://github.com/smolquery/smolquery/commit/761787fdcd3bfed79d502a5cddca4b50f4cfb3bc) | ok | 147.6 | 67770 | 30.8 | 43/43 | 38.99 | 36.98 | 716 |
| 2026-10-18 | [`83c63ad`](https://github.com/smolquery/smolquery/commit/83c63ad0427ad528f52ec879e6b0d9026b02c733) | ok | 145.8 | 68565 | 64.9 | 43/43 | 91.16 | 87.44 | 1709 |
| 2026-10-18 | [`aa7cc68`](https://github.com/smolquery/smolquery/commit/aa7cc68c6dd3d2e3c98cb1f82561060a798f80df) | ok | 144.0 | 69443 | 36.3 | 43/43 | 38.41 | 35.99 | 692 |
| 2026-10-18 | [`6e62333`](https://github.com/smolquery/smolquery/commit/6e62333b94d480df543d9b46214db410e435bdde) | ok | 143.5 | 69678 | 70.1 | 43/43 | 69.42 | 64.88 | 1254 |
| 2026-10-18 | [`6f28309`](https://github.com/smolquery/smolquery/commit/6f283093968432cebd6308a7b17d1cd86d29a501) | ok | 128.4 | 77901 | 18.2 | 43/43 | 64.47 | 61.08 | 1169 |
| 2026-10-17 | [`42b2f89`](https://github.com/smolquery/smolquery/commit/42b2f89c6b640ebccee0646ae2ad84acc29bdc43) | ok | 145.6 | 68701 | 24.4 | 43/43 | 39.19 | 36.55 | 706 |
| 2026-10-17 | [`5a257da`](https://github.com/smolquery/smolquery/commit/5a257daa5c726892b19f5933d6ab75b4bd47cb50) | ok | 152.1 | 65733 | 24.2 | 43/43 | 39.66 | 37.19 | 723 |
| 2026-10-16 | [`75776ec`](https://github.com/smolquery/smolquery/commit/75776ecb5a96042def8a7bc5c6f9ff45e42d5f5b) | ok | 146.5 | 68241 | 65.0 | 43/43 | 92.22 | 87.73 | 1716 |
| 2026-10-15 | [`4fb926b`](https://github.com/smolquery/smolquery/commit/4fb926bcd677161c130be6806c69f154cdc6fbf8) | ok | 170.9 | 58515 | 67.8 | 43/43 | 91.97 | 85.81 | 1682 |

<details><summary>Per-query times, latest run</summary>

| query | first ms | hot ms | previous hot ms | change |
|---|---:|---:|---:|---:|
| Q1 | 557 | 486 | 273 | +78% |
| Q2 | 540 | 516 | 1262 | -59% |
| Q3 | 541 | 526 | 1257 | -58% |
| Q4 | 622 | 554 | 1312 | -58% |
| Q5 | 633 | 649 | 2266 | -71% |
| Q6 | 757 | 785 | 2296 | -66% |
| Q7 | 474 | 482 | 1056 | -54% |
| Q8 | 521 | 485 | 1238 | -61% |
| Q9 | 685 | 675 | 1622 | -58% |
| Q10 | 786 | 786 | 1670 | -53% |
| Q11 | 633 | 542 | 1356 | -60% |
| Q12 | 611 | 590 | 1465 | -60% |
| Q13 | 773 | 830 | 1794 | -54% |
| Q14 | 934 | 885 | 2029 | -56% |
| Q15 | 791 | 772 | 1735 | -56% |
| Q16 | 684 | 654 | 1422 | -54% |
| Q17 | 932 | 944 | 2071 | -54% |
| Q18 | 880 | 898 | 1948 | -54% |
| Q19 | 1239 | 1246 | 2672 | -53% |
| Q20 | 1050 | 1007 | 644 | +56% |
| Q21 | 1443 | 1471 | 2103 | -30% |
| Q22 | 1486 | 1413 | 2076 | -32% |
| Q23 | 2408 | 2420 | 2975 | -19% |
| Q24 | 4127 | 4072 | 2678 | +52% |
| Q25 | 1381 | 1231 | 815 | +51% |
| Q26 | 1215 | 1168 | 725 | +61% |
| Q27 | 1273 | 1276 | 845 | +51% |
| Q28 | 1406 | 1406 | 2097 | -33% |
| Q29 | 7088 | 6982 | 5938 | +18% |
| Q30 | 619 | 582 | 1358 | -57% |
| Q31 | 831 | 907 | 1639 | -45% |
| Q32 | 1061 | 1076 | 1909 | -44% |
| Q33 | 1392 | 1438 | 3330 | -57% |
| Q34 | 1798 | 1767 | 3286 | -46% |
| Q35 | 1853 | 1826 | 3385 | -46% |
| Q36 | 711 | 625 | 1488 | -58% |
| Q37 | 1408 | 1407 | 883 | +59% |
| Q38 | 1295 | 1229 | 770 | +60% |
| Q39 | 1341 | 1299 | 759 | +71% |
| Q40 | 1590 | 1701 | 1054 | +61% |
| Q41 | 1147 | 1075 | 690 | +56% |
| Q42 | 1081 | 1129 | 638 | +77% |
| Q43 | 1047 | 1049 | 638 | +64% |

</details>

<!-- results:end -->
