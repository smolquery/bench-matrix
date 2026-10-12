package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"strings"
	"time"
)

type try struct {
	WallMs     float64         `json:"wall_ms"`
	DurationMs *float64        `json:"duration_ms"`
	TotalRows  *int64          `json:"total_rows"`
	FileCache  *string         `json:"file_cache"`
	Scatter    json.RawMessage `json:"scatter"`
	Status     int             `json:"status"`
	Error      string          `json:"error,omitempty"`
}

type queryResult struct {
	N       int     `json:"n"`
	SQL     string  `json:"sql"`
	Tries   []try   `json:"tries"`
	FirstMs float64 `json:"first_ms"`
	HotMs   float64 `json:"hot_ms"`
	Errors  int     `json:"errors"`
}

type querySummary struct {
	Queries      int     `json:"queries"`
	Passed       int     `json:"passed"`
	FirstSumS    float64 `json:"first_sum_s"`
	HotSumS      float64 `json:"hot_sum_s"`
	HotGeomeanMs float64 `json:"hot_geomean_ms"`
}

type queryRun struct {
	StartedAt  string         `json:"started_at"`
	FinishedAt string         `json:"finished_at"`
	Ref        string         `json:"ref"`
	Tries      int            `json:"tries"`
	Options    map[string]any `json:"options"`
	Summary    querySummary   `json:"summary"`
	Queries    []queryResult  `json:"queries"`
}

func runQuery(args []string) {
	fs := flag.NewFlagSet("query", flag.ExitOnError)
	base := fs.String("api", "http://127.0.0.1:4000", "smolquery API base URL")
	queries := fs.String("queries", "suites/clickbench/queries.sql", "one query per line, %{ref} for the table")
	ref := fs.String("ref", "bench.hits", "dataset.table")
	tries := fs.Int("tries", 3, "runs per query")
	timeoutMs := fs.Int("timeout-ms", 300_000, "timeoutMs sent with every job")
	options := fs.String("options", `{"fileCache":true}`, "extra JSON fields merged into every request body")
	out := fs.String("out", "out/query.json", "result JSON path")
	fs.Parse(args)

	list, err := readQueries(*queries, *ref)
	if err != nil {
		log.Fatal(err)
	}
	extra := map[string]any{}
	if err := json.Unmarshal([]byte(*options), &extra); err != nil {
		log.Fatalf("-options: %v", err)
	}
	extra["timeoutMs"] = *timeoutMs

	client := newAPI(*base, time.Duration(*timeoutMs+60_000)*time.Millisecond)
	run := queryRun{
		StartedAt: time.Now().UTC().Format(time.RFC3339),
		Ref:       *ref,
		Tries:     *tries,
		Options:   extra,
	}
	for n, sql := range list {
		r := queryResult{N: n + 1, SQL: sql}
		for range *tries {
			started := time.Now()
			resp, status, err := client.query(sql, extra)
			t := try{WallMs: msSince(started), Status: status}
			if err != nil {
				t.Error = err.Error()
				r.Errors++
			} else {
				t.DurationMs = resp.Job.DurationMs
				t.TotalRows = resp.TotalRows
				t.FileCache = resp.Job.FileCache
				t.Scatter = resp.Job.Scatter
			}
			r.Tries = append(r.Tries, t)
		}
		if r.Tries[0].Error == "" {
			r.FirstMs = r.Tries[0].WallMs
		}
		for _, t := range r.Tries[1:] {
			if t.Error == "" && (r.HotMs == 0 || t.WallMs < r.HotMs) {
				r.HotMs = t.WallMs
			}
		}
		log.Printf("Q%d: first %.0f ms, hot %.0f ms, errors %d", r.N, r.FirstMs, r.HotMs, r.Errors)
		if r.Errors > 0 {
			log.Printf("Q%d error: %s", r.N, lastError(r.Tries))
		}
		run.Queries = append(run.Queries, r)
	}
	run.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	run.Summary = summarize(run.Queries)
	writeJSON(*out, run)
	log.Printf("query: %d/%d passed, first sum %.1fs, hot sum %.1fs, hot geomean %.0f ms",
		run.Summary.Passed, run.Summary.Queries, run.Summary.FirstSumS, run.Summary.HotSumS, run.Summary.HotGeomeanMs)
}

func summarize(results []queryResult) querySummary {
	s := querySummary{Queries: len(results)}
	logSum := 0.0
	for _, r := range results {
		if r.Errors > 0 {
			continue
		}
		s.Passed++
		s.FirstSumS += r.FirstMs / 1000
		s.HotSumS += r.HotMs / 1000
		logSum += math.Log(max(r.HotMs, 1))
	}
	if s.Passed > 0 {
		s.HotGeomeanMs = math.Exp(logSum / float64(s.Passed))
	}
	return s
}

func lastError(tries []try) string {
	for i := len(tries) - 1; i >= 0; i-- {
		if tries[i].Error != "" {
			return tries[i].Error
		}
	}
	return ""
}

func readQueries(path, ref string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		out = append(out, strings.ReplaceAll(line, "%{ref}", ref))
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s holds no queries", path)
	}
	return out, scanner.Err()
}
