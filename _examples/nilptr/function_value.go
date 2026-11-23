//go:build ignore

// This variant shows Sheriff resolving a call made through a function value.
// The nil factory is called indirectly, so a naïve analyzer would not connect
// the call to the nil return.
//
//	go run _examples/nilptr/function_value.go
//	sheriff check _examples/nilptr/function_value.go
package main

import "fmt"

type Config struct{ Host string }

func nilFactory() *Config { return nil }

func main() {
	factory := nilFactory
	cfg := factory()
	fmt.Println(cfg.Host) // panics: cfg is nil
}
