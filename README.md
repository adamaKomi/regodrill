# regodrill

**Mutation testing for Rego policies.** regodrill tells you whether the tests of an
[Open Policy Agent](https://www.openpolicyagent.org/) policy would actually catch a bug.

> Status: **0.x**. Ten operators, test selection, text, JSON and SARIF reports; it runs on the whole
> [gatekeeper-library](https://github.com/open-policy-agent/gatekeeper-library). The JSON report is versioned
> (`schemaVersion`); flags and operators may still change before 1.0. See [Roadmap](#roadmap).

## Why

A green `opa test` run and 100 % coverage tell you that every line was *executed* by a test. They don't tell
you that a test would *fail* if that line were wrong.

In an authorization policy, that difference is a security issue. If nobody tests a user who is exactly 18, or
an inactive administrator, a wrong `>=` or a forgotten condition grants access, and every test stays green.

regodrill works like a fire drill: it sets off small, deliberate mistakes and checks that your tests react.

## How it works

1. It runs your tests once on the unmodified policy. They must all pass.
2. It creates **mutants**: copies of your policy, held in memory, each with exactly one small change, for
   example `>=` becomes `>`, or one condition is removed.
3. It runs your tests against each mutant:
   - **killed**: at least one test fails. Your tests protect this spot.
   - **survived**: every test still passes. A bug here would go unnoticed.
4. It reports a **mutation score** and the exact location of every survivor.

Your files are never modified. Mutants only exist in memory.

regodrill embeds OPA as a Go library: the policies are parsed and compiled by the same code as `opa`, and
the mutants run in parallel in a single process.

## Example

`testdata/authz/policy.rego`:

```rego
package authz

default allow := false

allow if {
	input.user.age >= 18
	input.user.active
}

allow if {
	"admin" in input.user.roles
}
```

Its tests (`testdata/authz/policy_test.rego`) check an adult (30), a minor (10) and an admin. All of them
pass, and every line is covered:

```console
$ regodrill run testdata/authz
testdata/authz/policy.rego:6:17  COMP_BOUNDARY   ">=" → ">"                  SURVIVED
testdata/authz/policy.rego:6:20  CONST_NUMBER    "18" → "19"                 SURVIVED
testdata/authz/policy.rego:6:20  CONST_NUMBER    "18" → "17"                 SURVIVED
testdata/authz/policy.rego:7:2   EXPR_REMOVE     "input.user.active" → ""    SURVIVED

Mutation score: 69.2 %   killed 9 · timeout 0 · survived 4 · no coverage 0 · stillborn 0 · equivalent 0 · ignored 0
Duration: 8 ms (baseline + coverage 3 ms, 13 mutants, 8 workers, test selection true)
```

Three gaps:

- No test checks a user who is exactly 18. With `>`, or with 19 as the limit, an 18-year-old is wrongly
  denied, and no test notices.
- No test checks a user who is 17. With 17 as the limit, a minor gets access, and no test notices.
- No test checks an inactive adult. Without the `input.user.active` condition, inactive users get access,
  and no test notices.

Adding `test_18_allowed`, `test_17_denied` and `test_inactive_adult_denied` kills all four mutants: the
score becomes 100 %.

## Install

**Binaries**: download the archive for your platform from the
[releases](https://github.com/adamaKomi/regodrill/releases) (Linux, macOS and Windows, amd64 and arm64),
check it against `checksums.txt`, and put `regodrill` on your `PATH`. A `wasip1` build is attached too, for
environments that can only run WebAssembly.

**With Go** 1.27.2 or later:

```console
$ go install github.com/adamaKomi/regodrill/cmd/regodrill@latest
$ regodrill version
```

**From source**:

```console
$ git clone https://github.com/adamaKomi/regodrill.git
$ cd regodrill
$ go build -o bin/regodrill ./cmd/regodrill
```

In VS Code, the [Rego Syntax Support](https://marketplace.visualstudio.com/items?itemName=AdamaKomi.rego-syntax-support)
extension runs it from the editor (its next version).

## Usage

```
regodrill run [flags] [paths...]    run mutation testing (default path: .)
regodrill list [flags] [paths...]   list the mutants, without running tests
regodrill operators [--format json] describe the mutation operators
regodrill version                   print the versions of regodrill and OPA
```

`paths` are directories or files (default: the current directory, or `paths` from the configuration file),
loaded like `opa test` does: `.rego` files, and JSON or YAML data files (`data.json`, `data.yaml`) rooted at
their directory. Test files are the files whose name ends with `_test.rego` or that define `test_` rules:
they are run, never mutated. That includes fixtures and helpers kept next to the tests.

| Flag | Default | Effect |
|---|---|---|
| `--format text\|json\|sarif` | `text` | Report format (`list`: text or json) |
| `--output FILE` | stdout | Write the report to a file; the text summary goes to stderr |
| `--operators A,B` | all | Operators to enable |
| `--exclude-operators A,B` | none | Operators to disable |
| `--threshold N` | none | Exit with code 3 if the score is below N (0 to 100) |
| `--since REF` | none | Only mutate the lines added or changed since the merge base of the git reference REF and HEAD, uncommitted changes and untracked files included. The score covers those lines only |
| `--config FILE` | `.regodrill.yaml` if present | Configuration file |
| `--ignore PATTERN` | none | Skip files and directories whose name matches the pattern when loading, like `opa test --ignore` (repeatable). Useful when Kubernetes manifests sit next to the policies |
| `--v0-compatible` | off | Parse policies as Rego v0 (syntax before OPA 1.0, used by gatekeeper-library) |
| `--allow-network` | off | Keep `http.send` and `net.lookup_ip_addr`. They are off by default, so that a mutant can never send a request; a policy that needs them fails at baseline with a hint |
| `--parallel N` | number of CPUs | Mutants evaluated in parallel |
| `--no-select` | off | Run every test for every mutant, instead of only the tests that look up the mutated rule. The verdicts are the same; it is only slower |
| `--timeout-factor F` | `5` | A mutant times out after F × the baseline duration of its tests (minimum 2 s) |

Exit codes: `0` run completed; `1` usage error (unknown flag or operator, missing path, invalid
configuration); `2` the baseline cannot run: a test fails on the unmodified policy, a test passes then fails
(time or randomness), a file does not parse or load, or there is no test; `3` the score is below
`--threshold`. A Rego v0 policy loaded without `--v0-compatible` gets a hint.

On a terminal, the text report is colored (`NO_COLOR` turns it off) and stderr shows the progress of the
run; redirected or in CI, the output is plain.

### Ignoring a mutant

Some mutants cannot be killed: `count(x) > 0` → `count(x) >= 1` behaves the same. After checking one,
mark it in the policy:

```rego
allow if {
	count(input.roles) > 0 # regodrill:ignore COMP_BOUNDARY,CONST_NUMBER -- same as >= 1
	# regodrill:ignore-next-line -- the gateway checks the level
	input.level >= 2
}
```

`# regodrill:ignore` applies to its own line, `# regodrill:ignore-next-line` to the next one. Operator IDs
narrow it; the text after `--` is the reason, copied into the report. Ignored mutants are not run and not
scored. An annotation that matches no mutant, or names an unknown operator, is reported as a warning.

### Configuration file

`.regodrill.yaml` in the current directory, or the file given with `--config`. Paths are relative to the
file; flags take precedence. Unknown keys are errors, so a typo never silently disables a setting.

```yaml
version: 1
paths: [policies]
exclude: ["policies/vendor/**"]   # loaded and tested, never mutated
threshold: 80
operators:
  exclude: [CONST_STRING]         # or include: [...]
ignore:
  - file: policies/authz.rego
    line: 14
    operator: COMP_BOUNDARY       # optional: all operators of the line otherwise
    reason: "age is always an integer, > 17 is tested"
  - pattern: 'input\.review\.kind\.kind =='   # every line whose text matches this regular expression
    reason: "Gatekeeper's match filters the kind"
ignore-files: ["*.yaml"]          # like --ignore
v0-compatible: false
parallel: 4
timeout-factor: 5
allow-network: false
```

### Statuses and score

| Status | Meaning | In the score |
|---|---|---|
| `killed` | At least one test fails or errors | Yes (good) |
| `timeout` | The tests took too long, for example an infinite loop introduced by the mutation | Yes, counts as killed |
| `survived` | Every test still passes | Yes (bad) |
| `no_coverage` | Every test still passes, and none of them evaluates the mutated line | Yes (bad) |
| `stillborn` | The mutant doesn't compile, for example a removed assignment leaves a variable undefined | No |
| `equivalent` | The patched source parses to the same policy as the original (a duplicate set element dropped); not run | No |
| `ignored` | An annotation or the configuration file says a person checked it; not run | No |

```
score = (killed + timeout) / (killed + timeout + survived + no_coverage) × 100
```

### Operators

Stillborn mutants (that cannot compile) are avoided where a static check can tell: 1.9 % of the mutants on
gatekeeper-library, with all ten operators. The check is conservative: on both corpora tried
(gatekeeper-library and trivy-checks), it never dropped a mutant that compiles.

| ID | Mutation | Bug it simulates |
|---|---|---|
| `COMP_BOUNDARY` | `>=`↔`>`, `<=`↔`<` | Off-by-one on a limit (age, level, quota) |
| `COMP_NEGATE` | `==`↔`!=`, `<`↔`>=`, `>`↔`<=` | An inverted ownership or identity check |
| `EXPR_REMOVE` | Delete one expression of a rule body that has at least two. An expression that binds a variable the rule still needs (`x :=`, `some x in`, `x = ...`, `items[x]`), with nothing else to bind it, is skipped: that mutant could not compile. `print` calls are never removed | A forgotten condition |
| `NOT_TOGGLE` | Remove the `not` of an expression, or add one. Not added to declarations, unifications, `print`, or expressions whose variables nothing else binds | An inverted deny list |
| `DEFAULT_FLIP` | `default x := false` ↔ `true` | A fail-open policy |
| `RULE_REMOVE` | Delete one definition of a rule that has several, in any policy file (incremental rules, partial sets, functions, Rego v0 chained bodies) | A whole access path is untested |
| `CONST_STRING` | `"admin"` → `"admin_regodrill"` in a comparison, or on the left of `in` | A role or action name typo |
| `CONST_NUMBER` | Integer `n` → `n+1` and `n-1` in a comparison | A wrong threshold |
| `COLLECTION_DROP` | Remove one element from a literal set or array of at least two constants | A privileged role is never tested |
| `QUANT_WEAKEN` | `every x in xs { body }` → `some x in xs; body` | "All" checked as "at least one" |

All ten are on by default; `--operators` selects a subset. `LOGIC_SWAP` (`and`↔`or`) waits for an OPA
version with those keywords that is at least two weeks old. See the
[specification](https://github.com/adamaKomi/rego-syntax-support/blob/main/docs/mutation-testing-spec.md#5-mutation-operators-mvp).

### JSON report

`--format json` prints a report with `schemaVersion: 1`, the tool and OPA versions, a `summary` (counts,
score, durations, threshold), warnings, per-operator counts, and every mutant with its location and `range`
(columns in UTF-16 code units, like editors), change, status, rule, covering, selected and killing tests,
and a one-sentence `message`. This format is what the Rego Syntax Support VS Code extension will read.
Every field is described in [docs/report-format.md](docs/report-format.md), and a test checks the reports
against that description.

### SARIF

`--format sarif` writes SARIF 2.1.0 for GitHub code scanning: one result per survived (`warning`) or
uncovered (`note`) mutant, one rule per operator, and `partialFingerprints.regodrillId` so alerts stay
stable across commits.

```yaml
- run: regodrill run --format sarif --output regodrill.sarif --threshold 80 policies
- uses: github/codeql-action/upload-sarif@v4
  if: always()
  with:
    sarif_file: regodrill.sarif
```

## First results on gatekeeper-library

Spike run of 2026-11-02 on [gatekeeper-library](https://github.com/open-policy-agent/gatekeeper-library) at
`a8d4799`: 51 policy directories, 1,003 tests, 1,300 mutants (`COMP_BOUNDARY` and `EXPR_REMOVE`).

- Every mutant patched the source exactly as intended (1,300 out of 1,300).
- The whole library runs in about 25 seconds on a laptop, 4 workers per directory. Since then, with all ten
  operators (2,426 mutants), it takes about 14 seconds with 8 workers: plain tests now run without the
  tracer that OPA's test runner always attaches, and stop at the first failure.
- Real gaps show up. For example, `allowedrepos` never tests that an `ephemeralContainer` from a forbidden
  repository is denied: that rule is not covered by any test. The exemption library copied into about
  fifteen templates never tests a wildcard exemption.

Details, limits and next steps: [docs/spike-m0.md](docs/spike-m0.md). Results on other public projects
(trivy-checks, conftest examples), and what does not run there and why: [docs/corpora.md](docs/corpora.md).

## Roadmap

The full design is in the [specification](https://github.com/adamaKomi/rego-syntax-support/blob/main/docs/mutation-testing-spec.md)
(kept in the extension repository for now).

| Milestone | Content |
|---|---|
| **M0 Spike** (done, decision: go) | Engine, 3 operators, test selection, text and JSON output, measures on gatekeeper-library |
| **M1 CLI MVP** (done except `LOGIC_SWAP`) | 10 of 11 operators, `# regodrill:ignore` annotations, `.regodrill.yaml`, SARIF output, `--threshold`, `--since <git ref>`, network built-ins off by default |
| **M2 Release** (done, v0.1.0) | Binaries for Linux, macOS and Windows, `go install` |
| M3 VS Code | Survivors as diagnostics in the [Rego Syntax Support](https://marketplace.visualstudio.com/items?itemName=AdamaKomi.rego-syntax-support) extension |
| M4 GitHub Action | Mutation testing of the lines changed by a pull request, results in code scanning |

regodrill is deterministic and uses no AI: the same policy and tests always give the same report, so it can
gate a CI pipeline.

## Development

```console
$ go vet ./...
$ go test -race ./...      # unit tests and the scenario catalog
$ go build -o bin/regodrill ./cmd/regodrill
$ ./bin/regodrill run testdata/authz
```

Every behavior change comes with a scenario in `testdata/scenarios`: a small policy, its tests, and the
expected verdict of each mutant. Each scenario runs with and without test selection, and both must agree.

| Path | Content |
|---|---|
| `cmd/regodrill` | Command line, reports |
| `internal/project` | Loading policies, tests and data with OPA's loader |
| `internal/mutate` | Operators: from the parsed AST to text patches on the original source |
| `internal/engine` | Baseline run with per-test tracing, test selection, parallel evaluation of mutants (a fast path without the test runner's tracer for plain tests) |
| `testdata/authz` | Small policy with two deliberate test gaps |
| `testdata/scenarios` | End-to-end scenario catalog with expected verdicts ([README](testdata/scenarios/README.md)) |

## License

[Apache License 2.0](LICENSE).
