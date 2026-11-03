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
| `equivalent-mutant` | A mutant that parses to the same policy (a duplicate set element dropped) is `equivalent`: not run, not scored |
| `stillborn-filter` | `EXPR_REMOVE` skips a declaration that the rule needs, keeps one that something else can bind (ref key iteration) or that acts as a guard, and leaves `=` to the compiler |
| `timeout` | A mutant that makes evaluation explode is stopped and counted as `timeout` |
| `operator-comp-negate` | `COMP_NEGATE` on `==`, `!=`, `<`, `<=`, `>`; a negation hidden by another definition survives |
| `operator-not-toggle` | `NOT_TOGGLE` removes and adds `not`; never on declarations, defaults or implicit bodies |
| `operator-rule-remove` | `RULE_REMOVE` across two files, partial sets and functions; single definitions and defaults are left alone |
| `operator-rule-remove-v0` | `RULE_REMOVE` on Rego v0 chained bodies and body-less function definitions |
| `operator-const-string` | `CONST_STRING` in comparisons, membership and raw strings; not in collections or other calls |
| `operator-const-number` | `CONST_NUMBER` with `n+1` and `n-1`, negative numbers; not decimals or call arguments |
| `operator-collection-drop` | `COLLECTION_DROP` on sets, arrays, multi-line literals with a trailing comma; not on single elements or non-constants |
| `operator-quant-weaken` | `QUANT_WEAKEN` on `every x` and `every k, v` |

The scenarios written before these operators existed pin `--operators` to the first three, so that each
scenario stays about what it was written for.

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
| `parameterized-tests` | A parameterized test (`test_x[name] if ...`): mutants run through OPA's test runner, which judges sub-results |
| `duplicate-tests-across-files` | One test name in two files: selection is turned off, names stay stable from run to run |
| `print-and-mocks` | `print` calls are never mutated; a function replaced with `with` in every test is `no_coverage`; a mocked built-in (`time.now_ns`) |
| `builtin-errors` | A built-in error is an undefined result, as in `opa test`: a type guard is equivalent for some inputs and not others |

### Project layout

| Scenario | What it checks |
|---|---|
| `test-helpers-file` | A `_test.rego` file without `test_` rules (fixtures, helpers) is not mutated |
| `data-files` | `data.json` and `data.yaml` are loaded like `opa test` does, rooted at their directory |
| `multiple-roots` | Policies and tests given as two paths; files outside them are not loaded |
| `ignore-files` | `--ignore '*.yaml'` skips Kubernetes manifests next to the policies, like `opa test --ignore` |
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
| `network-off` | 2 | `http.send` is off by default; the error suggests `--allow-network` |
| `yaml-manifests` | 2 | Two manifests load as conflicting data (merge error); the error suggests `--ignore` |
| `config-file` | 3 | Paths, exclusions, operators, an ignore and the threshold from `.regodrill.yaml`; the score is below the threshold |

### Ignores and settings

| Scenario | What it checks |
|---|---|
| `inline-ignores` | `regodrill:ignore` with and without operator IDs and reasons, `ignore-next-line`, warnings for a stale annotation and an unknown operator |
| `network-allowed` | `--allow-network` restores `http.send`; no test evaluates it, so no request is sent |
| `config-pattern-ignore` | A configuration ignore by line pattern (Gatekeeper kind filters), and a warning for a pattern that matches nothing |

Commands and flags that the scenario harness does not cover (`list`, `operators`, `version`, `--output`,
SARIF, `--since` in a temporary git repository, invalid flags and configuration files) are tested in
`cmd/regodrill/cli_test.go`.

## Not covered yet

- `LOGIC_SWAP` (spec §5): waits for OPA with `and`/`or`.
- Bundles (`.tar.gz`, `.manifest` roots), symlinks.
- Unstable tests (spec §10) have a unit test (`TestUnstableBaseline`), not a scenario: a deterministic
  scenario for a test that passes then fails needs a controllable source of time or randomness.
- Very large data files (store cost per run).
