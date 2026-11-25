# Examples

Realistic, self-contained projects that are vulnerable to Sheriff's analyzers.
Each is its own Go module (with `go.mod`) and is meant to be analyzed as a whole
project, not file by file.

The directory is prefixed with `_` so the Go tool ignores it: `go build ./...`,
`go test ./...` and `go vet ./...` at the repository root do not descend into it.
The examples are illustrations, not part of the tool.

## Requirements

- Go 1.25 or newer.
- A built `sheriff` binary (`make build`) or `go install ./cmd/sheriff`.

## The projects

| Project | What it exercises |
| --- | --- |
| [`http-scanner`](./http-scanner) | nil pointer cases (definite, maybe, proven-safe, unknown), discarded errors, a concurrent worker pool |
| [`type-safety`](./type-safety) | impossible type assertions, out-of-range indexing, division by zero |

## Running a project

From inside a project directory:

```sh
cd _examples/http-scanner
sheriff check ./...          # analyze the whole project
sheriff build -o scanner ./...   # remediate in an isolated workspace and build
```

`sheriff build` prints the report, fixes what it can prove, lists what needs
review, and compiles the remediated copy. The original source is never modified.