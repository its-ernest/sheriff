# Roadmap

Sheriff is in early development (`v0.1.x`/`v0.2.x` alpha). The direction is to
grow the analysis engine first, then add remediation. This document describes
directions, not current capabilities.

## Now (alpha)

- Strengthen the existing analyzers (`nilptr`, `divzero`, `bounds`, `assert`).
- Improve interprocedural tracking and reduce false positives.
- Keep diagnostics explainable and well tested.

## v0.2.x

- More analyzers, including invalid channel operations.
- Deeper interprocedural and control-flow reasoning.
- Broader coverage of common panic classes.

## v0.3.x

- Concurrency-aware analysis (channels, goroutines, shared state).
- More safety properties beyond panics.

## Pre-v1.0

- A unified finding model shared by all analyzers.
- Stable text, JSON and HTML reporting.
- Cross-package and dependency analysis.
- Reliable build orchestration.

## v1.0 — Remediation

The v1.0 goal is to move beyond identifying hazards to safely correcting the ones
Sheriff can justify: a `sheriff.toml` configuration, an isolated workspace that
never mutates the original project or the module cache, conservative automatic
fixes, developer-reviewed corrections, and validation of every applied patch.

See [`proposals/package-and-dependency-remidiation.md`](../proposals/package-and-dependency-remidiation.md)
for the full remediation proposal.

## Guiding principles

- Analyse independently of remediation selection.
- Never modify the original project or the shared module cache silently.
- Never report an unresolved finding as fixed.
- Prefer a clear implementation over an unnecessary abstraction.
- Accurate diagnostics over excessive warnings.