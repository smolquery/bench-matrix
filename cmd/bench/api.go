package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

type api struct {
	base   string
	auth   string
	client *http.Client
}

func newAPI(base string, timeout time.Duration) *api {
	return &api{
		base:   base,
		auth:   "Bearer " + os.Getenv("SMOLQUERY_API_KEY"),
		client: &http.Client{Timeout: timeout},
	}
}

func (a *api) do(method, path string, body any) (int, []byte, error) {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequest(method, a.base+path, reader)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", a.auth)
	resp, err := a.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	return resp.StatusCode, raw, err
}

func (a *api) must(method, path string, body any, ok ...int) []byte {
	status, raw, err := a.do(method, path, body)
	if err != nil {
		fatalf("%s %s: %v", method, path, err)
	}
	for _, code := range ok {
		if status == code {
			return raw
		}
	}
	if status/100 != 2 {
		fatalf("%s %s: HTTP %d: %s", method, path, status, truncate(string(raw), 500))
	}
	return raw
}

type queryResponse struct {
	TotalRows *int64            `json:"totalRows"`
	Rows      []json.RawMessage `json:"rows"`
	Job       struct {
		DurationMs *float64        `json:"durationMs"`
		FileCache  *string         `json:"fileCache"`
		Scatter    json.RawMessage `json:"scatter"`
		Statistics *struct {
			Hot struct {
				FilesTotal int64 `json:"filesTotal"`
			} `json:"hot"`
		} `json:"statistics"`
	} `json:"job"`
}

func (a *api) query(sql string, options map[string]any) (queryResponse, int, error) {
	body := map[string]any{"query": sql, "timeoutMs": 600_000, "maxResults": 1}
	for k, v := range options {
		body[k] = v
	}
	var parsed queryResponse
	status, raw, err := a.do(http.MethodPost, "/v1/queries", body)
	if err != nil {
		return parsed, status, err
	}
	if status/100 != 2 {
		return parsed, status, fmt.Errorf("HTTP %d: %s", status, truncate(string(raw), 300))
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return parsed, status, fmt.Errorf("decode: %w", err)
	}
	return parsed, status, nil
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func msSince(t time.Time) float64 {
	return float64(time.Since(t).Microseconds()) / 1000
}
