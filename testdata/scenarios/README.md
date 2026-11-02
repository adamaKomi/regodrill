# Scenario catalog

Each directory is one end-to-end run of `regodrill run`, checked by `cmd/regodrill/scenarios_test.go`:

1. The CLI runs on the directory (or on the paths listed in its `args` file) twice: with test selection and
   with `--no-select`. **Both runs must give the same output.** Test selection is an optimization; it must
   never change a verdict.
2. The output is compared with `expected.txt`: the exit code, the error message if any, and one line per
   mutant with its rule, change and verdict (`killed by <test>`, `survived`, `no_coverage`, `timeout`,
   `stillborn: <compile error>`).

Comments in the fixtures state the expected verdict of each mutant and why. `expected.txt` must agree with
them.

## Adding a scenario

1. Create a directory with the policy, its tests, and optionally an `args` file: one flag per line in
   `--name=value` form, or a path relative to the directory. `#` starts a comment.
2. Write the expected verdict of each mutation as a comment in the policy, **before** running anything.
3. Run `go test ./cmd/regodrill -run TestScenarios -update`, then compare `expected.txt` with your comments.
   A difference is either a mistake in the fixture or a bug in regodrill. Never accept a line you cannot
   explain.

## Catalog

### Operators

| Scenario | What it checks |
|---|---|
| `comparison-boundaries` | `COMP_BOUNDARY` on body expressions and on a rule value; a boundary that no test uses survives; `todo_` tests are skipped |
| `default-values` | `DEFAULT_FLIP` on booleans only; a default that no test ever falls back to is `no_coverage` |
| `expression-removal` | `EXPR_REMOVE` killed and surviving in the same body |
| `stillborn-filter` | `EXPR_REMOVE` skips a declaration that the rule needs, keeps one that something else can bind (ref key iteration) or that acts as a guard, and leaves `=` to the compiler |
| `timeout` | A mutant that makes evaluation explode is stopped and counted as `timeout` |

### Language constructs

| Scenario | What it checks |
|---|---|
| `else-chain` | Mutations inside `else` branches |
| `functions` | Function bodies; two definitions of one function, where a mutant causes a conflict error or an undefined result |
| `ref-head-rules` | Rules with ref heads (`checks.labels.present`), a variable in the head (`roles[name]`) |
| `iteration` | `every` bodies, comprehensions, `some ... in`; removing a binding is stillborn |
| `multiple-definitions` | A complete rule with two definitions (early exit), an incremental set split over two files |
| `rego-v0` | Rego v0 syntax with `--v0-compatible`, tests in the policy's package |
| `line-endings-and-unicode` | CRLF line endings and multi-byte characters before the mutated tokens |

### How tests reach rules

| Scenario | What it checks |
|---|---|
| `indexed-rule-and-duplicate-tests` | Rule indexing skips a body that a killing test never covers; a duplicate test name renamed `test_x#01` |
| `mocked-rules` | A rule replaced by `with` in every test is `no_coverage`; a mocked call survives |
| `whole-package-query` | Tests that query `data.pkg` instead of a rule |
| `dynamic-lookup` | A rule looked up through a variable (`checks[name]`) |
| `cross-package-library` | A library in another package and directory, tested only through its caller |

### Project layout

| Scenario | What it checks |
|---|---|
| `test-helpers-file` | A `_test.rego` file without `test_` rules (fixtures, helpers) is not mutated |
| `data-files` | `data.json` and `data.yaml` are loaded like `opa test` does, rooted at their directory |
| `multiple-roots` | Policies and tests given as two paths; files outside them are not loaded |
| `absolute-path` | An absolute path gives the same mutants as a relative one (editors and CI pass absolute paths) |

### Errors

| Scenario | Exit code | What it checks |
|---|---|---|
| `baseline-failure` | 2 | A test fails on the unmodified policy: nothing is mutated |
| `no-tests` | 2 | Only skipped tests: nothing to measure |
| `syntax-error` | 2 | A Rego file does not parse |
| `v0-without-flag` | 2 | Rego v0 parsed as v1: the error suggests `--v0-compatible` |
| `invalid-data-file` | 2 | A data file is not valid JSON |
| `missing-path` | 1 | A path that does not exist is a usage error |

## Not covered yet

- Operators of milestone M1 (spec §5): each new operator gets its own scenario.
- Bundles (`.tar.gz`, `.manifest` roots), `--ignore`, symlinks.
- Tests that depend on time or randomness (spec §10: flaky baseline detection).
- `http.send` and network built-ins (spec §10: network off by default).
- Built-in errors in tests (`--strict-builtin-errors`), `print` output.
- `with` on functions and built-ins (`with time.now_ns as ...`).
- Very large data files (store cost per run).
