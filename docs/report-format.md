# JSON report, schema version 1

`regodrill run --format json` prints this report. It is the contract with the VS Code extension and with
any other tool that reads it. `cmd/regodrill/report_contract_test.go` decodes the reports of the scenario
catalog into a copy of this description and fails on any field it does not list.

Compatibility: adding a field keeps `schemaVersion` at 1; removing or renaming one, or changing its
meaning, makes it 2. Readers should ignore fields they don't know.

## Top level

| Field | Type | Meaning |
|---|---|---|
| `schemaVersion` | number | `1` |
| `tool` | object | `name` (`"regodrill"`), `version` (regodrill), `opaVersion` (the embedded OPA library) |
| `summary` | object | Counts, score and durations, below |
| `warnings` | array of strings, optional | Stale ignore annotations, unknown operator IDs in annotations |
| `mutants` | array | One entry per mutant, in file order, then by position and operator |
| `operators` | object | Per operator ID: `total`, `killed` (timeouts included), `survived`, `noCoverage`, `stillborn`, `equivalent`, `ignored` |

## `summary`

| Field | Type | Meaning |
|---|---|---|
| `score` | number or null | `(killed + timeout) / (killed + timeout + survived + noCoverage) × 100`; null when nothing is scorable |
| `total` | number | Mutants in the report, ignored ones included |
| `killed`, `timeout`, `survived`, `noCoverage`, `stillborn`, `equivalent`, `ignored` | number | Mutants per status |
| `patchOk` | number | Mutants whose patched source parses to a different AST (every mutant that was run should) |
| `tests` | number | Tests in the baseline (skipped `todo_` tests excluded) |
| `parallel` | number | Workers |
| `select` | boolean | Test selection was on |
| `threshold` | number, optional | The `--threshold` in effect |
| `since` | string, optional | The `--since` reference: the report covers only the changed lines |
| `baselineMs`, `coverageMs`, `mutationMs`, `durationMs` | number | Durations in milliseconds |

## `mutants[]`

| Field | Type | Meaning |
|---|---|---|
| `id` | string | `file:line:column:OPERATOR:n`; stable across runs on the same source |
| `operator` | string | Operator ID (`regodrill operators`) |
| `file` | string | Path of the policy file, as loaded (relative or absolute, like the paths given) |
| `line`, `column` | number | Start of the change, 1-based; the column counts UTF-16 code units, like editors |
| `range` | object | `start` and `end`, each `{line, column}`; `end` is exclusive. An insertion has `start == end` |
| `original` | string | The replaced source text (empty for an insertion) |
| `replacement` | string | The new text (empty for a removal) |
| `rule` | string | Path of the enclosing rule, for example `data.app.authz.allow` |
| `status` | string | `killed`, `timeout`, `survived`, `no_coverage`, `stillborn`, `equivalent` or `ignored` |
| `message` | string | One sentence for people: what changed and what happened |
| `killedBy` | string, optional | For `killed`: the first failing test by name, as the test runner names it (`data.pkg.test_x`, or `data.pkg["test_x#01"]` for a renamed duplicate) |
| `coveringTests` | array, optional | Tests that evaluate the mutated line |
| `selectedTests` | array, optional | Tests that reach the enclosing rule: the ones run when selection is on |
| `error` | string, optional | For `stillborn`: the first compiler error |
| `ignoreReason` | string, optional | For `ignored`: the reason given in the annotation or the configuration |
| `patchOk` | boolean | The patched source parses to a different AST (false for equivalent mutants, and for ignored ones, which are not run) |
| `durationMs` | number | Time spent on this mutant |

## Statuses

| Status | Meaning | Scored |
|---|---|---|
| `killed` | A test fails or errors | Yes, good |
| `timeout` | The tests took longer than the budget (`--timeout-factor`) | Yes, counts as killed |
| `survived` | Every test passes, and one of them evaluates the mutated line | Yes, a gap |
| `no_coverage` | Every test passes, and none evaluates the mutated line | Yes, a gap |
| `stillborn` | The mutant does not compile | No |
| `equivalent` | The patched source parses to the same policy as the original (for example a duplicate set element dropped); not run | No |
| `ignored` | An annotation or the configuration marks it as checked; not run | No |

## Other formats

- `--format sarif`: SARIF 2.1.0, one result per `survived` (warning) or `no_coverage` (note) mutant, with
  `partialFingerprints.regodrillId` set to the mutant `id`.
- `regodrill list --format json`: `{schemaVersion, mutants, warnings}`, where each mutant has the fields of
  the table above up to `rule`, plus `ignored` and `ignoreReason`.
- `regodrill operators --format json`: an array of `{id, summary, example, risk}`.
