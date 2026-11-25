# type-safety

A small project that demonstrates type-safety defects: impossible type
assertions, out-of-range indexing, and division by zero.

## Layout

- `cmd/tool` — entry point.
- `internal/decode` — impossible type assertion (`v.(string)` on an `int`).
- `internal/measure` — index out of range (empty slice) and division by zero.

## Check

```sh
cd _examples/type-safety
sheriff check ./...        # assert, bounds and divzero findings
```

## Build (remediate + compile)

```sh
sheriff build -o tool ./...
```

Prints the report and, where it can, remediates in an isolated workspace. The
original source is never modified.