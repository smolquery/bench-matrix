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
	api          string
	dataset      string
	schema       string
	parquetDir   string
	files        int
	pipelines    int
	workers      int
	batchBytes   int
	duckdb       string
	duckThreads  int
	duckMemory   string
	sealWait     time.Duration
	out          string
	insertPrefix string
}

type tableDef struct {
	ID         string            `json:"id"`
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
	fs.StringVar(&cfg.api, "api", "http://127.0.0.1:4000", "smolquery API base URL")
	fs.StringVar(&cfg.dataset, "dataset", "bench", "dataset to create the table in")
	fs.StringVar(&cfg.schema, "schema", "suites/clickbench/schema.json", "table definition: id, clustering, schema")
	fs.StringVar(&cfg.parquetDir, "parquet", "data/hits", "directory holding hits_<n>.parquet")
	fs.IntVar(&cfg.files, "files", 10, "parquet files to load, hits_0 up to hits_<files-1>")
	fs.IntVar(&cfg.pipelines, "pipelines", 2, "parquet files converted at once")
	fs.IntVar(&cfg.workers, "workers", 8, "concurrent insert requests")
	fs.IntVar(&cfg.batchBytes, "batch-bytes", 6_000_000, "NDJSON bytes per insert request")
	fs.StringVar(&cfg.duckdb, "duckdb", "bin/duckdb", "duckdb CLI that converts parquet to NDJSON")
	fs.IntVar(&cfg.duckThreads, "duckdb-threads", 1, "threads per duckdb conversion")
	fs.StringVar(&cfg.duckMemory, "duckdb-memory", "1GB", "memory limit per duckdb conversion")
	fs.DurationVar(&cfg.sealWait, "seal-wait", 20*time.Minute, "longest wait for the hot tier to seal")
	fs.StringVar(&cfg.out, "out", "out/load.json", "result JSON path")
	fs.StringVar(&cfg.insertPrefix, "insert-prefix", fmt.Sprintf("bench-%d", time.Now().Unix()), "insertId prefix")
	fs.Parse(args)

	var def tableDef
	if err := readJSON(cfg.schema, &def); err != nil {
		log.Fatalf("read %s: %v", cfg.schema, err)
	}
	client := newAPI(cfg.api, 2*time.Minute)
	ref := cfg.dataset + "." + def.ID
	createTable(client, cfg.dataset, def)

	log.Printf("load: %d file(s) into %s, %d pipeline(s), %d worker(s), %d bytes per request",
		cfg.files, ref, cfg.pipelines, cfg.workers, cfg.batchBytes)
	ingest := ingestFiles(cfg, def.ID)
	log.Printf("load: %d rows accepted in %.1fs (%.0f rows/s), %d rejected, %d failed",
		ingest["rows_accepted"], ingest["duration_s"], ingest["rows_per_s"], ingest["rows_rejected"], ingest["rows_failed"])

	sealStarted := time.Now()
	hotLeft := waitForSeal(client, ref, cfg.sealWait)
	sealS := time.Since(sealStarted).Seconds()
	log.Printf("load: hot tier %d file(s) left after %.1fs", hotLeft, sealS)

	count, err := countRows(client, ref)
	if err != nil {
		log.Printf("load: count(*) failed: %v", err)
	}
	expected := int64(cfg.files) * 1_000_000
	log.Printf("load: count(*) = %d, expected %d", count, expected)

	result := map[string]any{
		"table":          ref,
		"files":          cfg.files,
		"rows_expected":  expected,
		"rows_counted":   count,
		"ingest":         ingest,
		"seal_s":         sealS,
		"hot_files_left": hotLeft,
		"total_s":        ingest["duration_s"].(float64) + sealS,
		"config": map[string]any{
			"pipelines":      cfg.pipelines,
			"workers":        cfg.workers,
			"batch_bytes":    cfg.batchBytes,
			"duckdb_threads": cfg.duckThreads,
		},
	}
	writeJSON(cfg.out, result)

	if count != expected {
		fatalf("load: counted %d rows, expected %d", count, expected)
	}
	if hotLeft != 0 {
		fatalf("load: the hot tier did not seal within %s (%d file(s) left)", cfg.sealWait, hotLeft)
	}
}

func createTable(client *api, dataset string, def tableDef) {
	client.must(http.MethodPost, "/v1/datasets", map[string]any{"id": dataset})
	client.must(http.MethodPost, "/v1/datasets/"+dataset+"/tables", map[string]any{"id": def.ID, "schema": def.Schema})
	if len(def.Clustering) > 0 {
		client.must(http.MethodPatch, "/v1/datasets/"+dataset+"/tables/"+def.ID, map[string]any{"clustering": def.Clustering})
	}
	log.Printf("load: table %s.%s ready, clustering %v", dataset, def.ID, def.Clustering)
}

func ingestFiles(cfg loadConfig, table string) map[string]any {
	url := fmt.Sprintf("%s/v1/datasets/%s/tables/%s/insert", cfg.api, cfg.dataset, table)
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
	auth := "Bearer " + os.Getenv("SMOLQUERY_API_KEY")

	started := time.Now()
	var workers sync.WaitGroup
	for range cfg.workers {
		workers.Go(func() {
			for b := range jobs {
				post(httpClient, url, auth, cfg.insertPrefix, b, st)
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

func post(client *http.Client, url, auth, prefix string, b batch, st *loadStats) {
	target := fmt.Sprintf("%s?insertId=%s-f%d-b%d", url, prefix, b.file, b.index)
	backoff := time.Second
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		req, err := http.NewRequest(http.MethodPost, target, bytes.NewReader(b.raw))
		if err != nil {
			log.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/x-ndjson")
		req.Header.Set("Authorization", auth)
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
		switch resp.StatusCode {
		case http.StatusOK:
			var parsed struct {
				InsertedRows int64 `json:"insertedRows"`
			}
			if err := json.Unmarshal(raw, &parsed); err != nil {
				log.Printf("load: f%d b%d: decode: %v", b.file, b.index, err)
			}
			st.record(resp.StatusCode, msSince(started))
			st.rowsAccepted.Add(parsed.InsertedRows)
			st.rowsRejected.Add(b.rows - parsed.InsertedRows)
			if parsed.InsertedRows != b.rows {
				log.Printf("load: f%d b%d: %d of %d rows accepted: %s", b.file, b.index, parsed.InsertedRows, b.rows, truncate(string(raw), 300))
			}
			return
		case http.StatusTooManyRequests:
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

func waitForSeal(client *api, ref string, limit time.Duration) int64 {
	deadline := time.Now().Add(limit)
	sql := fmt.Sprintf("SELECT count(*) AS n FROM %s WHERE CounterID = -1", ref)
	hot := int64(-1)
	for {
		resp, _, err := client.query(sql, map[string]any{"timeoutMs": 60_000})
		if err == nil && resp.Job.Statistics != nil {
			hot = resp.Job.Statistics.Hot.FilesTotal
			if hot == 0 {
				return 0
			}
		}
		if time.Now().After(deadline) {
			return hot
		}
		time.Sleep(5 * time.Second)
	}
}

func countRows(client *api, ref string) (int64, error) {
	resp, _, err := client.query("SELECT count(*) AS n FROM "+ref, nil)
	if err != nil {
		return 0, err
	}
	if len(resp.Rows) == 0 {
		return 0, errors.New("no rows")
	}
	var asObject map[string]json.Number
	if err := json.Unmarshal(resp.Rows[0], &asObject); err == nil {
		return asObject["n"].Int64()
	}
	var asArray []json.Number
	if err := json.Unmarshal(resp.Rows[0], &asArray); err == nil && len(asArray) > 0 {
		return asArray[0].Int64()
	}
	return 0, fmt.Errorf("unexpected row %s", resp.Rows[0])
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
