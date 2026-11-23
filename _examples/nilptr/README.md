# nilptr examples

Small programs that trigger a nil pointer dereference Sheriff reports. Each
variant is its own `package main` in a subdirectory, so a single project-wide
run checks them all.

## Run the whole project

```sh
sheriff check ./_examples/nilptr/...
```

Or from inside the directory:

```sh
cd _examples/nilptr
sheriff check ./...
```

## Run a single program

```sh
go run ./_examples/nilptr
go run ./_examples/nilptr/function-value
go run ./_examples/nilptr/method
```

Each panics at runtime.

## Variants

| Directory | What it shows |
| --- | --- |
| `.` | a nil pointer is declared and dereferenced |
| `function-value/` | a call through a function value (`factory := nilFactory`) |
| `method/` | a method call returning nil (`provider.GetConfig()`) |

These correspond to limitations that have been fixed and removed from
`testdata/limitations`; they stay here as regression examples.