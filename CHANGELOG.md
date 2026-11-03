# Changelog

All notable changes to regodrill. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and versions follow [Semantic Versioning](https://semver.org/).

## [Unreleased]

First version. Not released yet.

### Added

- `regodrill run`: mutation testing of Rego policies with their own tests, loaded like `opa test` does
  (`.rego` files, `data.json` and `data.yaml`, `--ignore`). Rego v1 by default, `--v0-compatible` for v0.
- Ten mutation operators: `COMP_BOUNDARY`, `COMP_NEGATE`, `EXPR_REMOVE`, `NOT_TOGGLE`, `DEFAULT_FLIP`,
  `RULE_REMOVE`, `CONST_STRING`, `CONST_NUMBER`, `COLLECTION_DROP`, `QUANT_WEAKEN`. Mutants are text patches
  on the original source, so each one is exactly one visible change.
- Statuses: killed, timeout, survived, no coverage, stillborn, equivalent, ignored. Mutants that a static
  check shows would not compile are not generated; mutants equal to the original policy are not run.
- Test selection: each mutant runs the tests that look up its rule, with the same verdicts as running every
  test (`--no-select`).
- Reports: text (colored on a terminal, `NO_COLOR` respected), JSON (documented in
  [docs/report-format.md](docs/report-format.md)), SARIF 2.1.0 for GitHub code scanning; `--output`.
- `# regodrill:ignore` and `# regodrill:ignore-next-line` annotations, with operator IDs and a reason.
- `.regodrill.yaml`: paths, exclusions, threshold, operators, ignores by line or by pattern, loading and
  run settings. Unknown keys are errors.
- `--threshold` (exit code 3), `--since <git ref>` for the lines changed in a pull request.
- `regodrill list`, `regodrill operators`, `regodrill version`.
- Safety: `http.send` and `net.lookup_ip_addr` are off unless `--allow-network`; the baseline runs twice
  and stops on unstable tests; a mutant that runs too long is a timeout. Hints for Rego v0 without
  `--v0-compatible`, custom built-ins, and data files that conflict.
