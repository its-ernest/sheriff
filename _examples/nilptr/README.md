# nilptr examples

Small programs that trigger a nil pointer dereference Sheriff reports. Each file
is a variant; `main.go` is the basic case, and the others exercise specific
resolution paths. Files with a `//go:build ignore` tag are standalone: run them
by naming the file.

## Run a program

```sh
go run _examples/nilptr/main.go
go run _examples/nilptr/function_value.go
go run _examples/nilptr/method.go
```

Each panics at runtime.

## Analyze one

```sh
sheriff check _examples/nilptr/main.go
sheriff check _examples/nilptr/function_value.go
sheriff check _examples/nilptr/method.go
```

## Variants

| File | What it shows |
| --- | --- |
| `main.go` | a nil pointer is declared and dereferenced |
| `function_value.go` | a call through a function value (`factory := nilFactory`) |
| `method.go` | a method call returning nil (`provider.GetConfig()`) |

These correspond to limitations that have been fixed and removed from
`testdata/limitations`; they stay here as regression examples.