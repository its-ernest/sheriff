# Usage

## Installation

```sh
go install github.com/its-ernest/sheriff/cmd/sheriff@latest
```

Or build from a checkout:

```sh
git clone https://github.com/its-ernest/sheriff.git
cd sheriff
make build     # writes bin/sheriff
```

Sheriff needs Go 1.25 or newer and has no runtime dependencies beyond the Go
toolchain.

## Checking a project

The interface is `sheriff check [flags] <packages>`. Point it at any Go package
pattern; `./...` checks every package in the module:

```sh
sheriff check ./...
sheriff check ./internal/...
sheriff check ./cmd/server
```

`./...` walks the whole module — `cmd/...`, `internal/...`, `pkg/...` — so you do
not run Sheriff per file or per directory.

## Pre-flight: race detector and go vet

`Sheriff check` reads the config (the source of truth). When `[analysis]` enables
them, it runs the test suite (with the race detector when `races` is set) and
then `go vet` first, and halts on the first failure before running its own
analyzers. `tests` gates all test execution, so `tests = false` skips both the
plain test run and the race detector:

```ini
[analysis]
tests = true
races = true
vet = true
```

Sheriff does not reimplement these tools; it wraps them and returns their exit
code.

## Flags

| Flag | Meaning |
| --- | --- |
| `-json` | print the report as JSON to stdout |
| `-html` | write an interactive HTML report to a file |
| `-o path` | output file for `-html` (default `<output>/sheriff-report.html`, where `<output>` is the config's `[report].output` or `.`) |
| `-flags` | print the analyzers' flags as JSON and exit |
| `-V=full` | print the tool version and exit |

Examples:

```sh
sheriff check -json ./...                 # machine-readable JSON
sheriff check -html ./...                 # writes ./sheriff-report.html
sheriff check -html -o report.html ./...  # custom path
```

With `-html`, Sheriff prints a `file://` URL you can click to open the report in
a browser. The report is a single self-contained file (no external assets) with a
summary, a filter box, and collapsible per-analyzer groups.

## Configuration

Sheriff reads an INI file (`sheriff.ini` by default; override with `-config`).
The `[report]` section selects the output formats and the output directory:

```ini
[report]
html = true
json = true
output = ".sheriff/reports"
```

With this, `sheriff check ./...` writes `sheriff-report.html` and
`sheriff-report.json` into `.sheriff/reports/` and still prints the text
overview to stdout. The `-json` and `-html` flags override the file (and print
only that format). The `[project]`, `[analysis]` and `[remediation]` sections
are parsed for upcoming features. A sample lives at
[`sheriff.ini`](../sheriff.ini).

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | no findings (or `-json`/`-html` output succeeded) |
| `1` | a package or analysis error occurred |
| `3` | findings were reported |

## Output

Text is the default:

```text
SHERIFF REPORT

Packages analyzed:  12
Findings detected:  7

Findings:
  [nilptr]  internal/config/config.go:42:9: nil pointer dereference: cfg may be nil
  ...

By analyzer:
  assert   2
  bounds   2
  divzero  1
  nilptr   2
```

JSON is available with `-json` for use by other tools, and HTML with `-html` for
human review. The same finding model backs all three.

## Examples

Runnable, vulnerable programs live under `_examples/`. See
[`_examples/README.md`](../_examples/README.md).