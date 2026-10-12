// Command bench runs the bench-matrix benches against one smolquery node.
//
//	bench load     create the table, insert the suite's rows, wait for seal
//	bench query    run the suite's queries and time each one
//	bench publish  file one run under results/ and refresh the README
//
// Every subcommand takes -h for its flags. The API key comes from the
// SMOLQUERY_API_KEY environment variable.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

func main() {
	log.SetFlags(log.Ltime)
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: bench load|query|publish [flags]")
		os.Exit(2)
	}
	args := os.Args[2:]
	switch os.Args[1] {
	case "load":
		runLoad(args)
	case "query":
		runQuery(args)
	case "publish":
		runPublish(args)
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n", os.Args[1])
		os.Exit(2)
	}
}

func writeJSON(path string, value any) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Fatal(err)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		log.Fatal(err)
	}
}

func readJSON(path string, into any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, into)
}
