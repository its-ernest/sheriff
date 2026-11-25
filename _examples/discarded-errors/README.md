# discarded-errors

A small project with discarded errors, used to exercise `sheriff check` and the
experimental `sheriff build` remediation. Each file is its own `package main` /
library so it can be run and analyzed as a project.

## Check

```sh
cd _examples/discarded-errors
sheriff check ./...          # reports discarded errors
sheriff check ./cmd/app      # also covers internal/store (imported)
```

## Build (remediate)

```sh
sheriff build ./...
```

Copies the project to an isolated workspace, fixes what it can prove, and prints
the workspace path. The original source is never modified. Inspect the workspace
to see the result.

## Cases

| Location | Shape | Outcome |
| --- | --- | --- |
| `store.Load` | returns `(*User, error)` | fixed: `err` captured and wrapped |
| `store.Save` | returns `[]byte` (no error) | reported for review |
| `store.LoadFirst` | returns `(error, int)` | reported for review (error not last) |

`store.Describe` handles its error, so it is not reported.