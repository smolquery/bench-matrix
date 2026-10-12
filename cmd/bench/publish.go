package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	readmeStart = "<!-- results:start -->"
	readmeEnd   = "<!-- results:end -->"
	historyRows = 30
)

type loadRun struct {
	Files       int     `json:"files"`
	RowsCounted int64   `json:"rows_counted"`
	HotLeft     int64   `json:"hot_files_left"`
	SealS       float64 `json:"seal_s"`
	TotalS      float64 `json:"total_s"`
	Ingest      struct {
		DurationS float64 `json:"duration_s"`
		RowsPerS  float64 `json:"rows_per_s"`
		MibPerS   float64 `json:"mib_per_s"`
	} `json:"ingest"`
}

type historyEntry struct {
	ID             string  `json:"id"`
	Date           string  `json:"date"`
	SHA            string  `json:"sha"`
	Subject        string  `json:"subject,omitempty"`
	Status         string  `json:"status"`
	Rows           int64   `json:"rows"`
	IngestS        float64 `json:"ingest_s"`
	IngestRowsPerS float64 `json:"ingest_rows_per_s"`
	SealS          float64 `json:"seal_s"`
	QueriesPassed  int     `json:"queries_passed"`
	QueriesTotal   int     `json:"queries_total"`
	FirstSumS      float64 `json:"first_sum_s"`
	HotSumS        float64 `json:"hot_sum_s"`
	HotGeomeanMs   float64 `json:"hot_geomean_ms"`
	Runner         string  `json:"runner"`
	RunURL         string  `json:"run_url,omitempty"`
}

func runPublish(args []string) {
	fs := flag.NewFlagSet("publish", flag.ExitOnError)
	runDir := fs.String("run", "out", "directory holding load.json and query.json")
	results := fs.String("results", "results", "results root")
	readme := fs.String("readme", "README.md", "README whose results section is rewritten")
	sha := fs.String("sha", "", "smolquery commit under test (required)")
	subject := fs.String("subject", "", "commit subject line")
	image := fs.String("image", "", "image reference under test")
	digest := fs.String("digest", "", "image digest under test")
	trigger := fs.String("trigger", "manual", "what started the run")
	fs.Parse(args)
	if len(*sha) < 7 {
		log.Fatal("-sha is required")
	}

	now := time.Now().UTC()
	id := now.Format("2006-01-02T150405Z") + "_" + (*sha)[:7]
	dir := filepath.Join(*results, "runs", id)

	var load *loadRun
	var query *queryRun
	if raw, err := os.ReadFile(filepath.Join(*runDir, "load.json")); err == nil {
		load = &loadRun{}
		must(json.Unmarshal(raw, load))
		must(os.MkdirAll(dir, 0o755))
		must(os.WriteFile(filepath.Join(dir, "load.json"), raw, 0o644))
	}
	if raw, err := os.ReadFile(filepath.Join(*runDir, "query.json")); err == nil {
		query = &queryRun{}
		must(json.Unmarshal(raw, query))
		must(os.MkdirAll(dir, 0o755))
		must(os.WriteFile(filepath.Join(dir, "query.json"), raw, 0o644))
	}

	runURL := ""
	if os.Getenv("GITHUB_RUN_ID") != "" {
		runURL = fmt.Sprintf("%s/%s/actions/runs/%s", os.Getenv("GITHUB_SERVER_URL"), os.Getenv("GITHUB_REPOSITORY"), os.Getenv("GITHUB_RUN_ID"))
	}
	runner := describeRunner()
	writeJSON(filepath.Join(dir, "meta.json"), map[string]any{
		"id":           id,
		"published_at": now.Format(time.RFC3339),
		"sha":          *sha,
		"subject":      *subject,
		"image":        *image,
		"digest":       *digest,
		"trigger":      *trigger,
		"runner":       runner,
		"run_url":      runURL,
	})

	entry := historyEntry{
		ID: id, Date: now.Format(time.RFC3339), SHA: *sha, Subject: *subject,
		Runner: runner, RunURL: runURL, Status: "ok",
	}
	switch {
	case load == nil:
		entry.Status = "load_failed"
	case load.RowsCounted != int64(load.Files)*1_000_000:
		entry.Status = "load_short"
	case load.HotLeft != 0:
		entry.Status = "seal_incomplete"
	case query == nil:
		entry.Status = "query_failed"
	case query.Summary.Passed < query.Summary.Queries:
		entry.Status = "query_errors"
	}
	if load != nil {
		entry.Rows = load.RowsCounted
		entry.IngestS = load.Ingest.DurationS
		entry.IngestRowsPerS = load.Ingest.RowsPerS
		entry.SealS = load.SealS
	}
	if query != nil {
		entry.QueriesPassed = query.Summary.Passed
		entry.QueriesTotal = query.Summary.Queries
		entry.FirstSumS = query.Summary.FirstSumS
		entry.HotSumS = query.Summary.HotSumS
		entry.HotGeomeanMs = query.Summary.HotGeomeanMs
	}

	historyPath := filepath.Join(*results, "history.jsonl")
	history := readHistory(historyPath)
	history = append(history, entry)
	line, err := json.Marshal(entry)
	must(err)
	f, err := os.OpenFile(historyPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	must(err)
	_, err = f.Write(append(line, '\n'))
	must(err)
	must(f.Close())

	var previous *queryRun
	if prev := previousOK(history); prev != nil {
		previous = &queryRun{}
		if err := readJSON(filepath.Join(*results, "runs", prev.ID, "query.json"), previous); err != nil {
			previous = nil
		}
	}
	section := renderResults(history, query, previous)
	must(rewriteSection(*readme, section))
	log.Printf("publish: %s (%s) → %s", id, entry.Status, dir)
}

func readHistory(path string) []historyEntry {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []historyEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var e historyEntry
		if json.Unmarshal(scanner.Bytes(), &e) == nil {
			out = append(out, e)
		}
	}
	return out
}

func previousOK(history []historyEntry) *historyEntry {
	for i := len(history) - 2; i >= 0; i-- {
		if history[i].QueriesTotal > 0 {
			return &history[i]
		}
	}
	return nil
}

func renderResults(history []historyEntry, latest, previous *queryRun) string {
	var b strings.Builder
	last := history[len(history)-1]
	fmt.Fprintf(&b, "### Latest run\n\n")
	fmt.Fprintf(&b, "%s on %s: **%s**", commitLink(last.SHA), last.Date[:10], last.Status)
	if last.RunURL != "" {
		fmt.Fprintf(&b, " ([workflow run](%s))", last.RunURL)
	}
	fmt.Fprintf(&b, "\n\n")

	fmt.Fprintf(&b, "### History\n\n")
	fmt.Fprintf(&b, "| date | commit | status | load s | rows/s | seal s | passed | first sum s | hot sum s | hot geomean ms |\n")
	fmt.Fprintf(&b, "|---|---|---|---:|---:|---:|---:|---:|---:|---:|\n")
	start := max(0, len(history)-historyRows)
	for i := len(history) - 1; i >= start; i-- {
		e := history[i]
		fmt.Fprintf(&b, "| %s | %s | %s | %.1f | %.0f | %.1f | %d/%d | %.2f | %.2f | %.0f |\n",
			e.Date[:10], commitLink(e.SHA), e.Status, e.IngestS, e.IngestRowsPerS, e.SealS,
			e.QueriesPassed, e.QueriesTotal, e.FirstSumS, e.HotSumS, e.HotGeomeanMs)
	}

	if latest != nil {
		prevHot := map[int]float64{}
		if previous != nil {
			for _, q := range previous.Queries {
				if q.Errors == 0 {
					prevHot[q.N] = q.HotMs
				}
			}
		}
		fmt.Fprintf(&b, "\n<details><summary>Per-query times, latest run</summary>\n\n")
		fmt.Fprintf(&b, "| query | first ms | hot ms | previous hot ms | change |\n")
		fmt.Fprintf(&b, "|---|---:|---:|---:|---:|\n")
		for _, q := range latest.Queries {
			if q.Errors > 0 {
				fmt.Fprintf(&b, "| Q%d | error | error | %s | |\n", q.N, formatMs(prevHot[q.N]))
				continue
			}
			change := ""
			if p := prevHot[q.N]; p > 0 {
				change = fmt.Sprintf("%+.0f%%", (q.HotMs-p)/p*100)
			}
			fmt.Fprintf(&b, "| Q%d | %.0f | %.0f | %s | %s |\n", q.N, q.FirstMs, q.HotMs, formatMs(prevHot[q.N]), change)
		}
		fmt.Fprintf(&b, "\n</details>\n")
	}
	return b.String()
}

func formatMs(v float64) string {
	if v == 0 {
		return ""
	}
	return fmt.Sprintf("%.0f", v)
}

func commitLink(sha string) string {
	return fmt.Sprintf("[`%s`](https://github.com/smolquery/smolquery/commit/%s)", sha[:7], sha)
}

func rewriteSection(path, section string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	text := string(raw)
	start := strings.Index(text, readmeStart)
	end := strings.Index(text, readmeEnd)
	if start < 0 || end < start {
		return fmt.Errorf("%s has no %s ... %s section", path, readmeStart, readmeEnd)
	}
	text = text[:start+len(readmeStart)] + "\n" + section + "\n" + text[end:]
	return os.WriteFile(path, []byte(text), 0o644)
}

func describeRunner() string {
	name := os.Getenv("RUNNER_NAME")
	if name == "" {
		name, _ = os.Hostname()
	}
	desc := fmt.Sprintf("%s (%s/%s, %d vCPU", name, runtime.GOOS, runtime.GOARCH, runtime.NumCPU())
	if mem := memTotalGiB(); mem > 0 {
		desc += fmt.Sprintf(", %.0f GiB", mem)
	}
	return desc + ")"
}

func memTotalGiB() float64 {
	raw, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	for line := range strings.SplitSeq(string(raw), "\n") {
		var kb float64
		if _, err := fmt.Sscanf(line, "MemTotal: %f kB", &kb); err == nil {
			return kb / 1048576
		}
	}
	return 0
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
