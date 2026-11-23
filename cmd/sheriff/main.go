// Command sheriff performs static safety analysis of Go programs.
//
// It runs Sheriff's analyzers over the named packages and prints a structured
// report (text by default, JSON with -json). Package loading, type checking and
// the analysis driver come from golang.org/x/tools; Sheriff only contributes the
// analyzers and the report formatting.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/checker"
	"golang.org/x/tools/go/packages"

	"github.com/its-ernest/sheriff/internal/analyzer"
	"github.com/its-ernest/sheriff/internal/reporting"
)

// version is Sheriff's own version, distinct from the analyzer set.
const version = "devel"

const usage = `usage: sheriff check [flags] <packages>

Run Sheriff's safety analyzers over the named Go packages and print a
structured report.

Flags:
  -json     emit the report as JSON
  -html     write an interactive HTML report to a file
  -o path   output file for -html (default "sheriff-report.html")
`

func main() {
	os.Exit(run(os.Args))
}

func run(argv []string) int {
	prog := filepath.Base(argv[0])
	args := argv[1:]

	// The go command probes a tool's identity with -V=full.
	for _, a := range args {
		if a == "-V=full" {
			fmt.Printf("%s version %s\n", prog, version)
			return 0
		}
	}

	if len(args) > 0 && args[0] == "help" {
		fmt.Print(usage)
		return 0
	}

	args, err := stripCheck(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n\n%s", prog, err, usage)
		return 2
	}

	fs := flag.NewFlagSet(prog, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	jsonOut := fs.Bool("json", false, "emit the report as JSON")
	htmlOut := fs.Bool("html", false, "write an HTML report to a file")
	outPath := fs.String("o", "sheriff-report.html", "output file for -html")
	printFlags := fs.Bool("flags", false, "print analyzer flags as JSON and exit")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	analyzers := analyzer.Analyzers()

	if *printFlags {
		return printAnalyzerFlags(analyzers)
	}

	patterns := fs.Args()
	if len(patterns) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}

	cfg := &packages.Config{Mode: packages.LoadAllSyntax | packages.NeedModule}
	initial, err := packages.Load(cfg, patterns...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", prog, err)
		return 1
	}
	if len(initial) == 0 {
		fmt.Fprintf(os.Stderr, "%s: %s matched no packages\n", prog, strings.Join(patterns, " "))
		return 1
	}
	pkgErrors := packages.PrintErrors(initial)

	graph, err := checker.Analyze(analyzers, initial, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", prog, err)
		return 1
	}

	findings, analysisErrors := collectFindings(graph)
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Position != findings[j].Position {
			return findings[i].Position < findings[j].Position
		}
		if findings[i].Analyzer != findings[j].Analyzer {
			return findings[i].Analyzer < findings[j].Analyzer
		}
		return findings[i].Message < findings[j].Message
	})

	report := reporting.Report{Packages: len(initial), Findings: findings}
	switch {
	case *htmlOut:
		if err := writeHTML(*outPath, report); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", prog, err)
			return 1
		}
		abs, err := filepath.Abs(*outPath)
		if err != nil {
			abs = *outPath
		}
		fmt.Printf("%d finding(s) across %d package(s). Report: file://%s\n", len(findings), len(initial), abs)
	case *jsonOut:
		if err := report.WriteJSON(os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", prog, err)
			return 1
		}
	default:
		report.WriteText(os.Stdout)
	}

	if pkgErrors > 0 || analysisErrors > 0 {
		return 1
	}
	if *jsonOut || *htmlOut {
		return 0
	}
	if len(findings) > 0 {
		return 3
	}
	return 0
}

func writeHTML(path string, report reporting.Report) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := report.WriteHTML(f); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// collectFindings gathers diagnostics from the root actions (one per analyzer
// and initial package) and counts actions that failed.
func collectFindings(graph *checker.Graph) ([]reporting.Finding, int) {
	var findings []reporting.Finding
	analysisErrors := 0
	for _, act := range graph.Roots {
		if act.Err != nil {
			analysisErrors++
			continue
		}
		fset := act.Package.Fset
		for _, d := range act.Diagnostics {
			findings = append(findings, reporting.Finding{
				Analyzer: act.Analyzer.Name,
				Position: fset.Position(d.Pos).String(),
				Message:  d.Message,
			})
		}
	}
	return findings, analysisErrors
}

// printAnalyzerFlags writes the analyzers' flags as a JSON array, matching the
// contract used by the go command.
func printAnalyzerFlags(analyzers []*analysis.Analyzer) int {
	type flagInfo struct {
		Name  string
		Bool  bool
		Usage string
	}
	var out []flagInfo
	for _, a := range analyzers {
		a.Flags.VisitAll(func(f *flag.Flag) {
			boolFlag := false
			if bf, ok := f.Value.(interface{ IsBoolFlag() bool }); ok {
				boolFlag = bf.IsBoolFlag()
			}
			out = append(out, flagInfo{Name: f.Name, Bool: boolFlag, Usage: f.Usage})
		})
	}
	if out == nil {
		out = []flagInfo{}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "\t")
	if err := enc.Encode(out); err != nil {
		return 1
	}
	return 0
}

// stripCheck requires and removes the "check" subcommand so that only package
// patterns (and flags) reach the driver. A repeated program name or a missing
// subcommand is rejected with an explicit error. Flags that take a separate
// value are skipped while scanning for the subcommand.
func stripCheck(args []string) ([]string, error) {
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "-o":
			i++ // the output path is a separate value
		case strings.HasPrefix(args[i], "-"):
			// other flags take no value
		case args[i] == "check":
			rest := append(append([]string{}, args[:i]...), args[i+1:]...)
			return rest, nil
		default:
			return nil, fmt.Errorf("unknown command %q", args[i])
		}
	}
	return args, nil
}
