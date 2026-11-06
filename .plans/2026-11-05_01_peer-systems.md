# Bench ClickHouse and VictoriaLogs next to smolquery

## Goal

Each bench run also benches ClickHouse and VictoriaLogs on the same runner, with the same data. The results show next to the smolquery results.

## Decisions

- **One system at a time.** smolquery runs first, so its history stays comparable. The harness removes the stack and its volumes before the next system starts.
- **Pinned peer versions.** `compose.yaml` pins `clickhouse/clickhouse-server:26.10.2.103` and `victoriametrics/victoria-logs:v1.53.0`. A peer number then changes only with the runner or a deliberate version change, so the peers also show runner noise.
- **Same ingest path.** All systems get the same NDJSON from the DuckDB conversion: 24 MB bodies over 8 concurrent HTTP requests. ClickHouse uses `FORMAT JSONEachRow`. VictoriaLogs uses `/insert/jsonline`.
- **Same memory limit.** Each peer container gets the 12 GB limit.
- **Upstream queries and schema.** The peer queries and the ClickHouse table come from ClickHouse/ClickBench at `ff15af5`. LogsQL is a translation, so answers can differ. Local validation compares them.
- **The seal column for peers.** The peers merge during the load, so the merge time is in the load time. The harness waits for merges to stop, which keeps them out of the query timings. The latest-run table leaves the peer seal cell blank and adds `total s` (load plus seal).
- **A peer failure does not stop the publish.** The workflow publishes all results, then fails.
- **Known difference.** smolquery reads from object storage with a file cache. The peers read from a local Docker volume. The README states this.

## Steps

- [x] Fast-forward local `main` (40 result commits behind).
- [x] Add `suites/clickbench/clickhouse` and `suites/clickbench/victorialogs`.
- [x] Add the two services to `compose.yaml`.
- [x] Add a `-system` flag and a `system` interface to `bench load` and `bench query`.
- [x] Publish per-system results, history and README tables. Old history lines must still render.
- [x] Replace the per-bench workflow steps with `scripts/bench.sh <system>`.
- [x] Validate locally with all three systems. Use a scratch results directory: local runs must not enter the history.
- [x] Compare the query answers between systems, and record the differences here.
- [x] Update the README.
- [x] Commit and push.
- [x] Confirm the first GitHub runner result: run 42111347768 passed in 16 minutes.

## Findings

Local run, M1 Mac, 8 CPUs, smolquery `4a72dc3`, 10M rows. These numbers are not in the history.

| system | load rows/s | seal s | passed | first sum s | hot sum s | hot geomean ms |
|---|---:|---:|---:|---:|---:|---:|
| smolquery | 54755 | 63.0 | 43/43 | 46.32 | 43.24 | 912 |
| ClickHouse | 74915 | 0.0 | 43/43 | 2.63 | 1.93 | 23 |
| VictoriaLogs | 73271 | 0.0 | 43/43 | 15.05 | 14.04 | 94 |

A second load gave 63392, 71356 and 65874 rows/s, so ingest noise is about 15%.

Answers against smolquery: ClickHouse matches 32 of 43, VictoriaLogs 30 of 43.

- Q4, ClickHouse: the `Int64` sum overflows. A wrapped sum gives -6.533e11, the same as the ClickHouse answer. DuckDB on the Parquet gives 2.513e18, the same as smolquery and VictoriaLogs.
- Q6, VictoriaLogs: `count_uniq` does not count the empty string (835092, not 835093).
- Q23 and Q26, VictoriaLogs: `min` and sort use a different string order.
- Q29, smolquery: `.` does not match a newline. DuckDB gives 113920 without the `s` flag and 113921 with it, and the peers give 113921. 266 `Referer` values hold a newline.
- Q18: no `ORDER BY`, so any 10 rows are valid.
- Q22, Q32, Q33, Q39, Q40, Q41: the sort values are equal between systems, so the rows differ only on ties.
- Q24: the 10 `EventTime` values are equal between systems.
- Q43: the counts are equal between systems. Only the time format differs.

Other findings:

- ClickHouse held 6 active parts after the load, so merges kept up with ingest. The 0.0 s seal is real.
- VictoriaLogs answers 200 and stores nothing when the content type is a form type. The harness sends `application/stream+json`, and the row count check catches a repeat.
- Every smolquery query except Q1 takes at least 560 ms, against 1 to 202 ms for the fastest peer on each query. The cause is not known.
- VS Code holds ports 4000, 8123 and 18123 on the local machine, and `curl` with no timeout hangs on them. `up.sh` now uses `--max-time`.

## GitHub runner result

Run 42111347768, 4 vCPU, 16 GB, smolquery `4a72dc3`. All steps passed in 16 minutes: smolquery 7.7, ClickHouse 2.5, VictoriaLogs 4.8.

| system | load rows/s | seal s | passed | first sum s | hot sum s | hot geomean ms |
|---|---:|---:|---:|---:|---:|---:|
| smolquery | 73100 | 13.3 | 43/43 | 91.94 | 89.64 | 1772 |
| ClickHouse | 102836 | 0.0 | 43/43 | 4.95 | 3.90 | 36 |
| VictoriaLogs | 57672 | 0.0 | 43/43 | 34.99 | 33.64 | 187 |

The same smolquery commit had a hot sum of 107.30 s on 2026-11-02, so runner noise between runs can reach about 16%.
