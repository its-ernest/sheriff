// Package scanner fetches targets and reads their status.
package scanner

import (
	"fmt"
)

// Config describes a scan target.
type Config struct {
	Host string
	Port int
}

// DefiniteNil dereferences a config that is never initialised.
func DefiniteNil() {
	var cfg *Config
	fmt.Println(cfg.Host) // definite nil
}

// load returns nil for an empty address.
func load(addr string) *Config {
	if addr == "" {
		return nil
	}
	return &Config{Host: addr}
}

// MaybeNil dereferences a config that may be nil.
func MaybeNil() {
	cfg := load("")
	fmt.Println(cfg.Host) // maybe nil
}

// lookup returns a config for a positive id.
func lookup(id int) *Config {
	if id > 0 {
		return &Config{Host: "localhost"}
	}
	return nil
}

// ProvenSafe dereferences a config proven non-nil by the argument.
func ProvenSafe() {
	cfg := lookup(10)
	fmt.Println(cfg.Host) // safe: id is a positive constant
}

// Unknown dereferences a config from an unresolved function value.
func Unknown() {
	var load func() *Config
	cfg := load()
	fmt.Println(cfg.Host) // unknown
}
