# `regodrill check` fixtures

Each directory is a small project for `regodrill check`. `cmd/regodrill/check_test.go` checks the exit code and
the errors of each one, and compares the whole output (stdout, stderr, exit code) with `opa check` when an
`opa` binary is on the `PATH` (`REQUIRE_OPA=1` makes that comparison mandatory).

| Directory | Content |
|---|---|
| `valid` | A Rego v1 policy and its test: no error |
| `parse-error` | A missing closing brace: `rego_parse_error` |
| `unsafe-var` | A variable used but never bound: `rego_unsafe_var_error` |
| `v0` | Rego v0 syntax: errors as Rego v1, none with `--v0-compatible` |
| `type-errors` | Calls with wrong argument types: `rego_type_error`, with `details` |
| `many-errors` | Twelve errors, more than the default limit of 10 |
| `data-ignored` | An invalid `data.json` next to a valid policy: only `.rego` files are loaded |
