# Limitations

Sheriff is conservative: it reports only what it can establish, and stays silent
otherwise. A clean exit does not mean a program is safe. The current gaps are
collected as runnable cases in
[`testdata/limitations`](../testdata/limitations/limitations.go).

## General

- Analysis is per package and mostly intraprocedural. Cross-function tracking is
  one level for arguments and (in `nilptr`) return values; it is not a general
  interprocedural framework.
- Function values, methods, imported functions, interfaces, maps, channels and
  goroutines are not reasoned through.
- Variables whose address is taken or that are captured by a closure are treated
  as unknown.
- Summaries are context-insensitive: a function that returns nil only on some
  inputs is treated as maybe nil at every call.

## nilptr

- Function values that hold a **function literal** are not resolved.
- **Method values** (`f := x.M`) and imported functions are not resolved.
- Pointer values through **interfaces**, **maps**, **channels** and
  **goroutines** are not tracked.
- Recursive and mutually recursive functions yield unknown.
- Nil interfaces, nil function calls, nil map writes and channel operations are
  not reported (they need separate rules).

## divzero

- Package-level variables are not tracked.
- Values reached through pointers are not tracked.
- Function results and type parameters are not tracked.
- Floating-point and complex division are ignored because they do not panic.

## bounds

- Capacities are not tracked; a slice's explicit high bound may extend up to the
  capacity, so only provable violations are reported.
- Loop induction values are not tracked.
- Arrays reached through struct fields (selectors) are not length-tracked.

## assert

- Dynamic types through struct fields, maps, channels and type parameters are
  not tracked.
- Only the single-value form is reported; the two-value form `v, ok := x.(T)`
  does not panic.
- An assertion on a nil interface, which also panics, is not reported.

## Reporting

- Findings are diagnostics, not proof of safety. Sheriff reasons only about what
  its analyzers implement; dynamic values, reflection, `unsafe` and long
  cross-package flows may remain beyond its reach.