// Package fetch fetches URLs concurrently with a worker pool.
package fetch

import (
	"encoding/json"
	"sync"
)

// Result is one fetched body.
type Result struct {
	URL  string
	Body []byte
}

// Fetcher fetches a set of URLs with a worker pool.
type Fetcher struct {
	urls    []string
	results chan Result
	wg      sync.WaitGroup
}

// New creates a Fetcher.
func New(urls []string) *Fetcher {
	return &Fetcher{urls: urls, results: make(chan Result, len(urls))}
}

// Start launches a worker per URL.
func (f *Fetcher) Start() {
	for _, u := range f.urls {
		f.wg.Add(1)
		go f.worker(u)
	}
}

func (f *Fetcher) worker(u string) {
	defer f.wg.Done()
	body, _ := json.Marshal(u) // discarded error; no error return -> review
	f.results <- Result{URL: u, Body: body}
}

// Collect waits for the workers and returns the results.
func (f *Fetcher) Collect() []Result {
	f.wg.Wait()
	close(f.results)
	var out []Result
	for r := range f.results {
		out = append(out, r)
	}
	return out
}
