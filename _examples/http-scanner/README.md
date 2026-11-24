# http-scanner

A small HTTP scanner that demonstrates Sheriff finding defects across a
realistic multi-package project. Each defect lives in the package where it
naturally belongs.

## Layout

- `cmd/scanner` — entry point.
- `internal/scanner` — nil pointer dereference (nil response).
- `internal/parse` — index out of range (empty buffer).
- `internal/middleware` — impossible type assertion (non-logger value).

## Run

```sh
cd _examples/http-scanner
go run ./cmd/scanner   # panics
```

## Analyze the whole project

```sh
cd _examples/http-scanner
sheriff check ./...
```

Expected: a `nilptr` finding in `scanner`, a `bounds` finding in `parse`, and an
`assert` finding in `middleware`.