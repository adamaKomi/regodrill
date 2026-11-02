# regodrill

**Mutation testing for Rego policies.** regodrill tells you whether the tests of an
[Open Policy Agent](https://www.openpolicyagent.org/) policy would actually catch a bug.

> Status: **prototype (spike M0)**. The engine works end to end with 3 operators and has run on the whole
> [gatekeeper-library](https://github.com/open-policy-agent/gatekeeper-library). Flags, output formats and
> operators will change before v0.1.0. See [Roadmap](#roadmap).

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
testdata/authz/policy.rego:6:17  COMP_BOUNDARY  ">=" → ">"                 SURVIVED
testdata/authz/policy.rego:7:2   EXPR_REMOVE    "input.user.active" → ""   SURVIVED

Mutation score: 50.0 %   killed 2 · timeout 0 · survived 2 · no coverage 0 · stillborn 0
Duration: 10 ms (baseline + coverage 6 ms, 4 mutants, 8 workers, test selection true)
```

Two gaps:

- No test checks a user who is exactly 18. With `>`, an 18-year-old is wrongly denied, and no test notices.
- No test checks an inactive adult. Without the `input.user.active` condition, inactive users get access,
  and no test notices.

Adding `test_18_allowed` and `test_inactive_adult_denied` kills both mutants.

## Install

regodrill is not released yet. Build it from source with Go 1.27 or later:

```console
$ git clone https://github.com/adamaKomi/regodrill.git
$ cd regodrill
$ go build -o bin/regodrill ./cmd/regodrill
$ ./bin/regodrill run path/to/policies
```

## Usage

```
regodrill run [flags] [paths...]
```

`paths` are directories or files (default: the current directory). Every `.rego` file found is loaded. Files
that define `test_` rules are test files: they are run, never mutated.

| Flag | Default | Effect |
|---|---|---|
| `--format text\|json` | `text` | Report format on stdout |
| `--parallel N` | number of CPUs | Mutants evaluated in parallel |
| `--v0-compatible` | off | Parse policies as Rego v0 (syntax before OPA 1.0, used by gatekeeper-library) |
| `--operators A,B` | all | Operators to enable |
| `--no-select` | off | Run every test for every mutant, instead of only the tests that cover the mutated line |
| `--timeout-factor F` | `5` | A mutant times out after F × the baseline duration of its tests (minimum 2 s) |

Exit codes: `0` run completed, `1` usage or load error, `2` the unmodified tests do not all pass.

### Statuses and score

| Status | Meaning | In the score |
|---|---|---|
| `killed` | At least one test fails or errors | Yes (good) |
| `timeout` | The tests took too long, for example an infinite loop introduced by the mutation | Yes, counts as killed |
| `survived` | Every test still passes | Yes (bad) |
| `no_coverage` | No test evaluates the mutated line, so no test can kill it | Yes (bad) |
| `stillborn` | The mutant doesn't compile, for example a removed assignment leaves a variable undefined | No |

```
score = (killed + timeout) / (killed + timeout + survived + no_coverage) × 100
```

### Operators

| ID | Mutation | Bug it simulates |
|---|---|---|
| `COMP_BOUNDARY` | `>=`↔`>`, `<=`↔`<` | Off-by-one on a limit (age, level, quota) |
| `EXPR_REMOVE` | Delete one expression of a rule body that has at least two | A forgotten condition |
| `DEFAULT_FLIP` | `default x := false` ↔ `true` | A fail-open policy |

Eight more operators are specified (comparison negation, `not` toggle, rule removal, string and number
constants, collection elements, `every` weakened to `some`, `and`↔`or`). See the
[specification](https://github.com/adamaKomi/rego-syntax-support/blob/main/docs/mutation-testing-spec.md#5-mutation-operators-mvp).

### JSON report

`--format json` prints a report with `schemaVersion: 1`, a `summary` (counts, score, durations), per-operator
counts, and every mutant with its location, change, status, covering tests and killing test. This format is
what the Rego Lens VS Code extension will read.

## First results on gatekeeper-library

Spike run of 2026-11-02 on [gatekeeper-library](https://github.com/open-policy-agent/gatekeeper-library) at
`a8d4799`: 51 policy directories, 1,003 tests, 1,300 mutants (`COMP_BOUNDARY` and `EXPR_REMOVE`).

- Every mutant patched the source exactly as intended (1,300 out of 1,300).
- The whole library runs in about 30 seconds on a laptop, 4 workers per directory.
- Real gaps show up. For example, `allowedrepos` never tests that an `initContainer` or an
  `ephemeralContainer` from a forbidden repository is denied: those two rules are not covered by any test.

Details, limits and next steps: [docs/spike-m0.md](docs/spike-m0.md).

## Roadmap

The full design is in the [specification](https://github.com/adamaKomi/rego-syntax-support/blob/main/docs/mutation-testing-spec.md)
(kept in the Rego Lens repository for now).

| Milestone | Content |
|---|---|
| **M0 Spike** (in progress) | Engine, 3 operators, coverage-based test selection, text and JSON output, measures on gatekeeper-library |
| M1 CLI MVP | 11 operators, `# regodrill:ignore` annotations, `.regodrill.yaml`, SARIF output, `--threshold`, `--since <git ref>`, network built-ins off by default |
| M2 Release | Binaries for Linux, macOS and Windows, `go install`, v0.1.0 |
| M3 VS Code | Survivors as diagnostics in the [Rego Lens](https://marketplace.visualstudio.com/items?itemName=AdamaKomi.rego-syntax-support) extension |
| M4 GitHub Action | Mutation testing of the lines changed by a pull request, results in code scanning |

regodrill is deterministic and uses no AI: the same policy and tests always give the same report, so it can
gate a CI pipeline.

## Development

```console
$ go vet ./...
$ go build -o bin/regodrill ./cmd/regodrill
$ ./bin/regodrill run testdata/authz
```

| Path | Content |
|---|---|
| `cmd/regodrill` | Command line, reports |
| `internal/project` | Loading and parsing the `.rego` files |
| `internal/mutate` | Operators: from the parsed AST to text patches on the original source |
| `internal/engine` | Baseline run, per-test coverage, parallel evaluation of mutants |
| `testdata/authz` | Small policy with two deliberate test gaps |

## License

[Apache License 2.0](LICENSE).
