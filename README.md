# Sheriff

**Static safety analysis for Go.**

Sheriff identifies dangerous execution paths, verifies assumptions and catches
potential failures before runtime.

> **Status: early development.** The first analyzer, `nilptr`, reports
> dereferences of pointers that are provably nil within the enclosing function.
> Other analysis rules are not implemented yet. A clean exit does not mean a
> program is safe.

## The problem

Go programs fail at runtime for reasons that are often visible in the source:
an index that can exceed a slice's length, a pointer that may be nil, a type
assertion that may not hold, a divisor that may be zero, a channel operation
that may block or fail, or an assumption that was never enforced.

Panics are one class of these failures. Sheriff aims to identify conditions
that static analysis can establish before a program runs, and to be explicit
about what it cannot establish.

## Goals

- Identify dangerous execution paths and verify assumptions before runtime.
- Detect statically provable and potentially reachable unsafe operations.
- Prefer accurate, explainable diagnostics over excessive warnings.
- Distinguish proven unsafe operations from potentially unsafe ones and from
  operations Sheriff cannot currently evaluate.
- Stay lightweight: the Go standard library and `golang.org/x/tools` are the
  only foundations.
- Improve reasoning incrementally without overstating guarantees.

## Long-term objective

Make Go applications more predictable, resilient and consistent through
increasingly powerful static reasoning.

Panic detection is the first concrete focus, not the boundary of the project.
Future work may extend to other classes of unsafe or unreliable behaviour that
can be established statically.

Sheriff does not promise to eliminate every possible failure.

## Requirements

- Go 1.25 or newer.
- No runtime dependencies beyond the Go toolchain.

## Installation

```sh
go install github.com/its-ernest/sheriff/cmd/sheriff@latest
```

Or build from a checkout:

```sh
git clone https://github.com/its-ernest/sheriff.git
cd sheriff
make build
```

The binary is written to `bin/sheriff`.

## Usage

```sh
sheriff check ./...
sheriff check ./internal/...
sheriff check ./...
```

`check` is accepted by a small adapter in `cmd/sheriff` and removed before the
official analysis driver takes over. Package patterns and analysis flags are
owned by the driver, so flags such as `-flags` and `-V=full` behave as usual.

**Current behavior:** the `nilptr` analyzer is registered. It reports
dereferences of pointers that are provably nil within the enclosing function,
for example:

```text
main.go:7:6: nil pointer dereference: p is nil
```

The analyzer is conservative: when it cannot establish nilness, it stays
silent. Other panic categories and broader safety checks are not implemented
yet.

## Implemented analysis

### `nilptr`

Reports dereferences where a local pointer is definitely nil on every path that
reaches the expression. This includes explicit dereferences (`*p`), field
selection through a nil pointer (`p.Field`), indexing or slicing a nil pointer
to an array, and ranging over a nil pointer to an array.

The analysis is intraprocedural and flow-sensitive for declarations,
assignments, `if`/`else`, loop conditions and `switch` cases. It does not reason
across function boundaries, through interfaces, maps, channels, function values
or concurrent code. Variables whose address is taken or that are captured by a
closure are treated as unknown.

A missing diagnostic means the analyzer could not prove a nil dereference, not
that the code is safe.

## Architecture

Sheriff builds on Go's official static-analysis infrastructure rather than a
custom parser, type checker or package loader.

```text
Go source
    |
package loading (go/packages, through the analysis driver)
    |
type information (go/types)
    |
analysis driver (golang.org/x/tools/go/analysis)
    |
analyzers registered in internal/analyzer
    |
diagnostics
    |
CLI output
```

The repository is intentionally small:

```text
sheriff/
├── cmd/sheriff/              executable entry point
├── internal/analyzer/        analyzer registration and coordination
│   └── nilptr/               nil pointer dereference analyzer
├── internal/reporting/       reserved for future diagnostic presentation
├── testdata/src/nilptr/      analyzer fixtures
├── README.md
├── CONTRIBUTING.md
├── LICENSE
├── instruction.md
├── .gitignore
├── go.mod
└── Makefile
```

`internal/analyzer` is a registry, not a home for analysis implementations.
Analyzer implementations live in focused subpackages and are wired into the
registry.

### CLI design

The intended interface is `sheriff check <packages>`. The official
`multichecker` driver expects package patterns directly, so the command strips
a leading `check` argument before delegating. This keeps flag parsing, package
loading, type checking and diagnostics in the driver, and avoids a CLI
framework for a single subcommand. If the command surface grows, that decision
can be revisited.

## Planned analysis directions

These are directions, not current capabilities. `nilptr` is implemented;
the remaining items are not. Panic-inducing paths are the first focus; the
project is not limited to panics.

- Slice and array bounds.
- Unsafe type assertions.
- Division by zero.
- Invalid channel operations.
- Explicit panic detection.
- Assumption and invariant verification.
- Cross-function data-flow analysis.
- Control-flow analysis.
- Additional safety properties that can be established statically.

## Known limitations

- Only the `nilptr` analyzer is implemented.
- `nilptr` is intraprocedural and conservative. It does not track pointer
  values across function calls, interface values, maps, channels, function
  values or goroutines.
- Variables whose address is taken or that are captured by a closure are not
  tracked, even when they are never modified indirectly.
- `nilptr` does not report nil interfaces, nil function calls, nil map writes or
  channel operations; those require separate rules.
- Sheriff reasons only about what its analyzers implement. Dynamic values,
  reflection, `unsafe` and long cross-package flows may remain beyond its
  reach.
- Diagnostics are advisory, not a proof of safety. The absence of a diagnostic
  is not evidence that code cannot fail.
- There is no configuration system, editor integration or source rewriting.
- The current `check` handling is deliberately minimal and may evolve as the
  command gains more analyzers.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for build, test and analyzer
contribution instructions.

## License

Sheriff is licensed under the [Apache License 2.0](LICENSE).

Copyright 2026 Ernest, [@its-ernest](https://github.com/its-ernest).
