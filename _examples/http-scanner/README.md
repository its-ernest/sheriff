# http-scanner

A realistic HTTP scanner used to exercise Sheriff across a whole project. It
contains nil pointer cases (definite, maybe, proven-safe, unknown), discarded
errors, and a concurrent worker pool.

## Layout

- `cmd/scanner` — entry point.
- `internal/scanner` — nil pointer cases:
  - `DefiniteNil` — a config never initialised (definite nil).
  - `MaybeNil` — a config that may be nil (maybe nil).
  - `ProvenSafe` — a config proven non-nil from the argument.
  - `Unknown` — a config from an unresolved function value (unknown).
- `internal/parse` — discarded errors (one fixable, one review).
- `internal/fetch` — a concurrent worker pool (goroutines + channels).

## Check

```sh
cd _examples/http-scanner
sheriff check ./...        # nilptr (definite + maybe) and discarded findings
```

## Build (remediate + compile)

```sh
sheriff build -o scanner ./...
```

Prints the report, remediates what it can in an isolated workspace (the
fixable discarded error), lists what needs review, and builds the `scanner`
binary. The original source is never modified.