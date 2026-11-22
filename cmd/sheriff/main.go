// Command sheriff performs static safety analysis of Go programs.
package main

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/tools/go/analysis/multichecker"

	"github.com/its-ernest/sheriff/internal/analyzer"
)

const usage = "usage: sheriff check <packages>"

func main() {
	if err := stripCheckSubcommand(); err != nil {
		fmt.Fprintf(os.Stderr, "sheriff: %v\n%s\n", err, usage)
		os.Exit(2)
	}
	multichecker.Main(analyzer.Analyzers()...)
}

// stripCheckSubcommand requires and strips the "check" subcommand so that only
// package patterns reach the official analysis driver. Flags owned by the
// driver may appear before the subcommand. Any other first positional argument
// is rejected with an explicit error instead of being treated as a package
// pattern, which would otherwise fail later inside the driver.
func stripCheckSubcommand() error {
	args := os.Args[1:]
	for i, arg := range args {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		if arg != "check" {
			return fmt.Errorf("unknown command %q", arg)
		}
		rest := append(append([]string{}, args[:i]...), args[i+1:]...)
		os.Args = append(os.Args[:1], rest...)
		return nil
	}
	// No positional arguments (only flags); let the driver handle them.
	return nil
}
