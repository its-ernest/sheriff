# Sheriff

### The Go Safety Build System

**Find the bugs before they find your users.**

Go code can compile successfully and still contain runtime hazards. Sheriff aims to catch those problems before they reach production.

## How it works

```text
Go source
    |
    v
 Sheriff
    |
 Analyze
    |
 Detect hazards
    |
 Suggest safe fixes
    |
 Validate
    |
    v
 Go compiler
    |
 Safer binary
```

Sheriff is being built to analyse application code and dependencies, identify potential failures, apply safe corrections in an isolated workspace, and validate the result before building.

*Remediation and guarded builds are planned capabilities, not yet implemented.*

## Installation

```bash
go install github.com/its-ernest/sheriff/cmd/sheriff@latest
```

## Usage

```bash
sheriff check ./...
```

## Documentation

- [Usage](docs/USAGE.md)
- [Analyzers](docs/ANALYZERS.md)
- [Architecture](docs/ARCHITECTURE.md)
- [Roadmap](docs/ROADMAP.md)
- [Limitations](docs/LIMITATIONS.md)
- [Contributing](docs/CONTRIBUTING.md)

---

**Sheriff — Building safer Go software, one analysis at a time.**

