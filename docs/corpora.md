# Real-world corpora

regodrill is checked on public Rego projects, beyond its own scenario catalog. For each corpus: what runs,
what doesn't and why, and the invariants that must hold everywhere:

- every patch changes the policy exactly as intended (`patchOk` for every mutant that compiles);
- the verdicts are the same with and without test selection (`--no-select`);
- no mutant that compiles is dropped by the stillborn filter (compared run by run when the filter changed).

Measured on 2026-11-03, regodrill at the state of the M1 work (ten operators), OPA 1.21.3.

## open-policy-agent/gatekeeper-library (`a8d4799`)

Rego v0 (`--v0-compatible`), 51 directories under `src/` with tests, 1,003 tests. One run per directory.

| Measure | Value |
|---|---|
| Mutants (ten operators) | 2,426 |
| Killed / survived / no coverage / stillborn | 1,688 / 487 / 206 / 45 (1.9 %) |
| Inexact patches | 0 |
| Verdicts that differ with `--no-select` | 0 |
| Duration, 8 workers per directory | 13.5 s (55 s for the mutation phase before the fast path, 8.8 s after) |

`QUANT_WEAKEN` and `DEFAULT_FLIP` produce nothing here: the library uses neither `every` nor boolean
defaults. Details of the spike: [spike-m0.md](spike-m0.md) and [spike-m0-survivor-review.md](spike-m0-survivor-review.md).

## aquasecurity/trivy-checks (`f9099b7`)

Rego v1, METADATA annotations, shared libraries. The project runs its tests with a custom OPA build that
adds built-ins (`result.new` in 558 files, `sh.parse_commands`, `cidr.*`, `squealer.scan_string`,
`isManaged`), so most checks cannot run with standard OPA, and neither can regodrill. The baseline stops
with exit code 2 and a hint that names the missing built-ins.

`lib/kubernetes` uses none of them:

| Measure | Value |
|---|---|
| Mutants | 72 (after the stillborn filter; 105 before) |
| Killed / survived / no coverage / stillborn | 26 / 7 / 39 / 0 |
| Inexact patches | 0 |

This corpus found the gaps of the first stillborn filter: unifications used as declarations
(`pod = object`), iteration (`pods[pod]`), and `not` on expressions that bind a variable. The filter now
covers them, and dropped only stillborn mutants on both corpora.

## open-policy-agent/conftest examples (`4005c5d`)

Ten directories under `examples/` contain tests. Conftest examples are written for conftest, not for
`opa test` alone:

| Directory | Result | Why |
|---|---|---|
| `dotenv/policy` | Runs: 10 mutants, score 100 % | |
| `kubernetes/policy` | Runs: 39 mutants, score 56.4 % | |
| `hcl2/policy`, `jenkins/policy`, `nginx/policy` | Exit 2, hint | The tests call conftest built-ins (`parse_config`, `parse_config_file`) |
| `kubernetes/combine` | Exit 2, hint | Kubernetes manifests next to the policy load as conflicting data: `--ignore '*.yaml'` |
| `data/policy` | Exit 2 | The policy imports `data.services` from `data/exclusions`, which conftest passes with `--data`: give both paths |
| `report/fail`, `report/success` | Exit 2 | The tests use a `deny` rule defined elsewhere, on purpose (they demonstrate conftest's reports) |
| `traefik/policy` | Exit 2 | A test fails on the unmodified policy; not investigated |

In every case regodrill stops before mutating, with exit code 2 and the reason. Comparing each directory
with `opa test` itself is still to do.
