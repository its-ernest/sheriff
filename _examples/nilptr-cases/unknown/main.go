// Command unknown illustrates a value whose nilness Sheriff cannot establish.
//
// The config comes from a function value supplied at runtime, so Sheriff cannot
// prove whether it is nil. It stays silent rather than guessing. The program may
// or may not panic depending on what the loader returns.
package main

import "fmt"

type Config struct{ Host string }

func start(cfg *Config) {
	fmt.Println("starting at", cfg.Host)
}

func main() {
	var load func() *Config
	// A real program would receive load from another package.
	load = func() *Config { return &Config{Host: "localhost"} }
	start(load())
}
