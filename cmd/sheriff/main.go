// Command sheriff performs static safety analysis of Go programs.
//
// It runs Sheriff's analyzers over the named packages and prints a structured
// report (text by default, JSON with -json). Package loading, type checking and
// the analysis driver come from golang.org/x/tools; Sheriff only contributes the
// analyzers and the report formatting.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/checker"
	"golang.org/x/tools/go/packages"

	"github.com/its-ernest/sheriff/internal/analyzer"
	"github.com/its-ernest/sheriff/internal/config"
	"github.com/its-ernest/sheriff/internal/remediate"
	"github.com/its-ernest/sheriff/internal/reporting"
)

// version is Sheriff's own version, distinct from the analyzer set.
const version = "devel"

const usage = `usage: sheriff check|build|run [flags] <packages>

check runs Sheriff's safety analyzers and prints a structured report. build
prints the report, applies conservative fixes to an isolated workspace copy, and
compiles the result (the original source is never modified). run is an alias for
build. The [analysis] section of the config file can enable a test/race pre-flight
and go vet that halts on failure. The tests flag gates all test execution.

Flags:
  -json     emit the report as JSON
  -html     write an interactive HTML report to a file
  -o path   output file for -html (default: <output>/sheriff-report.html)
  -config   config file (default "sheriff.ini")
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

	sub, rest, err := splitSub(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n\n%s", prog, err, usage)
		return 2
	}
	switch sub {
	case "build":
		return runBuild(prog, rest)
	case "run":
		return runAll(prog, rest)
	}
	return runCheck(prog, rest)
}

// runAll runs the full pipeline. build already includes the check phase, so run
// is an alias for build.
func runAll(prog string, args []string) int {
	return runBuild(prog, args)
}

// preFlight runs the config-driven gates (test/race, then go vet). It returns a
// non-zero exit code if a gate fails, so the caller can halt.
func preFlight(prog string, fileCfg *config.Config, patterns []string) int {
	if fileCfg == nil {
		return 0
	}
	if fileCfg.Analysis.Tests {
		testArgs := []string{"test"}
		if fileCfg.Analysis.Races {
			testArgs = append(testArgs, "-race")
		}
		testArgs = append(testArgs, patterns...)
		if code := runGo(prog, testArgs...); code != 0 {
			return code
		}
	}
	if fileCfg.Analysis.Vet {
		if code := runGo(prog, append([]string{"vet"}, patterns...)...); code != 0 {
			return code
		}
	}
	return 0
}

// runCheck runs Sheriff's own analyzers over the named packages.
func runCheck(prog string, args []string) int {
	fs := flag.NewFlagSet(prog, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	jsonOut := fs.Bool("json", false, "emit the report as JSON")
	htmlOut := fs.Bool("html", false, "write an HTML report to a file")
	outPath := fs.String("o", "", "output file for -html (default: <output>/sheriff-report.html)")
	configPath := fs.String("config", "sheriff.ini", "config file (INI)")
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

	var fileCfg *config.Config
	if c, err := config.Load(*configPath); err == nil {
		fileCfg = &c
	} else if !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "%s: %v\n", prog, err)
		return 2
	}

	patterns := fs.Args()
	if len(patterns) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}

	// Pre-flight gates, driven by the config (the source of truth).
	if code := preFlight(prog, fileCfg, patterns); code != 0 {
		return code
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

	targets := modulePackages(initial)
	graph, err := checker.Analyze(analyzers, targets, nil)
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

	report := reporting.Report{Packages: len(targets), Findings: findings}
	switch {
	case *htmlOut:
		path := *outPath
		if path == "" {
			path = filepath.Join(outputDir(fileCfg), "sheriff-report.html")
		}
		if dir := filepath.Dir(path); dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", prog, err)
				return 1
			}
		}
		if err := writeHTML(path, report); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", prog, err)
			return 1
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			abs = path
		}
		fmt.Printf("%d finding(s) across %d package(s). Report: file://%s\n", len(findings), len(initial), abs)
	case *jsonOut:
		if err := report.WriteJSON(os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", prog, err)
			return 1
		}
	default:
		if fileCfg != nil && (fileCfg.Report.HTML || fileCfg.Report.JSON) {
			if err := writeConfigReports(fileCfg, report); err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", prog, err)
				return 1
			}
		}
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

// outputDir returns the configured report directory, or "." when unset.
func outputDir(cfg *config.Config) string {
	if cfg != nil && cfg.Report.Output != "" {
		return cfg.Report.Output
	}
	return "."
}

// writeConfigReports writes the report files selected by the [report] section.
func writeConfigReports(cfg *config.Config, report reporting.Report) error {
	dir := outputDir(cfg)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if cfg.Report.JSON {
		path := filepath.Join(dir, "sheriff-report.json")
		if err := writeJSON(path, report); err != nil {
			return err
		}
		fmt.Printf("JSON report: %s\n", path)
	}
	if cfg.Report.HTML {
		path := filepath.Join(dir, "sheriff-report.html")
		if err := writeHTML(path, report); err != nil {
			return err
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			abs = path
		}
		fmt.Printf("HTML report: file://%s\n", abs)
	}
	return nil
}

func writeJSON(path string, report reporting.Report) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := report.WriteJSON(f); err != nil {
		f.Close()
		return err
	}
	return f.Close()
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

// modulePackages returns the packages in the main module reachable from the
// initial packages, including transitive same-module imports. This makes a
// target such as ./cmd/app also analyze the internal packages it depends on.
func modulePackages(initial []*packages.Package) []*packages.Package {
	if len(initial) == 0 {
		return nil
	}
	mainMod := initial[0].Module
	if mainMod == nil {
		return initial
	}
	seen := make(map[*packages.Package]bool)
	var out []*packages.Package
	var visit func(p *packages.Package)
	visit = func(p *packages.Package) {
		if p == nil || seen[p] {
			return
		}
		seen[p] = true
		if p.Module != nil && p.Module.Path == mainMod.Path {
			out = append(out, p)
		}
		for _, imp := range p.Imports {
			visit(imp)
		}
	}
	for _, p := range initial {
		visit(p)
	}
	if len(out) == 0 {
		return initial
	}
	return out
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

// splitSub finds and removes the "check" subcommand so that only its arguments
// remain. Flags that take a separate value are skipped while scanning. A
// non-flag first positional that is not a known subcommand is rejected with an
// explicit error.
func splitSub(args []string) (string, []string, error) {
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "-o":
			i++ // the output path is a separate value
		case args[i] == "-config":
			i++ // the config path is a separate value
		case strings.HasPrefix(args[i], "-"):
			// other flags take no value
		case args[i] == "check" || args[i] == "build" || args[i] == "run":
			rest := append(append([]string{}, args[:i]...), args[i+1:]...)
			return args[i], rest, nil
		default:
			return "", nil, fmt.Errorf("unknown command %q", args[i])
		}
	}
	return "check", args, nil
}

// runGo runs an external go subcommand (for example "vet" or "test -race") and
// returns its exit code. Sheriff does not reimplement these tools; it wraps
// them.
func runGo(prog string, args ...string) int {
	cmd := exec.Command("go", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "%s: %v\n", prog, err)
		return 1
	}
	return 0
}

// runBuild scans the packages, then applies conservative fixes to a copy of the
// project in an isolated workspace. The original source is never modified.
func runBuild(prog string, args []string) int {
	fs := flag.NewFlagSet(prog, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	configPath := fs.String("config", "sheriff.ini", "config file (INI)")
	oFlag := fs.String("o", "", "output binary name (overrides [build] output)")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	patterns := fs.Args()
	if len(patterns) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}
	var fileCfg *config.Config
	if c, err := config.Load(*configPath); err == nil {
		fileCfg = &c
	} else if !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "%s: %v\n", prog, err)
		return 2
	}

	if code := preFlight(prog, fileCfg, patterns); code != 0 {
		return code
	}

	analyzers := analyzer.Analyzers()
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
	packages.PrintErrors(initial)

	targets := modulePackages(initial)
	graph, err := checker.Analyze(analyzers, targets, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", prog, err)
		return 1
	}
	findings, _ := collectFindings(graph)
	sort.Slice(findings, func(i, j int) bool { return findings[i].Position < findings[j].Position })

	// Check phase: print the structured report (and any configured report files)
	// before remediating.
	report := reporting.Report{Packages: len(targets), Findings: findings}
	report.WriteText(os.Stdout)
	if fileCfg != nil && (fileCfg.Report.HTML || fileCfg.Report.JSON) {
		if err := writeConfigReports(fileCfg, report); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", prog, err)
		}
	}

	byFile := map[string][]remediate.Finding{}
	for _, f := range findings {
		if f.Analyzer != "discarded" {
			continue
		}
		file := fileOf(f.Position)
		if file == "" {
			continue
		}
		byFile[file] = append(byFile[file], remediate.Finding{Analyzer: f.Analyzer, Position: f.Position, Message: f.Message})
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", prog, err)
		return 1
	}
	ws, err := os.MkdirTemp("", "sheriff-build-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", prog, err)
		return 1
	}
	if err := copyTree(cwd, ws); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", prog, err)
		return 1
	}

	applied, reviewCount := 0, 0
	var reviewMsgs []string
	for file, list := range byFile {
		src, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		out, res := remediate.FixFile(file, src, list)
		applied += res.Applied
		reviewCount += len(res.Review)
		reviewMsgs = append(reviewMsgs, res.Review...)
		if res.Applied > 0 {
			rel, err := filepath.Rel(cwd, file)
			if err != nil {
				rel = filepath.Base(file)
			}
			dst := filepath.Join(ws, rel)
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				continue
			}
			if err := os.WriteFile(dst, out, 0o644); err != nil {
				continue
			}
		}
	}
	fmt.Printf("Remediation: %d applied, %d need review\n", applied, reviewCount)
	for _, msg := range reviewMsgs {
		fmt.Printf("Review: %s\n", msg)
	}
	fmt.Printf("Workspace: %s\n", ws)

	// Strict mode: the build is "safe or fail". If anything remains unresolved
	// after remediation, fail without producing a binary.
	mode := "strict"
	if fileCfg != nil && fileCfg.Build.Mode != "" {
		mode = fileCfg.Build.Mode
	}
	unresolved := len(findings) - applied
	if mode == "strict" && unresolved > 0 {
		fmt.Printf("Build failed: %d unresolved finding(s) in strict mode\n", unresolved)
		return 1
	}

	output := *oFlag
	if output == "" && fileCfg != nil {
		output = fileCfg.Build.Output
		if output == "" {
			output = fileCfg.Project.Name
		}
	}
	if output != "" {
		if err := buildWorkspace(ws, cwd, output, patterns); err != nil {
			fmt.Fprintf(os.Stderr, "%s: build failed: %v\n", prog, err)
			return 1
		}
	}
	return 0
}

// buildWorkspace compiles the remediated copy in ws into the output binary.
func buildWorkspace(ws, cwd, output string, patterns []string) error {
	mains, err := mainPackages(ws, patterns)
	if err != nil {
		return err
	}
	if len(mains) == 0 {
		fmt.Println("Nothing to build (no main package).")
		return nil
	}
	dst := output
	if !filepath.IsAbs(dst) {
		dst = filepath.Join(cwd, output)
	}
	args := []string{"build", "-o", dst}
	if len(mains) > 1 {
		if err := os.MkdirAll(dst, 0o755); err != nil {
			return err
		}
	}
	args = append(args, mains...)
	cmd := exec.Command("go", args...)
	cmd.Dir = ws
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	fmt.Printf("Built: %s\n", dst)
	return nil
}

// mainPackages returns the import paths of the main packages matched by the
// patterns, evaluated in the workspace.
func mainPackages(ws string, patterns []string) ([]string, error) {
	args := append([]string{"list", "-f", "{{.Name}}\t{{.ImportPath}}"}, patterns...)
	cmd := exec.Command("go", args...)
	cmd.Dir = ws
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var mains []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) == 2 && parts[0] == "main" {
			mains = append(mains, parts[1])
		}
	}
	return mains, nil
}

// fileOf returns the file path from a "file:line:col" position.
func fileOf(pos string) string {
	i := strings.LastIndexByte(pos, ':')
	if i < 0 {
		return ""
	}
	j := strings.LastIndexByte(pos[:i], ':')
	if j < 0 {
		return pos[:i]
	}
	return pos[:j]
}

// copyTree copies the regular files under src into dst, skipping VCS and build
// output directories.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "bin", ".sheriff":
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), data, 0o644)
	})
}
