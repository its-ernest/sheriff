// Command function-value shows Sheriff resolving a call made through a function
// value. The nil factory is called indirectly, so a naïve analyzer would not
// connect the call to the nil return.
//
//	go run ./_examples/nilptr/function-value
//	sheriff check ./_examples/nilptr/function-value
package main

import "fmt"

type Config struct{ Host string }

func nilFactory() *Config { return nil }

func main() {
	factory := nilFactory
	cfg := factory()
	fmt.Println(cfg.Host) // panics: cfg is nil
}
