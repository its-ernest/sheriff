// Command definite-nil illustrates a provably nil pointer dereference.
//
// cfg is declared but never assigned, so it is definitely nil. Sheriff reports
// the dereference as a definite finding. Running the program panics.
package main

import "fmt"

type Config struct {
	Host string
	Port int
}

func main() {
	var cfg *Config
	fmt.Println(cfg.Host, cfg.Port)
}
