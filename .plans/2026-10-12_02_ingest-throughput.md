# Ingest throughput above 60k rows/s

## Question

The load bench ingests 10M rows at about 60k rows/s locally and 42k rows/s on a GitHub runner. Find the bottleneck. Make the bench as fast as it can be on a GitHub runner (4 vCPU, 16 GB). Larger batches are fine.

## Findings

- The DuckDB conversion is not the limit: one thread converts 1M rows in 7.6 s (about 132k rows/s).
- The load is CPU-bound: smolquery uses about 290% and the client about 75% of 4 pinned CPUs.
- RowBinary is not cheaper: smolquery transcodes it to NDJSON on the ingest node.

## Sweep (3M rows, all containers and the client pinned to 4 CPUs on an M1 Max)

| config | rows/s | seal s |
|---|---:|---:|
| baseline: 6 MB bodies, 8 workers | 69k | 17 |
| 16 workers | 73k | 18 |
| 4 workers | 70k | 18 |
| 24 MB bodies (32 MB cap) | 103k | 21 |
| **24 MB bodies + 32 MB flush trigger, 256 MB buffer** | **109k** | 16 |
| 4 table partitions | 82k | 66 |

The user accepted about 100k rows/s and stopped the sweep before the 48 MB and 64 MB runs.

## Steps

- [x] Measure the DuckDB conversion alone.
- [x] Measure the server under a 4-CPU pin: workers, body size, flush size, partitions.
- [x] Set the defaults: `-batch-bytes 24000000`, `SMOLQUERY_INSERT_MAX_NDJSON_BYTES=32000000`, `SMOLQUERY_FLUSH_MAX_BYTES=32000000`, `SMOLQUERY_MAX_BUFFERED_BYTES=256000000`.
- [ ] Confirm on a GitHub runner (baseline there: 42k rows/s).
