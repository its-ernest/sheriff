// Command scanner is a small HTTP scanner that wires together the scanner,
// parse and fetch packages.
package main

import (
	"fmt"

	"example/http-scanner/internal/fetch"
	"example/http-scanner/internal/parse"
	"example/http-scanner/internal/scanner"
)

func main() {
	// A valid target is proven safe.
	scanner.ProvenSafe()

	// A missing address yields a nil config (maybe nil) and panics.
	scanner.MaybeNil()

	// Concurrency: a worker pool fetching URLs.
	f := fetch.New([]string{"http://localhost"})
	f.Start()
	for _, r := range f.Collect() {
		fmt.Println(string(r.Body))
	}

	// Discarded errors in parsing.
	_, _ = parse.Decode([]byte(`{"Name": "arnold"}`))
	fmt.Println(string(parse.Log(parse.Payload{Name: "arnold"})))
}
