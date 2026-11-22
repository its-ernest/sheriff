# Sheriff

**Static safety analysis for Go.**

Sheriff identifies dangerous execution paths, verifies assumptions and catches
potential failures before runtime.

> **Status: early development.** Four analyzers are implemented: `nilptr`
> reports dereferences of pointers that are provably nil or possibly nil,
> `divzero` reports integer division or modulo by a divisor that is provably
> zero, `bounds` reports indexing or slicing that is provably out of range, and
> `assert` reports type assertions that provably cannot succeed. Other analysis
> rules are not implemented yet. A clean exit does not mean a program is safe.

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

**Current behavior:** the `nilptr`, `divzero`, `bounds` and `assert` analyzers
are registered. They report dereferences of pointers that are provably nil,
integer division or modulo by a divisor that is provably zero, indexing or
slicing that is provably out of range, and type assertions that provably cannot
succeed, for example:

```text
main.go:7:6: nil pointer dereference: p is nil
main.go:5:10: division by zero: d is zero
main.go:6:10: index out of range: index 3 for length 3
main.go:9:10: impossible type assertion: r is *strings.Reader, not *bytes.Buffer
main.go:12:5: nil pointer dereference: cfg may be nil
```

The analyzers are conservative: when they cannot establish a value, they stay
silent. Other panic categories and broader safety checks are not implemented
yet.

## Implemented analysis

### `nilptr`

Reports dereferences where a local pointer is definitely nil on every path that
reaches the expression. This includes explicit dereferences (`*p`), field
selection through a nil pointer (`p.Field`), indexing or slicing a nil pointer
to an array, and ranging over a nil pointer to an array.

The analysis is flow-sensitive for declarations, assignments, `if`/`else`, loop
conditions and `switch` cases. It follows values across same-package function
boundaries: tracked arguments are seeded into a callee's parameters, and a
call's result is evaluated per call site by analyzing the callee with those
argument values bound. Evaluation is argument-sensitive, so decidable branch
conditions are pruned: a callee that returns nil only for non-positive IDs is
known to return non-nil when called with a positive constant, and the nil path
is not reported. Nilness propagates transitively through chains of calls.

Nilness is one of four states: nil, non-nil, **maybe nil** (a known possibility,
for example a function with mixed return paths), or unknown (nilness could not be
established). A dereference of a maybe-nil pointer is reported as a potential
risk (`may be nil`); unknown values are not reported. Recursive cycles yield
unknown. The analyzer does not reason through function values, methods, imported
functions, interfaces, maps, channels or concurrent code. Variables whose address
is taken or that are captured by a closure are treated as unknown.

A missing diagnostic means the analyzer could not prove a nil dereference, not
that the code is safe.

### `divzero`

Reports integer division (`/`) or modulo (`%`) where the divisor is definitely
zero on every path that reaches the operation. Floating-point and complex
division are ignored because they do not panic.

Like `nilptr`, the analysis is intraprocedural and flow-sensitive for
declarations, assignments, zero comparisons, `if`/`else`, loops and `switch`
cases. It also follows integer arguments into same-package functions: a tracked
divisor argument is seeded into the callee's parameter, so a division by a
parameter is reported at the callee. Constant expressions are evaluated through
`go/types`. Variables whose address is taken or that are captured by a closure
are treated as unknown.

The analyzer does not track package-level variables, values reached through
pointers, function results, type parameters or interprocedural flow. A missing
diagnostic means it could not prove a zero divisor, not that the code is safe.

### `bounds`

Reports indexing and slicing that is provably out of range. Constant indices
that are out of range are already rejected by the compiler, so the analyzer
focuses on values established by local flow, for example an index variable
assigned `len(a)` and then used to index `a`.

It tracks local integer variables whose value is a constant, including `len` of
an array, and the length of local arrays, slice literals, `make` calls with a
known length, string constants, and reslices of those values. It checks:

- indexing an array, pointer to array, slice or string;
- two- and three-index slicing, including negative bounds, `low > high` and
  bounds beyond an array or string length.

For slices an explicit high bound may extend up to the capacity, which is not
tracked, so only provable violations are reported. It also follows integer and
sequence arguments into same-package functions: a tracked index or a known
length is seeded into the callee's parameter, so an out-of-range operation on a
parameter is reported at the callee. Values assigned in loops, multiple-value
assignments, address-taken variables and closure-captured variables are treated
as unknown. A missing diagnostic means the analyzer could not prove an
out-of-range operation, not that the code is safe.

### `assert`

Reports single-value type assertions that provably cannot succeed. The compiler
already rejects an assertion from an interface to a concrete type that does not
implement it, so the analyzer targets the cases the compiler cannot see: a local
interface variable whose concrete dynamic type is statically known and is
incompatible with the asserted type.

It tracks local interface variables through declarations and assignments,
recording the concrete dynamic type stored in them. An assertion is reported
when that dynamic type:

- is not identical to the asserted concrete type; or
- does not implement the asserted interface type.

Only the single-value form `v := x.(T)` is reported, because the two-value form
`v, ok := x.(T)` does not panic. It also follows interface arguments into
same-package functions: a tracked dynamic type is seeded into the callee's
parameter, so an impossible assertion on a parameter is reported at the callee.
Values whose dynamic type cannot be established (interface-typed right-hand
sides, function results, and interface values reached through fields, maps or
channels) are treated as unknown. Variables assigned in loops, address-taken
variables, closure-captured variables and multiple-value assignments are treated
as unknown. A missing diagnostic means the analyzer could not prove an
impossible assertion, not that the code is safe.

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
│   ├── assert/               impossible type assertion analyzer
│   ├── bounds/               index and slice bounds analyzer
│   ├── divzero/              division/modulo by zero analyzer
│   └── nilptr/               nil pointer dereference analyzer
├── internal/reporting/       reserved for future diagnostic presentation
├── testdata/src/assert/      analyzer fixtures
├── testdata/src/bounds/      analyzer fixtures
├── testdata/src/divzero/     analyzer fixtures
├── testdata/src/nilptr/      analyzer fixtures
├── README.md
├── CONTRIBUTING.md
├── LICENSE
├── i.md
├── .gitignore
├── go.mod
└── Makefile
```

`internal/analyzer` is a registry, not a home for analysis implementations.
Analyzer implementations live in focused subpackages and are wired into the
registry.

### CLI design

The interface is `sheriff check <packages>`. The official `multichecker` driver
expects package patterns directly, so the command requires and strips a leading
`check` subcommand before delegating; driver flags may appear before it. Any
other first positional argument is rejected with an explicit error and usage
rather than being passed to the driver as a package pattern, where it would fail
later with a confusing message. This keeps flag parsing, package loading, type
checking and diagnostics in the driver, and avoids a CLI framework for a single
subcommand. If the command surface grows, that decision can be revisited.

## Planned analysis directions

These are directions, not current capabilities. `nilptr`, `divzero`, `bounds`
and `assert` are implemented; the remaining items are not. Panic-inducing paths
are the first focus; the project is not limited to panics.

- Invalid channel operations.
- Explicit panic detection.
- Assumption and invariant verification.
- Cross-function data-flow analysis.
- Control-flow analysis.
- Additional safety properties that can be established statically.

## Known limitations

- The `nilptr`, `divzero`, `bounds` and `assert` analyzers are implemented.
- All four analyzers are conservative. They follow tracked values passed as
  arguments into same-package functions, seeding the callee's parameter and
  analyzing its body. `nilptr` also evaluates each call's single pointer result
  per call site, binding the actual argument values and pruning decidable
  branches, so nilness propagates transitively through same-package call chains
  and a provably non-nil result stays silent. `nilptr` argument evaluation reads
  constants from the call-site expression (literals and constant expressions);
  integer constants held in the caller's local variables are not propagated into
  the evaluation, so `id := 10; f(id)` is treated like an unknown argument. A
  callee that uses loops, `switch`, `select` or `goto` makes the result unknown
  rather than guessed, and recursive cycles yield unknown. None of the analyzers
  resolve function values, methods or imported functions. `nilptr` does not track
  pointer values through interface values, maps, channels, function values or
  goroutines. `divzero` does not track package-level variables, values reached
  through pointers, function results or type parameters. `bounds` does not track
  capacities, loop induction values or interprocedural lengths. `assert` does not
  track dynamic types through fields, maps, channels or type parameters.
- Variables whose address is taken or that are captured by a closure are not
  tracked by any analyzer, even when they are never modified indirectly.
- `nilptr` does not report nil interfaces, nil function calls, nil map writes or
  channel operations; those require separate rules.
- `divzero` ignores floating-point and complex division because it does not
  panic.
- `bounds` checks an explicit slice high bound against the length only for
  arrays and strings. For slices, bounds above the length but within the
  capacity are legal and are not reported.
- `assert` reports only the single-value form, because the two-value form does
  not panic. It does not report an assertion on a nil interface, which also
  panics; that requires separate reasoning about nilness.
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

Copyright 2026 [@its-ernest](https://github.com/its-ernest).
