# Proposal: Dependency & Project Remediation

**Project:** Sheriff  
**Target release:** v1.0.0  
**Current release:** v0.1.6-alpha  
**Status:** Proposed

## 1. Overview

Sheriff will introduce a remediation system that allows developers to identify and automatically correct eligible safety issues in their Go projects and external dependencies.

The feature will support:

- Remediation of the project's own source code.
- Remediation of selected external dependencies.
- Remediation of every resolved module in the dependency graph.

The original project and Go module cache must never be modified directly.

This feature is planned for v1.0.0. Current alpha development will focus on strengthening Sheriff's analysis engine before remediation is introduced.

## 2. Configuration

Sheriff will use a `sheriff.toml` configuration file.

Example:

```toml
version = 1

[project]
name = "myapp"

[analysis]
dependencies = true
tests = true

[remediation]
enabled = true
auto_apply = true
require_validation = true

modules = [
    ".",
    "github.com/example/config@v1.4.0",
    "github.com/example/auth@v2.1.0"
]

[report]
html = true
json = true
output = ".sheriff/reports"
```

### Module selection

The `modules` list determines which modules are eligible for remediation.

Sheriff will support three selection types:

| Selector | Meaning |
|---|---|
| `.` | The root project module |
| `module@version` | A specific external module and version |
| `all` | Every resolved module, including the root project |

## 3. Usage Examples

### A. Remediate the project's own code

```toml
[remediation]
enabled = true
auto_apply = true

modules = ["."]
```

Sheriff analyses the root module and applies eligible corrections to its isolated workspace.

External dependencies are analysed if dependency analysis is enabled, but are not remediated.

### B. Remediate selected dependencies

```toml
[remediation]
enabled = true
auto_apply = true

modules = [
    ".",
    "github.com/example/config@v1.4.0",
    "github.com/example/auth@v2.1.0"
]
```

Sheriff can remediate the root project and the two specified dependencies.

Other dependencies remain eligible for analysis but not automatic modification.

### C. Remediate everything

```toml
[remediation]
enabled = true
auto_apply = true

modules = ["all"]
```

Sheriff considers every resolved module eligible for remediation:

- Root project.
- Direct dependencies.
- Transitive dependencies.

Only fixes considered safe by Sheriff's remediation engine may be applied automatically.

`all` does not mean every finding must be fixed.

### Configuration rules

- `.` resolves to the root Go module.
- Module versions must match the versions selected by Go.
- Unversioned external module paths should not be accepted for remediation.
- `all` cannot be combined with individual module selectors.
- Duplicate module declarations should be rejected or normalised.
- Invalid or unresolved module declarations must produce clear errors.

## 4. Analysis and Remediation Behaviour

Analysis and remediation are separate operations.

When dependency analysis is enabled, Sheriff should inspect the complete resolved dependency graph regardless of the remediation selection.

For each finding, Sheriff determines whether it is eligible for automatic correction.

Findings have four possible remediation outcomes:

| Status | Meaning |
|---|---|
| Auto-fixed | A safe correction was applied |
| Requires review | A proposed correction needs approval |
| Unresolved | No safe correction is available |
| Validation failed | The correction did not pass validation |

Sheriff must never classify a finding as fixed merely because the project compiles.

## 5. Isolated Workspace

All remediation must happen inside a controlled workspace.

Sheriff must:

- Preserve the original project source.
- Never modify Go's shared module cache.
- Preserve module versions and dependency relationships.
- Record every applied patch.
- Validate modified packages and the consuming project.
- Keep the resulting workspace reproducible.

```
myapp/
├── go.mod
├── go.sum
├── main.go
├── internal/
├── cmd/
├── sheriff.toml
│
└── .sheriff/
    ├── workspace/
    │   ├── project/
    │   │   ├── main.go
    │   │   └── internal/
    │   │
    │   └── modules/
    │       ├── github.com/
    │       │   └── example/
    │       │       └── config/
    │       │
    │       └── github.com/
    │           └── example/
    │               └── auth/
    │
    ├── patches/
    │   ├── config.patch
    │   └── auth.patch
    │
    └── reports/
        ├── sheriff-report.html
        └── sheriff-report.json
```

Sample .gitignore:
```shell
.sheriff/workspace/
.sheriff/cache/
```

`sheriff build` should compile from the remediated workspace, not from the original project. And if remediation or validation fails, it must not silently fall back to compiling the unremediated source while claiming success.

Go module replacements and build overlays may be used where appropriate.

The implementation must account for Go workspaces, `replace` directives, build tags, tests, `go:embed`, generated files and other project build requirements.

## 6. CLI Proposal

```bash
# Inspect project and dependencies
sheriff audit ./...

# Apply configured remediations
sheriff fix

# Review findings requiring attention
sheriff review

# Analyse, remediate and delegate compilation to Go
sheriff build ./...
```

Final command behaviour and flags will be established during implementation.

## 7. Reporting

Sheriff should generate HTML and JSON reports.

Example:

```text
SHERIFF REMEDIATION REPORT

Modules inspected:          24
Modules eligible:            4

Findings detected:          18
Automatically fixed:         8
Requires review:             6
Unresolved:                   4

Validation:
  Dependency tests:       PASSED
  Project tests:          PASSED
  Build:                  PASSED

Original source modified:     NO
Module cache modified:       NO
```

Reports must identify the affected module, exact version, source location, finding, applied correction and validation result.

## 8. Development Roadmap

### v0.1.x-alpha — Analysis foundation

- Strengthen nil pointer analysis.
- Improve interprocedural analysis.
- Improve control-flow and reachability tracking.
- Reduce false positives.
- Establish reliable diagnostics and comprehensive tests.

### Pre-v1.0.0 — Analysis maturity

- Expand supported safety analyzers.
- Introduce a unified finding model.
- Improve cross-package and dependency analysis.
- Establish stable reporting.
- Develop reliable build orchestration.
- Test dependency resolution and workspace handling.

### v1.0.0 — Remediation

- Introduce `sheriff.toml`.
- Support root, selected and all-module remediation.
- Introduce isolated dependency workspaces.
- Implement conservative automatic fixes.
- Support developer-reviewed corrections.
- Validate all applied patches.
- Deliver HTML, JSON and interactive CLI reports.

## 9. Non-negotiable Principles

1. Analyse dependencies independently of remediation selection.
2. Never modify the original project silently.
3. Never modify the shared Go module cache.
4. Never silently change dependency versions.
5. Never invent business logic to silence findings.
6. Never report unresolved findings as fixed.
7. Preserve reproducibility and patch history.
8. Treat successful compilation as validation evidence, not proof of complete safety.

## 10. Final Objective

Sheriff should allow a Go developer to inspect and improve the safety of their own project and the external packages it depends on.

Package authors should be able to use Sheriff to strengthen their libraries. Package consumers should be able to independently inspect and remediate dependencies without waiting for upstream changes.

**v1.0.0 should mark the point where Sheriff moves beyond identifying potential hazards and becomes capable of safely correcting the ones it can justify.**