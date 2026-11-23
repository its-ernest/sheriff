//go:build ignore

// This variant shows Sheriff resolving a method call. The method returns nil,
// and the caller dereferences the result.
//
//	go run _examples/nilptr/method.go
//	sheriff check _examples/nilptr/method.go
package main

import "fmt"

type Config struct{ Host string }

type Provider struct{}

func (p *Provider) GetConfig() *Config { return nil }

func main() {
	var provider Provider
	cfg := provider.GetConfig()
	fmt.Println(cfg.Host) // panics: cfg is nil
}
