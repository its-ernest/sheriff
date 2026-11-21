# nilptr reasoning cases

Four small programs that illustrate how the `nilptr` analyzer reasons about a
pointer's nilness. Each is a `package main` in its own directory so they can be
run and analyzed independently.

| Case | Program | Sheriff result |
| --- | --- | --- |
| Provably nil | [`definite-nil`](./definite-nil) | `nil pointer dereference: cfg is nil` |
| Provably non-nil | [`proven-safe`](./proven-safe) | silent (no diagnostic) |
| Possibly nil | [`maybe-nil`](./maybe-nil) | `nil pointer dereference: user may be nil` |
| Unknown | [`unknown`](./unknown) | silent (no diagnostic) |

## Running a case

From the repository root:

```sh
# Run the program (some cases panic by design).
go run ./_examples/nilptr-cases/definite-nil

# Analyze it.
go run ./cmd/sheriff check ./_examples/nilptr-cases/definite-nil
```

## What each case shows

- **definite-nil** — a local pointer that is never assigned is definitely nil;
  the dereference is a definite finding.
- **proven-safe** — `lookup` returns nil only for a non-positive id, but it is
  called with `10`. The analyzer evaluates the call with the argument bound,
  prunes the `id > 0` branch, and proves the result non-nil, so it stays silent.
- **maybe-nil** — the same `lookup`, but the id comes from the environment and
  cannot be established. The nil path is possible, so the analyzer reports a
  potential finding (`may be nil`).
- **unknown** — the pointer comes from a function value the analyzer cannot
  resolve. It stays silent rather than guessing.

The point of `proven-safe` and `unknown` is restraint: the analyzer reports only
what it can establish, and stays quiet when it cannot prove either safety or a
defect.