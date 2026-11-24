# Analyzers

Sheriff ships four analyzers. Each is intraprocedural, flow-sensitive, and
conservative: it reports only what it can establish, and stays silent otherwise.
Diagnostics are advisory.

| Analyzer | Reports |
| --- | --- |
| [`nilptr`](#nilptr) | dereferences of pointers that are provably nil or possibly nil |
| [`divzero`](#divzero) | integer division or modulo by a provably zero divisor |
| [`bounds`](#bounds) | indexing or slicing that is provably out of range |
| [`assert`](#assert) | type assertions that provably cannot succeed |

## nilptr

Reports nil pointer dereferences: explicit `*p`, field selection `p.Field`,
indexing or slicing a nil pointer to an array, and ranging over one.

It tracks each pointer's nilness as one of four states — nil, non-nil, **maybe
nil** (a known possibility, for example a function with mixed return paths), or
unknown (nilness could not be established). It follows values across same-package
function boundaries: arguments are seeded into parameters, and a function's
single pointer result is summarized from its reachable returns and applied at its
call sites, transitively. Function values that hold a same-package function and
method calls on same-package types are resolved.

A dereference of a maybe-nil pointer is reported as a potential risk
(`may be nil`); unknown values are not reported.

```go
func describe(cfg *Config) string {
	return cfg.Host // nil pointer dereference: cfg may be nil
}
```

## divzero

Reports integer division or modulo by a divisor that is provably zero. It tracks
integer constants for local variables and follows integer arguments into
same-package functions. Floating-point and complex division are ignored because
they do not panic.

```go
func average(total, count int) int {
	return total / count // division by zero: count is zero
}
```

## bounds

Reports indexing and slicing that is provably out of range. It tracks integer
constants and the lengths of local arrays, slice literals, `make` calls with a
known length, string constants, and reslices of those values, and follows integer
and sequence arguments into same-package functions.

A slice's explicit high bound may extend up to its capacity, which is not
tracked, so only provable violations are reported.

```go
func first(items []string) string {
	return items[0] // index out of range: index 0 for length 0
}
```

## assert

Reports single-value type assertions `v := x.(T)` that provably cannot succeed.
The compiler already rejects an assertion from an interface to a concrete type
that does not implement it, so the analyzer targets what the compiler cannot see:
a local interface variable whose concrete dynamic type is statically known and
is incompatible with the asserted type. The two-value form `v, ok := x.(T)` is
ignored because it does not panic.

```go
func consume(r io.Reader) {
	_ = r.(*bytes.Buffer) // impossible type assertion: r is *strings.Reader, not *bytes.Buffer
}
```

## Shared behaviour

- Diagnostics point at the dangerous operation; the message names the value.
- Values assigned in loops, multiple-value assignments, address-taken variables
  and closure-captured variables are treated as unknown.
- Function values, methods, imported functions, interfaces, maps, channels and
  goroutines are not reasoned through (see [Limitations](LIMITATIONS.md)).