# Contributing to Sheriff

Sheriff is a static safety-analysis tool for Go, in early development. Small,
focused changes with clear reasoning are more valuable than large speculative
ones.

## Prerequisites

- Go 1.25 or newer.
- `make` (optional; the underlying `go` commands work directly).
- `gofmt` and `go vet`, which ship with Go.

## Getting the source

```sh
git clone https://github.com/its-ernest/sheriff.git
cd sheriff
```

## Building and testing

```sh
make build   # go build -o bin/sheriff ./cmd/sheriff
make test    # go test ./...
make vet     # go vet ./...
make clean   # remove bin/
```

The equivalent direct commands are:

```sh
go build ./cmd/sheriff
go test ./...
go vet ./...
```

Before opening a pull request, run all of them and make sure the tree is
formatted:

```sh
gofmt -w .
go mod tidy
go test ./...
go vet ./...
go build ./cmd/sheriff
```

Do not suppress warnings or ignore errors to obtain a green build.

## Project layout

- `cmd/sheriff` — executable entry point. Keep it thin.
- `internal/analyzer` — analyzer registration and coordination. Analysis
  implementations do not belong in this package.
- `internal/reporting` — reserved for future diagnostic presentation. Do not
  build a reporting framework before there is a concrete need.
- `testdata` — fixtures for analyzer tests.

## Coding conventions

- Write idiomatic, readable Go. Prefer a clear implementation over an
  unnecessarily abstract one.
- Keep functions focused and handle errors explicitly. Never discard an error
  silently.
- Do not use `panic` for ordinary error handling, and do not add broad
  `recover` calls that hide implementation defects.
- Avoid unnecessary allocations, interfaces, pointer indirection and generics.
  Use them when they solve a real problem.
- Add documentation comments to exported declarations. Comments should explain
  intent, non-obvious behaviour and design decisions, not restate the code.
- Keep dependencies minimal. The standard library and
  `golang.org/x/tools` should cover the analysis infrastructure. Justify any
  new dependency in the pull request.
- Keep diagnostics deterministic where practical.

## Adding an analyzer

The `nilptr`, `divzero`, `bounds` and `assert` analyzers are reference
implementations for package layout, registration and tests. Panic-inducing paths
are the first focus, but the project is not limited to panic detection.

When adding a rule:

1. Put the analyzer in its own package, for example
   `internal/analyzer/bounds`, rather than in the registry package.
2. Implement an `*analysis.Analyzer` using `go/analysis`, `go/ast`,
   `go/token` and `go/types`.
3. Register it in `internal/analyzer/registry.go`.
4. Test it with `analysistest` from
   `golang.org/x/tools/go/analysis/analysistest`, using minimal fixtures under
   `testdata/src/<package>`.
5. Cover positive, negative and genuinely uncertain cases. A rule must not
   report uncertainty as certainty.
6. Document the rule's behaviour and limitations, including anything it cannot
   evaluate.

Example test shape:

```go
import (
	"path/filepath"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	testdata, err := filepath.Abs("../../../testdata")
	if err != nil {
		t.Fatal(err)
	}
	analysistest.Run(t, testdata, Analyzer, "example")
}
```

## Tests and documentation

- Add tests for new behaviour and for bug fixes.
- Do not add placeholder tests that claim to validate functionality which does
  not exist.
- Update the README when a change affects installation, usage or capabilities.
- State limitations honestly. Do not describe planned behaviour as implemented.

## Pull requests

- Keep each pull request focused on one change.
- Explain the problem, the approach and any trade-offs.
- Include tests and documentation updates where relevant.
- Ensure `gofmt`, `go test ./...`, `go vet ./...` and the build pass.
- Avoid unrelated refactoring in the same pull request.
- Call out new dependencies and why the standard library or existing
  dependencies are insufficient.

By contributing, you agree that your contributions are licensed under the
Apache License 2.0.
