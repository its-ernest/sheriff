# concurrent-pipeline

A concurrent, shard-based pipeline that demonstrates Sheriff finding defects in
a project built with goroutines and channels.

## Layout

- `cmd/pipeline` — entry point.
- `internal/pipeline` — the channel/goroutine pipeline, plus:
  - `config.go` — nil pointer dereference (nil config).
  - `shard.go` — index out of range (out-of-range shard).
  - `handler.go` — impossible type assertion (non-string message).

The pipeline itself (`pipeline.go`) moves values over a channel and fans work
out to workers; the defects are in the helpers with statically known values.

## Run

```sh
cd _examples/concurrent-pipeline
go run ./cmd/pipeline   # the pipeline runs, then panics
```

## Analyze the whole project

```sh
cd _examples/concurrent-pipeline
sheriff check ./...
```

Expected: `nilptr` in `config.go`, `bounds` in `shard.go`, `assert` in
`handler.go`. Values received from channels are a known limitation, so the
findings come from values with known types.