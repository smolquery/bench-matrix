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

[`76cea1a`](https://github.com/smolquery/smolquery/commit/76cea1a524f7d04ef2cf9d7310f0d076a4aeb11e) on 2026-10-18: **ok** ([workflow run](https://github.com/smolquery/bench-matrix/actions/runs/39153224354))

### History

| date | commit | status | load s | rows/s | seal s | passed | first sum s | hot sum s | hot geomean ms |
|---|---|---|---:|---:|---:|---:|---:|---:|---:|
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
| Q1 | 361 | 350 | 390 | -10% |
| Q2 | 1665 | 1510 | 396 | +281% |
| Q3 | 1669 | 1597 | 399 | +300% |
| Q4 | 1645 | 1546 | 414 | +274% |
| Q5 | 2506 | 2568 | 505 | +409% |
| Q6 | 2786 | 2694 | 632 | +326% |
| Q7 | 1671 | 1636 | 389 | +320% |
| Q8 | 1625 | 1582 | 392 | +303% |
| Q9 | 2198 | 2070 | 561 | +269% |
| Q10 | 2151 | 2186 | 638 | +243% |
| Q11 | 1682 | 1745 | 440 | +297% |
| Q12 | 1777 | 1431 | 453 | +216% |
| Q13 | 2457 | 2041 | 605 | +237% |
| Q14 | 2453 | 2411 | 737 | +227% |
| Q15 | 2182 | 2232 | 630 | +254% |
| Q16 | 1902 | 1849 | 529 | +249% |
| Q17 | 2629 | 2573 | 748 | +244% |
| Q18 | 2443 | 2471 | 679 | +264% |
| Q19 | 3301 | 3155 | 972 | +225% |
| Q20 | 769 | 610 | 683 | -11% |
| Q21 | 2459 | 2160 | 880 | +145% |
| Q22 | 2132 | 2126 | 821 | +159% |
| Q23 | 3324 | 2638 | 1365 | +93% |
| Q24 | 2422 | 1902 | 2222 | -14% |
| Q25 | 726 | 779 | 778 | +0% |
| Q26 | 739 | 729 | 735 | -1% |
| Q27 | 739 | 734 | 772 | -5% |
| Q28 | 2213 | 2175 | 860 | +153% |
| Q29 | 9631 | 8420 | 6075 | +39% |
| Q30 | 1664 | 1758 | 426 | +312% |
| Q31 | 2265 | 2099 | 651 | +222% |
| Q32 | 2461 | 2330 | 709 | +228% |
| Q33 | 4938 | 5085 | 1195 | +326% |
| Q34 | 3451 | 3484 | 1195 | +192% |
| Q35 | 3811 | 3509 | 1262 | +178% |
| Q36 | 2160 | 2145 | 617 | +248% |
| Q37 | 2226 | 2220 | 823 | +170% |
| Q38 | 2094 | 2024 | 793 | +155% |
| Q39 | 751 | 669 | 724 | -8% |
| Q40 | 969 | 900 | 954 | -6% |
| Q41 | 677 | 641 | 650 | -1% |
| Q42 | 614 | 578 | 657 | -12% |
| Q43 | 783 | 612 | 621 | -2% |

</details>

<!-- results:end -->
