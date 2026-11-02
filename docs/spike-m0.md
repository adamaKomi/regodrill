# Spike M0: first measures

Date: 2026-11-02. Specification: §14 of
[mutation-testing-spec.md](https://github.com/adamaKomi/rego-syntax-support/blob/main/docs/mutation-testing-spec.md#14-spike-before-committing-to-the-mvp).

## Setup

- regodrill at the state of this commit, OPA library v1.21.3, Go 1.27.2, Linux x86-64 laptop (8 CPUs).
- Corpus: gatekeeper-library `a8d4799` (2026-10-20), every directory under `src/` that contains a
  `*_test.rego` file: 51 directories, 1,003 tests. The library is Rego v0, so every run uses `--v0-compatible`.
- One `regodrill run --parallel 4 --format json` per directory, then the same with `--no-select`.
- No `default` boolean rules exist in this corpus, so `DEFAULT_FLIP` produced no mutant.

## Results

| Measure | Value |
|---|---|
| Mutants | 1,300 (EXPR_REMOVE 1,251, COMP_BOUNDARY 49) |
| Stillborn (don't compile) | 589 (45 %), all EXPR_REMOVE |
| Killed / survived / no coverage | 328 / 322 / 61 |
| Score (with test selection) | 46.1 % |
| Patched source parses and differs from the original AST | 1,300 / 1,300 (100 %) |
| Total wall time, 51 directories, with selection | 29.3 s |
| Same, without selection | 36.9 s |
| Mutation phase only, summed: with / without selection | 15.7 s / 23.3 s (1.5×) |
| Baseline + per-test coverage, summed | 12.9 s |

### Second measure: sound selection and single-pass coverage

Same corpus and flags, after the fixes for findings 1 and 2 (see "Fixes" below).

| Measure | Value |
|---|---|
| Killed / survived / no coverage | 342 / 310 / 59, **identical with and without selection** |
| Score | 48.2 % |
| Total wall time, with / without selection | 24.2 s / 29.9 s |
| Mutation phase only, summed: with / without selection | 21.2 s / 27.0 s (1.27×) |
| Baseline with coverage, summed | 2.3 s (was 12.9 s) |

The single-pass recorder gives the same covering and selected tests as running each test alone: 0 differences
over the 1,300 mutants.

## Against the go criteria

| Criterion | Target | Result | Verdict |
|---|---|---|---|
| Exact text patches | ≥ 99 % | 100 % | Met |
| Duration on the full library | ≤ 10 min | 24 s | Met |
| Coverage-based selection | ≥ 3× faster | 1.27× on the mutation phase, now sound; coverage costs 2.3 s | Not met on this corpus |
| Useful signal | ≥ 50 % real gaps among survivors | Not measured yet: manual review pending | Open |
| WASM build | Recorded | Not tried yet | Open |

## Findings

1. **Per-test coverage is the main cost.** Each test runs once more, alone, with a coverage tracer. On small
   suites like gatekeeper's (about 20 tests per directory, a few ms each), this costs more than the selection
   saves. Options: one coverage run with a tracer that records the current test, or skipping selection when
   the suite is small.
2. **Line-level selection is not sound.** Without selection, 342 mutants are killed; with it, 328. So 14
   mutants (1.1 %) were killed by a test that does not cover the mutated line. Two causes, both fixed:
   - **Duplicate test names.** The test runner renames a second `test_x` in the same package `test_x#01`, and
     its filter matches `data.pkg["test_x#01"]`. regodrill built `data.pkg.test_x#01`, so these tests had no
     coverage and were never selected.
   - **Rule indexing.** For an input that doesn't match an indexed expression (`input.review.kind.kind ==
     "Service"`, `input.review.operation == "UPDATE"`), OPA skips the rule body entirely: the line is not
     covered, yet removing the expression makes the rule match and the test fails. Line coverage cannot see
     this, so neither can the spec's §6 step 4 ("a mutant on a line that no test covers is `no_coverage`").
3. **45 % of mutants are stillborn.** Removing an assignment (`msg := sprintf(...)`, `container := ...[_]`)
   leaves a variable undefined. Each costs a compilation. Fix to try: don't remove an expression that binds
   a variable used later in the body.
4. **Real gaps appear.** `general/allowedrepos` and `allowedreposv2`: the `ephemeralContainers` rule is
   covered by no test, so a broken check there would never be noticed. (The first measure also listed
   `initContainers`: that was the duplicate-name bug of finding 2. Its tests are named like the container
   tests.) The shared `lib_exempt_container.rego`, copied into about fifteen templates, never tests a
   wildcard exemption (`endswith(exemption, "*")`, `startswith(img, prefix)`).
5. **Some survivors are debatable.** Many are type guards (`is_string(mem)`, `is_number(orig)`) or kind
   filters (`input.review.kind.kind == "Service"`) that Gatekeeper's constraint `match` already enforces in
   production. They need the manual review of criterion 4, and probably operator rules or ignore
   annotations.

## Fixes

- **Test names** are built like the runner's filter (`ast.Ref.Extend` turns the name into a string term).
- **Selection by rule reach.** The baseline records, for each test, the documents it looks up (the `Index`
  trace event, emitted even when the index matches no rule). A mutant runs the tests that look up its
  enclosing rule, a document containing it (`data.pkg`), or a document inside it. Line coverage is still
  recorded, for reporting only.
- **`no_coverage`** now means: no test evaluates the mutated line, and no test kills the mutant. It is the
  same with and without selection. A mutant that no test reaches is classified without running, as before.
- **Single-pass coverage.** The baseline is one run with a tracer that starts a new record at each test's
  root query (parallelism 1, so tests don't interleave). No more per-test runs, and no `*cover.Cover`, which
  made the runner add two supplementary passes per test (indexing and early exit disabled).
- Regression tests: `internal/engine/engine_test.go` with `testdata/selection` (a duplicate test name and an
  indexed rule).

Spec to update: §6 steps 2, 4 and 5 (selection by rule reach, `no_coverage` definition).

## Next steps

1. Manual review of a random sample of 50 survivors: real gap, debatable or equivalent (criterion 4).
2. Skip predictable stillborn mutants (finding 3).
3. Try the WASM build (`GOOS=wasip1`) and record size and speed.
4. Unit tests for each operator: one killed and one surviving example.
