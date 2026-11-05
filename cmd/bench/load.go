package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

const maxAttempts = 30

type loadConfig struct {
	system       string
	api          string
	ref          string
	schema       string
	parquetDir   string
	files        int
	pipelines    int
	workers      int
	batchBytes   int
	duckdb       string
	duckThreads  int
	duckMemory   string
	partitions   int
	sealWait     time.Duration
	out          string
	insertPrefix string
}

type tableDef struct {
	Clustering []string          `json:"clustering"`
	Schema     []json.RawMessage `json:"schema"`
}

type batch struct {
	file  int
	index int
	rows  int64
	raw   []byte
}

type loadStats struct {
	mu           sync.Mutex
	latenciesMs  []float64
	statuses     map[int]int64
	rowsRead     atomic.Int64
	rowsAccepted atomic.Int64
	rowsRejected atomic.Int64
	rowsFailed   atomic.Int64
	bytesSent    atomic.Int64
	requests     atomic.Int64
	retries429   atomic.Int64
	retriesOther atomic.Int64
}

func runLoad(args []string) {
	fs := flag.NewFlagSet("load", flag.ExitOnError)
	cfg := loadConfig{}
	fs.StringVar(&cfg.system, "system", "smolquery", "system under test: smolquery, clickhouse or victorialogs")
	fs.StringVar(&cfg.api, "api", "", "API base URL (default: the system's local port)")
	fs.StringVar(&cfg.ref, "ref", "", "table to load (default: the system's table)")
	fs.StringVar(&cfg.schema, "schema", "", "table definition (default: the system's definition in suites/clickbench)")
	fs.StringVar(&cfg.parquetDir, "parquet", "data/hits", "directory holding hits_<n>.parquet")
	fs.IntVar(&cfg.files, "files", 10, "parquet files to load, hits_0 up to hits_<files-1>")
	fs.IntVar(&cfg.pipelines, "pipelines", 2, "parquet files converted at once")
	fs.IntVar(&cfg.workers, "workers", 8, "concurrent insert requests")
	fs.IntVar(&cfg.batchBytes, "batch-bytes", 24_000_000, "NDJSON bytes per insert request")
	fs.StringVar(&cfg.duckdb, "duckdb", "bin/duckdb", "duckdb CLI that converts parquet to NDJSON")
	fs.IntVar(&cfg.duckThreads, "duckdb-threads", 1, "threads per duckdb conversion")
	fs.StringVar(&cfg.duckMemory, "duckdb-memory", "1GB", "memory limit per duckdb conversion")
	fs.IntVar(&cfg.partitions, "partitions", 0, "table write partitions (0 = deployment default)")
	fs.DurationVar(&cfg.sealWait, "seal-wait", 20*time.Minute, "longest wait for the hot tier to seal")
	fs.StringVar(&cfg.out, "out", "out/load.json", "result JSON path")
	fs.StringVar(&cfg.insertPrefix, "insert-prefix", fmt.Sprintf("bench-%d", time.Now().Unix()), "insertId prefix")
	fs.Parse(args)

	defaults := defaultsFor(cfg.system)
	if cfg.api == "" {
		cfg.api = defaults.api
	}
	if cfg.ref == "" {
		cfg.ref = defaults.ref
	}
	if cfg.schema == "" {
		cfg.schema = defaults.schema
	}
	sys := newSystem(cfg.system, cfg.api, cfg.ref, 2*time.Minute)
	sys.createTable(cfg.schema, cfg.partitions)

	log.Printf("load: %d file(s) into %s, %d pipeline(s), %d worker(s), %d bytes per request",
		cfg.files, cfg.system, cfg.pipelines, cfg.workers, cfg.batchBytes)
	ingest := ingestFiles(cfg, sys)
	log.Printf("load: %d rows accepted in %.1fs (%.0f rows/s), %d rejected, %d failed",
		ingest["rows_accepted"], ingest["duration_s"], ingest["rows_per_s"], ingest["rows_rejected"], ingest["rows_failed"])

	left, settled := sys.settle(cfg.sealWait)
	sealS := settled.Seconds()
	log.Printf("load: %d unit(s) of background work left after %.1fs", left, sealS)

	count, err := sys.countRows()
	if err != nil {
		log.Printf("load: count failed: %v", err)
	}
	expected := int64(cfg.files) * 1_000_000
	log.Printf("load: counted %d rows, expected %d", count, expected)

	result := map[string]any{
		"system":         cfg.system,
		"version":        sys.version(),
		"table":          cfg.ref,
		"files":          cfg.files,
		"rows_expected":  expected,
		"rows_counted":   count,
		"ingest":         ingest,
		"seal_s":         sealS,
		"hot_files_left": left,
		"total_s":        ingest["duration_s"].(float64) + sealS,
		"config": map[string]any{
			"pipelines":      cfg.pipelines,
			"workers":        cfg.workers,
			"batch_bytes":    cfg.batchBytes,
			"duckdb_threads": cfg.duckThreads,
			"partitions":     cfg.partitions,
		},
	}
	writeJSON(cfg.out, result)

	if count != expected {
		fatalf("load: counted %d rows, expected %d", count, expected)
	}
	if left != 0 {
		fatalf("load: background work did not finish within %s (%d unit(s) left)", cfg.sealWait, left)
	}
}

func ingestFiles(cfg loadConfig, sys system) map[string]any {
	st := &loadStats{statuses: map[int]int64{}}
	jobs := make(chan batch, cfg.workers*2)
	httpClient := &http.Client{
		Timeout: 2 * time.Minute,
		Transport: &http.Transport{
			MaxIdleConns:        cfg.workers * 2,
			MaxIdleConnsPerHost: cfg.workers * 2,
			DisableCompression:  true,
		},
	}

	started := time.Now()
	var workers sync.WaitGroup
	for range cfg.workers {
		workers.Go(func() {
			for b := range jobs {
				post(httpClient, sys, cfg.insertPrefix, b, st)
			}
		})
	}

	files := make(chan int)
	var converters sync.WaitGroup
	for range cfg.pipelines {
		converters.Go(func() {
			for i := range files {
				if err := convertFile(cfg, i, jobs, st); err != nil {
					log.Fatalf("load: hits_%d.parquet: %v", i, err)
				}
			}
		})
	}

	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				elapsed := time.Since(started).Seconds()
				log.Printf("load: %d rows accepted, %.0f rows/s, %d requests, %d x 429",
					st.rowsAccepted.Load(), float64(st.rowsAccepted.Load())/elapsed, st.requests.Load(), st.retries429.Load())
			}
		}
	}()

	for i := range cfg.files {
		files <- i
	}
	close(files)
	converters.Wait()
	close(jobs)
	workers.Wait()
	close(done)

	return st.summary(time.Since(started).Seconds())
}

func convertFile(cfg loadConfig, i int, jobs chan<- batch, st *loadStats) error {
	path := filepath.Join(cfg.parquetDir, fmt.Sprintf("hits_%d.parquet", i))
	if _, err := os.Stat(path); err != nil {
		return err
	}
	sql := fmt.Sprintf(`SET threads = %d; SET memory_limit = '%s';
COPY (SELECT * REPLACE (
    make_timestamp(EventTime * 1000000) AS EventTime,
    make_timestamp(ClientEventTime * 1000000) AS ClientEventTime,
    make_timestamp(LocalEventTime * 1000000) AS LocalEventTime,
    DATE '1970-01-01' + EventDate::INTEGER AS EventDate)
  FROM read_parquet('%s', binary_as_string = true))
TO '/dev/stdout' (FORMAT json)`, cfg.duckThreads, cfg.duckMemory, path)
	cmd := exec.Command(cfg.duckdb, "-c", sql)
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	reader := bufio.NewReaderSize(stdout, 4<<20)
	index := 0
	buf := make([]byte, 0, cfg.batchBytes+64<<10)
	var rows int64
	flush := func() {
		if rows == 0 {
			return
		}
		st.rowsRead.Add(rows)
		jobs <- batch{file: i, index: index, rows: rows, raw: buf}
		index++
		buf = make([]byte, 0, cfg.batchBytes+64<<10)
		rows = 0
	}
	for {
		line, err := reader.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			if len(buf)+len(line) > cfg.batchBytes {
				flush()
			}
			buf = append(buf, line...)
			if line[len(line)-1] != '\n' {
				buf = append(buf, '\n')
			}
			rows++
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
	}
	flush()
	return cmd.Wait()
}

func post(client *http.Client, sys system, prefix string, b batch, st *loadStats) {
	insertID := fmt.Sprintf("%s-f%d-b%d", prefix, b.file, b.index)
	backoff := time.Second
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		req, err := sys.insertRequest(b.raw, insertID)
		if err != nil {
			log.Fatal(err)
		}
		started := time.Now()
		resp, err := client.Do(req)
		st.requests.Add(1)
		st.bytesSent.Add(int64(len(b.raw)))
		if err != nil {
			st.record(0, 0)
			st.retriesOther.Add(1)
			log.Printf("load: f%d b%d attempt %d: %v", b.file, b.index, attempt, err)
			time.Sleep(backoff)
			backoff = min(backoff*2, 8*time.Second)
			continue
		}
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		switch {
		case resp.StatusCode/100 == 2:
			inserted, err := sys.acceptedRows(raw, b.rows)
			if err != nil {
				log.Printf("load: f%d b%d: decode: %v", b.file, b.index, err)
			}
			st.record(resp.StatusCode, msSince(started))
			st.rowsAccepted.Add(inserted)
			st.rowsRejected.Add(b.rows - inserted)
			if inserted != b.rows {
				log.Printf("load: f%d b%d: %d of %d rows accepted: %s", b.file, b.index, inserted, b.rows, truncate(string(raw), 300))
			}
			return
		case resp.StatusCode == http.StatusTooManyRequests:
			st.record(resp.StatusCode, 0)
			st.retries429.Add(1)
			wait := 1.0
			if v, err := strconv.ParseFloat(resp.Header.Get("Retry-After"), 64); err == nil {
				wait = v
			}
			time.Sleep(time.Duration(min(wait, 5) * float64(time.Second)))
		default:
			st.record(resp.StatusCode, 0)
			st.retriesOther.Add(1)
			log.Printf("load: f%d b%d attempt %d: HTTP %d: %s", b.file, b.index, attempt, resp.StatusCode, truncate(string(raw), 300))
			time.Sleep(backoff)
			backoff = min(backoff*2, 8*time.Second)
		}
	}
	log.Printf("load: f%d b%d gave up after %d attempts, %d rows lost", b.file, b.index, maxAttempts, b.rows)
	st.rowsFailed.Add(b.rows)
}

func (s *loadStats) record(status int, latencyMs float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.statuses[status]++
	if latencyMs > 0 {
		s.latenciesMs = append(s.latenciesMs, latencyMs)
	}
}

func (s *loadStats) summary(elapsed float64) map[string]any {
	s.mu.Lock()
	latencies := append([]float64(nil), s.latenciesMs...)
	statuses := map[string]int64{}
	for code, n := range s.statuses {
		statuses[strconv.Itoa(code)] = n
	}
	s.mu.Unlock()
	sort.Float64s(latencies)
	accepted := s.rowsAccepted.Load()
	mib := float64(s.bytesSent.Load()) / 1048576
	return map[string]any{
		"rows_read":     s.rowsRead.Load(),
		"rows_accepted": accepted,
		"rows_rejected": s.rowsRejected.Load(),
		"rows_failed":   s.rowsFailed.Load(),
		"requests":      s.requests.Load(),
		"retries_429":   s.retries429.Load(),
		"retries_other": s.retriesOther.Load(),
		"responses":     statuses,
		"duration_s":    elapsed,
		"rows_per_s":    float64(accepted) / max(elapsed, 1e-9),
		"data_sent_mib": mib,
		"mib_per_s":     mib / max(elapsed, 1e-9),
		"latency_ms": map[string]float64{
			"p50": percentile(latencies, 50),
			"p90": percentile(latencies, 90),
			"p99": percentile(latencies, 99),
			"max": percentile(latencies, 100),
		},
	}
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	rank := p / 100 * float64(len(sorted)-1)
	lo := int(rank)
	hi := min(lo+1, len(sorted)-1)
	return sorted[lo] + (sorted[hi]-sorted[lo])*(rank-float64(lo))
}
