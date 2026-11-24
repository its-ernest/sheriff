# Architecture

Sheriff is a static-analysis driver built on the official Go analysis
infrastructure. It contributes the analyzers and the report formatting; package
loading, type checking and the analysis driver come from `golang.org/x/tools`.

## Layout

```text
.
├── cmd/sheriff/          CLI entry point (thin)
├── internal/
│   ├── analyzer/         analyzer registry
│   │   ├── nilptr/       nil pointer dereference analyzer
│   │   ├── divzero/      division/modulo by zero analyzer
│   │   ├── bounds/       index and slice bounds analyzer
│   │   └── assert/       impossible type assertion analyzer
│   └── reporting/        structured report formatting (text, JSON, HTML)
├── testdata/             analyzer fixtures
├── _examples/            runnable vulnerable example projects
└── docs/                 this documentation
```

`internal/analyzer` is a registry, not a home for analysis implementations. Each
analyzer lives in its own focused subpackage and is wired into the registry.

## Request flow

```text
sheriff check ./...
        |
        v
strip "check" subcommand        (cmd/sheriff)
        |
        v
go/packages.Load                (load + type-check the packages)
        |
        v
go/analysis/checker.Analyze     (run each analyzer over each package)
        |
        v
collect diagnostics             (internal/analyzer/*)
        |
        v
reporting.Report                (internal/reporting)
        |
        +--> text (default)
        +--> JSON (-json)
        +--> HTML (-html)
```

## Analysis model

- **Per package.** Each analyzer runs on one package at a time, over that
  package's syntax and type information.
- **Flow-sensitive.** State is tracked through declarations, assignments,
  `if`/`else`, loops and `switch`, and joined at control-flow merges.
- **One level of interprocedural reasoning.** Tracked arguments are seeded into a
  same-package callee's parameters, and `nilptr` summarizes return values. This
  is not a general interprocedural framework: function values, methods and
  imported functions are not resolved.
- **Conservative.** When a value's nilness or bound cannot be established, the
  analyzers treat it as unknown and stay silent.

## Reporting

`internal/reporting` defines a small, stable model:

```go
type Report struct {
	Packages int
	Findings []Finding
}

type Finding struct {
	Analyzer string
	Position string
	Message  string
}
```

Text, JSON and HTML are renderers over this model, so every output format
carries the same information. The HTML renderer embeds its CSS and filter script,
so a report is a single self-contained file.