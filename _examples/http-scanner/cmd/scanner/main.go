// Command scanner is a small HTTP scanner that wires together the scanner,
// parse and middleware packages.
package main

import (
	"fmt"

	"example/http-scanner/internal/middleware"
	"example/http-scanner/internal/parse"
	"example/http-scanner/internal/scanner"
)

func main() {
	// nilptr: no target configured, so Scan dereferences a nil response.
	var cfg *scanner.Config
	fmt.Println(scanner.Scan(cfg))

	// bounds: parsing an empty buffer reads element 0.
	fmt.Println(parse.PeekEmpty())

	// assert: a non-logger context value is asserted to *Logger.
	middleware.Run()
}
