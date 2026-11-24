// Package pipeline runs a concurrent, shard-based processing pipeline.
package pipeline

import "fmt"

// Config describes the pipeline.
type Config struct {
	Addr string
}

// Load returns nil when addr is empty.
func Load(addr string) *Config {
	if addr == "" {
		return nil
	}
	return &Config{Addr: addr}
}

// Describe formats the listen address. It assumes cfg is non-nil.
func Describe(cfg *Config) string {
	return fmt.Sprintf("listening on %s", cfg.Addr)
}

// MustDescribe describes an empty address, which yields a nil config.
func MustDescribe() string {
	return Describe(Load(""))
}
