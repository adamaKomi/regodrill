# Changelog

All notable changes to regodrill. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and versions follow [Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.3.0] - 2026-11-03

### Added

- `regodrill check`: parses and compiles Rego files like `opa check` of the same OPA version, with the same
  output for `--format pretty|json`, `--v0-compatible`, `--max-errors` and `--ignore`. Editors can check Rego
  with the regodrill binary alone. CI compares both commands byte for byte on `testdata/check`.

### Fixed

- Every release asset has a build provenance attestation, the WebAssembly build included: 0.1.0 and 0.2.0
  attest only the native archives and `checksums.txt`.

## [0.2.0] - 2026-11-03

### Added

- `ruleLine` in the JSON report: the line where the mutant's rule definition starts, so editors can show a
  summary per definition.

### Changed

- Release notes start from GitHub's generated notes (pull request titles) instead of a list of commits.

### Fixed

- One-line `else` clauses without braces (`else := 2 if input.c >= 4`) are mutated like other bodies. OPA
  gives such a body the location of the whole clause, so their comparisons and constants were skipped, and
  `NOT_TOGGLE` produced a stillborn mutant there.
- `regodrill version` reports the module version when installed with `go install` (0.1.0 printed
  `0.1.0-dev`).

## [0.1.0] - 2026-11-03

First version.

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

[Unreleased]: https://github.com/adamaKomi/regodrill/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/adamaKomi/regodrill/releases/tag/v0.3.0
[0.2.0]: https://github.com/adamaKomi/regodrill/releases/tag/v0.2.0
[0.1.0]: https://github.com/adamaKomi/regodrill/releases/tag/v0.1.0
