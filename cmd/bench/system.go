package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	settlePoll       = 5 * time.Second
	settleQuietPolls = 3
)

// system is one database under test. The load and query benches use it, so
// every system gets the same data, the same concurrency and the same timing.
type system interface {
	createTable(schemaPath string, partitions int)
	insertRequest(body []byte, insertID string) (*http.Request, error)
	acceptedRows(response []byte, sent int64) (int64, error)
	settle(limit time.Duration) (left int64, elapsed time.Duration)
	countRows() (int64, error)
	runQuery(sql string, options map[string]any, timeoutMs int) try
	version() string
}

// systemDefaults holds the flag values that depend on the system under test.
type systemDefaults struct {
	api     string
	ref     string
	schema  string
	queries string
	options string
}

var defaultsBySystem = map[string]systemDefaults{
	"smolquery": {
		api:     "http://127.0.0.1:4000",
		ref:     "bench.hits",
		schema:  "suites/clickbench/schema.json",
		queries: "suites/clickbench/queries.sql",
		options: `{"fileCache":true}`,
	},
	"clickhouse": {
		api:     "http://127.0.0.1:8123",
		ref:     "hits",
		schema:  "suites/clickbench/clickhouse/create.sql",
		queries: "suites/clickbench/clickhouse/queries.sql",
		options: `{}`,
	},
	"victorialogs": {
		api:     "http://127.0.0.1:9428",
		queries: "suites/clickbench/victorialogs/queries.logsql",
		options: `{}`,
	},
}

func defaultsFor(name string) systemDefaults {
	defaults, ok := defaultsBySystem[name]
	if !ok {
		fatalf("unknown system %q", name)
	}
	return defaults
}

func newSystem(name, base, ref string, timeout time.Duration) system {
	client := &http.Client{Timeout: timeout}
	switch name {
	case "smolquery":
		return &smolquery{client: newAPI(base, timeout), ref: ref}
	case "clickhouse":
		return &clickhouse{client: client, base: base, ref: ref}
	case "victorialogs":
		return &victorialogs{client: client, base: base}
	}
	fatalf("unknown system %q", name)
	return nil
}

type smolquery struct {
	client *api
	ref    string
}

func (s *smolquery) createTable(schemaPath string, partitions int) {
	var def tableDef
	if err := readJSON(schemaPath, &def); err != nil {
		fatalf("read %s: %v", schemaPath, err)
	}
	dataset, table, found := strings.Cut(s.ref, ".")
	if !found {
		fatalf("-ref %q is not dataset.table", s.ref)
	}
	s.client.must(http.MethodPost, "/v1/datasets", map[string]any{"id": dataset})
	s.client.must(http.MethodPost, "/v1/datasets/"+dataset+"/tables", map[string]any{"id": table, "schema": def.Schema})
	if len(def.Clustering) > 0 {
		s.client.must(http.MethodPatch, "/v1/datasets/"+dataset+"/tables/"+table, map[string]any{"clustering": def.Clustering})
	}
	if partitions > 0 {
		s.client.must(http.MethodPatch, "/v1/datasets/"+dataset+"/tables/"+table, map[string]any{"partitions": partitions})
	}
	log.Printf("load: table %s ready, clustering %v, partitions %d", s.ref, def.Clustering, partitions)
}

func (s *smolquery) insertRequest(body []byte, insertID string) (*http.Request, error) {
	dataset, table, _ := strings.Cut(s.ref, ".")
	target := fmt.Sprintf("%s/v1/datasets/%s/tables/%s/insert?insertId=%s", s.client.base, dataset, table, insertID)
	req, err := http.NewRequest(http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-ndjson")
	req.Header.Set("Authorization", s.client.auth)
	return req, nil
}

func (s *smolquery) acceptedRows(response []byte, _ int64) (int64, error) {
	var parsed struct {
		InsertedRows int64 `json:"insertedRows"`
	}
	if err := json.Unmarshal(response, &parsed); err != nil {
		return 0, err
	}
	return parsed.InsertedRows, nil
}

func (s *smolquery) settle(limit time.Duration) (int64, time.Duration) {
	started := time.Now()
	deadline := started.Add(limit)
	sql := fmt.Sprintf("SELECT count(*) AS n FROM %s WHERE CounterID = -1", s.ref)
	hot := int64(-1)
	for {
		resp, _, err := s.client.query(sql, map[string]any{"timeoutMs": 60_000})
		if err == nil && resp.Job.Statistics != nil {
			hot = resp.Job.Statistics.Hot.FilesTotal
			if hot == 0 {
				return 0, time.Since(started)
			}
		}
		if time.Now().After(deadline) {
			return hot, time.Since(started)
		}
		time.Sleep(settlePoll)
	}
}

func (s *smolquery) countRows() (int64, error) {
	resp, _, err := s.client.query("SELECT count(*) AS n FROM "+s.ref, nil)
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

func (s *smolquery) runQuery(sql string, options map[string]any, _ int) try {
	started := time.Now()
	resp, status, err := s.client.query(sql, options)
	t := try{WallMs: msSince(started), Status: status}
	if err != nil {
		t.Error = err.Error()
		return t
	}
	t.DurationMs = resp.Job.DurationMs
	t.TotalRows = resp.TotalRows
	t.FileCache = resp.Job.FileCache
	t.Scatter = resp.Job.Scatter
	return t
}

func (s *smolquery) version() string {
	return ""
}

type clickhouse struct {
	client *http.Client
	base   string
	ref    string
}

func (c *clickhouse) statement(sql string, params url.Values) (int, []byte, error) {
	req, err := http.NewRequest(http.MethodPost, c.base+"/?"+params.Encode(), strings.NewReader(sql))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "text/plain")
	return send(c.client, req)
}

func (c *clickhouse) scalar(sql string) (string, error) {
	status, raw, err := c.statement(sql, url.Values{"wait_end_of_query": {"1"}})
	if err != nil {
		return "", err
	}
	if status/100 != 2 {
		return "", fmt.Errorf("HTTP %d: %s", status, truncate(string(raw), 300))
	}
	return strings.TrimSpace(string(raw)), nil
}

func (c *clickhouse) scalarInt(sql string) (int64, error) {
	text, err := c.scalar(sql)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(text, 10, 64)
}

func (c *clickhouse) createTable(schemaPath string, _ int) {
	ddl, err := os.ReadFile(schemaPath)
	if err != nil {
		fatalf("read %s: %v", schemaPath, err)
	}
	if _, err := c.scalar(string(ddl)); err != nil {
		fatalf("create table: %v", err)
	}
	log.Printf("load: table %s ready", c.ref)
}

func (c *clickhouse) insertRequest(body []byte, _ string) (*http.Request, error) {
	params := url.Values{
		"query":             {"INSERT INTO " + c.ref + " FORMAT JSONEachRow"},
		"wait_end_of_query": {"1"},
	}
	req, err := http.NewRequest(http.MethodPost, c.base+"/?"+params.Encode(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-ndjson")
	return req, nil
}

func (c *clickhouse) acceptedRows(_ []byte, sent int64) (int64, error) {
	return sent, nil
}

func (c *clickhouse) settle(limit time.Duration) (int64, time.Duration) {
	sql := fmt.Sprintf("SELECT count() FROM system.merges WHERE table = '%s'", c.ref)
	return waitQuiet(limit, func() (int64, error) { return c.scalarInt(sql) })
}

func (c *clickhouse) countRows() (int64, error) {
	return c.scalarInt("SELECT count() FROM " + c.ref)
}

func (c *clickhouse) runQuery(sql string, _ map[string]any, timeoutMs int) try {
	params := url.Values{
		"wait_end_of_query":  {"1"},
		"max_execution_time": {strconv.Itoa(timeoutMs / 1000)},
	}
	return timedTry(func() (int, []byte, error) { return c.statement(sql, params) })
}

func (c *clickhouse) version() string {
	text, err := c.scalar("SELECT version()")
	if err != nil {
		log.Printf("version: %v", err)
	}
	return text
}

type victorialogs struct {
	client *http.Client
	base   string
}

var victorialogsVersion = regexp.MustCompile(`app_version="[^"]*?(v\d+\.\d+\.\d+)[^"]*"`)

func (v *victorialogs) logsql(query string, timeoutMs int) (int, []byte, error) {
	form := url.Values{"query": {query}}
	if timeoutMs > 0 {
		form.Set("timeout", strconv.Itoa(timeoutMs/1000)+"s")
	}
	req, err := http.NewRequest(http.MethodPost, v.base+"/select/logsql/query", strings.NewReader(form.Encode()))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return send(v.client, req)
}

func (v *victorialogs) metrics() (string, error) {
	req, err := http.NewRequest(http.MethodGet, v.base+"/metrics", nil)
	if err != nil {
		return "", err
	}
	status, raw, err := send(v.client, req)
	if err != nil {
		return "", err
	}
	if status/100 != 2 {
		return "", fmt.Errorf("HTTP %d: %s", status, truncate(string(raw), 300))
	}
	return string(raw), nil
}

func (v *victorialogs) activeMerges() (int64, error) {
	text, err := v.metrics()
	if err != nil {
		return 0, err
	}
	var active int64
	for line := range strings.SplitSeq(text, "\n") {
		if !strings.HasPrefix(line, "vl_active_merges{") {
			continue
		}
		fields := strings.Fields(line)
		n, err := strconv.ParseInt(fields[len(fields)-1], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parse %q: %w", line, err)
		}
		active += n
	}
	return active, nil
}

func (v *victorialogs) createTable(string, int) {
	log.Printf("load: victorialogs takes no schema")
}

func (v *victorialogs) insertRequest(body []byte, _ string) (*http.Request, error) {
	params := url.Values{
		"_time_field":    {"EventTime"},
		"_stream_fields": {"AdvEngineID,CounterID"},
	}
	req, err := http.NewRequest(http.MethodPost, v.base+"/insert/jsonline?"+params.Encode(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/stream+json")
	return req, nil
}

func (v *victorialogs) acceptedRows(_ []byte, sent int64) (int64, error) {
	return sent, nil
}

func (v *victorialogs) settle(limit time.Duration) (int64, time.Duration) {
	req, err := http.NewRequest(http.MethodPost, v.base+"/internal/force_flush", nil)
	if err != nil {
		fatalf("force flush: %v", err)
	}
	status, raw, err := send(v.client, req)
	if err != nil || status/100 != 2 {
		log.Printf("load: force flush: HTTP %d: %v: %s", status, err, truncate(string(raw), 300))
	}
	return waitQuiet(limit, v.activeMerges)
}

func (v *victorialogs) countRows() (int64, error) {
	status, raw, err := v.logsql("* | stats count() n", 0)
	if err != nil {
		return 0, err
	}
	if status/100 != 2 {
		return 0, fmt.Errorf("HTTP %d: %s", status, truncate(string(raw), 300))
	}
	var row struct {
		N json.Number `json:"n"`
	}
	if err := json.Unmarshal(raw, &row); err != nil {
		return 0, fmt.Errorf("decode %q: %w", truncate(string(raw), 300), err)
	}
	return row.N.Int64()
}

func (v *victorialogs) runQuery(query string, _ map[string]any, timeoutMs int) try {
	return timedTry(func() (int, []byte, error) { return v.logsql(query, timeoutMs) })
}

func (v *victorialogs) version() string {
	text, err := v.metrics()
	if err != nil {
		log.Printf("version: %v", err)
		return ""
	}
	match := victorialogsVersion.FindStringSubmatch(text)
	if match == nil {
		return ""
	}
	return match[1]
}

func send(client *http.Client, req *http.Request) (int, []byte, error) {
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	return resp.StatusCode, raw, err
}

func timedTry(do func() (int, []byte, error)) try {
	started := time.Now()
	status, raw, err := do()
	t := try{WallMs: msSince(started), Status: status}
	switch {
	case err != nil:
		t.Error = err.Error()
	case status/100 != 2:
		t.Error = fmt.Sprintf("HTTP %d: %s", status, truncate(string(raw), 300))
	}
	return t
}

// waitQuiet polls until poll returns zero on settleQuietPolls polls in a row.
// The elapsed time ends at the last busy poll, so the quiet polls do not count.
func waitQuiet(limit time.Duration, poll func() (int64, error)) (int64, time.Duration) {
	started := time.Now()
	deadline := started.Add(limit)
	lastBusy := started
	left := int64(-1)
	quiet := 0
	for {
		n, err := poll()
		switch {
		case err != nil:
			log.Printf("load: settle poll: %v", err)
			quiet = 0
			lastBusy = time.Now()
		case n == 0:
			quiet++
			if quiet == settleQuietPolls {
				return 0, lastBusy.Sub(started)
			}
		default:
			left = n
			quiet = 0
			lastBusy = time.Now()
		}
		if time.Now().After(deadline) {
			return left, time.Since(started)
		}
		time.Sleep(settlePoll)
	}
}
