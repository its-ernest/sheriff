# Sheriff

### The Go Safety Build System

**Find the bugs before they find your users.**

Go code can compile successfully and still contain runtime hazards. Sheriff aims to catch those problems before they reach production.

## How it works

Sheriff loads your module with `go/packages`, runs its analyzers across every
package, and prints a structured report. For example:

```text
$ sheriff check ./...
SHERIFF REPORT

Packages analyzed:  12
Findings detected:  7

Findings:
  [nilptr]  internal/config/config.go:42:9: nil pointer dereference: cfg may be nil
  [divzero] internal/metrics/avg.go:18:12: division by zero: count is zero
  [assert]  internal/middleware/auth.go:47:9: impossible type assertion: v is int, not *Session
  [assert]  internal/middleware/mw.go:23:6: impossible type assertion: v is string, not *Logger
  [bounds]  internal/parse/headers.go:31:15: index out of range: index 2 for length 2
  [bounds]  internal/parse/parse.go:57:8: index out of range: index 4 for length 4
  [nilptr]  internal/server/server.go:88:14: nil pointer dereference: res may be nil

By analyzer:
  assert   2
  bounds   2
  divzero  1
  nilptr   2
```

Sheriff is being built to analyse application code and dependencies, identify potential failures, apply safe corrections in an isolated workspace, and validate the result before building.

*Remediation and guarded builds are planned capabilities, not yet implemented.*

## Installation

```bash
go install github.com/its-ernest/sheriff/cmd/sheriff@latest
```

## Usage

```bash
sheriff check ./...   # analyze and print a structured report
sheriff build ./...   # experimental: apply safe fixes in an isolated copy
sheriff run ./...     # check + build in one pass
```

`sheriff build` never modifies your source. It copies the project to an isolated
workspace, fixes what it can prove (currently discarded errors in functions that
return an error), and prints the workspace path. Findings it cannot fix safely are
reported for review.

## Documentation

- [Usage](docs/USAGE.md)
- [Analyzers](docs/ANALYZERS.md)
- [Architecture](docs/ARCHITECTURE.md)
- [Roadmap](docs/ROADMAP.md)
- [Limitations](docs/LIMITATIONS.md)
- [Contributing](CONTRIBUTING.md)

---

**Sheriff — Building safer Go software, one analysis at a time.**

