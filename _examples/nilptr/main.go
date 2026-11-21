// Command nilptr demonstrates a nil pointer dereference that Sheriff reports.
//
// A *Config is declared but never initialised, then passed to describe, which
// reads fields through it. Running the program panics; Sheriff follows the nil
// argument into describe and reports the dereference there.
package main

import "fmt"

type Config struct {
	Host string
	Port int
}

func describe(cfg *Config) string {
	return fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
}

func main() {
	var cfg *Config
	fmt.Println(describe(cfg))
}
