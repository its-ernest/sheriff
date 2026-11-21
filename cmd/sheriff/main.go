// Command sheriff performs static safety analysis of Go programs.
package main

import (
	"os"

	"golang.org/x/tools/go/analysis/multichecker"

	"github.com/its-ernest/sheriff/internal/analyzer"
)

func main() {
	stripCheckSubcommand()
	multichecker.Main(analyzer.Analyzers()...)
}

// stripCheckSubcommand adapts the intended "sheriff check <packages>"
// interface to the official analysis driver, which expects package patterns as
// its positional arguments. The driver retains ownership of flags, package
// loading, type checking and diagnostics.
func stripCheckSubcommand() {
	if len(os.Args) > 1 && os.Args[1] == "check" {
		os.Args = append(os.Args[:1], os.Args[2:]...)
	}
}
