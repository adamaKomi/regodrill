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

## Against the go criteria

| Criterion | Target | Result | Verdict |
|---|---|---|---|
| Exact text patches | ≥ 99 % | 100 % | Met |
| Duration on the full library | ≤ 10 min | 30 s | Met |
| Coverage-based selection | ≥ 3× faster | 1.5× on the mutation phase, and coverage costs 12.9 s | Not met on this corpus |
| Useful signal | ≥ 50 % real gaps among survivors | Not measured yet: manual review pending | Open |
| WASM build | Recorded | Not tried yet | Open |

## Findings

1. **Per-test coverage is the main cost.** Each test runs once more, alone, with a coverage tracer. On small
   suites like gatekeeper's (about 20 tests per directory, a few ms each), this costs more than the selection
   saves. Options: one coverage run with a tracer that records the current test, or skipping selection when
   the suite is small.
2. **Line-level selection is not sound.** Without selection, 342 mutants are killed; with it, 328. So 14
   mutants (1.1 %) were killed by a test that does not cover the mutated line. Likely cause: removing an
   expression changes which other rules evaluate. Fix to try: select the tests that cover any line of the
   enclosing rule, not only the mutated line.
3. **45 % of mutants are stillborn.** Removing an assignment (`msg := sprintf(...)`, `container := ...[_]`)
   leaves a variable undefined. Each costs a compilation. Fix to try: don't remove an expression that binds
   a variable used later in the body.
4. **Real gaps appear.** `general/allowedrepos`: the `initContainers` and `ephemeralContainers` rules are
   covered by no test (`no_coverage`), so a broken check there would never be noticed. The same pattern
   shows up in `allowedreposv2`.
5. **Some survivors are debatable.** Many are type guards (`is_string(mem)`, `is_number(orig)`) or kind
   filters (`input.review.kind.kind == "Service"`) that Gatekeeper's constraint `match` already enforces in
   production. They need the manual review of criterion 4, and probably operator rules or ignore
   annotations.

## Next steps

1. Manual review of a random sample of 50 survivors: real gap, debatable or equivalent (criterion 4).
2. Fix the soundness of test selection (finding 2), then measure again.
3. Single-pass coverage (finding 1), then measure again.
4. Skip predictable stillborn mutants (finding 3).
5. Try the WASM build (`GOOS=wasip1`) and record size and speed.
6. Unit tests for each operator: one killed and one surviving example.
