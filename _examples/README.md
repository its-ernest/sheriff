# Examples

Small, self-contained programs that are vulnerable to the analyzers Sheriff
ships. Each one is valid Go that compiles and runs, but panics at runtime; the
matching analyzer reports the defect before it runs.

The directory is prefixed with `_` so the Go tool ignores it: `go build ./...`,
`go test ./...` and `go vet ./...` at the repository root do not descend into
it. The examples are illustrations, not part of the tool.

## Requirements

- Go 1.25 or newer.
- A built `sheriff` binary (`make build`) or use `go run ./cmd/sheriff`.

## Running an example

From the repository root, run the program to see it fail:

```sh
go run ./_examples/nilptr
```

Then run Sheriff against it to see the diagnostic:

```sh
go run ./cmd/sheriff check ./_examples/nilptr
```

Replace `nilptr` with any example directory below.

## The examples

| Example | Analyzer | Vulnerability | Sheriff reports |
| --- | --- | --- | --- |
| [`nilptr`](./nilptr) | `nilptr` | a nil `*Config` is dereferenced in `describe` | `nil pointer dereference: cfg is nil` |
| [`divzero`](./divzero) | `divzero` | `average` divides by a zero count | `division by zero: count is zero` |
| [`bounds`](./bounds) | `bounds` | `first` indexes element 0 of an empty slice | `index out of range: index 0 for length 0` |
| [`assert`](./assert) | `assert` | `consume` asserts an `io.Reader` to `*bytes.Buffer` | `impossible type assertion: r is *strings.Reader, not *bytes.Buffer` |

Each example passes the risky value as an argument to a helper, so the
diagnostic is reported inside the callee. This exercises the analyzers'
interprocedural parameter tracking: the argument's known value is followed into
the function that uses it.

## Full example projects

Two realistic, multi-package projects. Each is its own Go module (with a
`go.mod`) and is meant to be analyzed as a whole project, not file by file.

| Project | Packages | Defects |
| --- | --- | --- |
| [`http-scanner`](./http-scanner) | `cmd/scanner`, `internal/{scanner,parse,middleware}` | nilptr, bounds, assert |
| [`concurrent-pipeline`](./concurrent-pipeline) | `cmd/pipeline`, `internal/pipeline` | nilptr, bounds, assert (channels/goroutines) |
| [`discarded-errors`](./discarded-errors) | `cmd/app`, `internal/store` | discarded errors (exercises `sheriff build` remediation) |

Run and analyze a project from inside its directory:

```sh
cd _examples/http-scanner
go run ./cmd/scanner     # panics
sheriff check ./...      # reports defects across every package
```

`./...` scans every package in the module (`cmd/...`, `internal/...`, `pkg/...`),
so there is no need to run Sheriff per file.