// Package scanner fetches a target and reads its status.
package scanner

import "net/http"

// Config describes a scan target.
type Config struct {
	Target string
	Status int
}

// buildResponse returns nil when cfg is nil.
func buildResponse(cfg *Config) *http.Response {
	if cfg == nil {
		return nil
	}
	return &http.Response{StatusCode: cfg.Status}
}

// Result is the outcome of scanning one target.
type Result struct {
	URL    string
	Status int
}

// Scan reads the status for cfg. It assumes cfg is valid.
func Scan(cfg *Config) Result {
	resp := buildResponse(cfg)
	return Result{URL: cfg.Target, Status: resp.StatusCode}
}
