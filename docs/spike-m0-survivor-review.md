# Spike M0: manual review of 50 survivors

Criterion 4 of the spike ([spec §14](https://github.com/adamaKomi/rego-syntax-support/blob/main/docs/mutation-testing-spec.md#14-spike-before-committing-to-the-mvp)):
at least 50 % of the surviving mutants should be real test gaps.

## Method

- Corpus: gatekeeper-library `a8d4799`, regodrill with sound test selection and the stillborn filter
  (`survived` + `no_coverage`: 369 mutants).
- Sample: 50 mutants drawn with Python's `random.sample`, seed `20261102`, from those 369, in file order.
- Each mutant is classified by reading the policy and its tests:
  - **Real gap**: the mutation is a plausible bug, and no test would notice it. A test could and should be
    added.
  - **Debatable**: the mutation changes behavior, but only in a way that the deployment already prevents
    (Gatekeeper's `match` filters the kind), or only the wording of a message.
  - **Equivalent**: no input that the Kubernetes API accepts changes the result. Most of these are type
    guards before a built-in that would fail anyway: OPA turns built-in errors into an undefined result, so
    removing the guard changes nothing. Some are not equivalent for arbitrary JSON (an empty array passes
    `count(x) == 0` where a string was expected), so skipping them automatically could hide real gaps in
    other policies; the M1 operator rules must be narrower than "never remove a type guard".
- First pass by the author's assistant; the verdicts marked as uncertain need a second look.

## Result

| Verdict | Count | Share |
|---|---|---|
| Real gap | 27 | 54 % |
| Debatable | 8 | 16 % |
| Equivalent | 15 | 30 % |

**Criterion met on the raw sample (54 % ≥ 50 %), but only just, and not once duplicates are removed:**

1. **Duplicates.** `lib_exempt_container.rego` is copied into about fifteen templates, so one gap counts
   several times. 10 of the 27 real gaps are 4 distinct mutations of that library, and 2 equivalent
   mutants are 1. Counting each distinct mutation once: **21 real gaps out of 43 (49 %)**. Grouping identical
   files in the report is an M1 improvement.
2. **Equivalent mutants follow clear patterns**, which operator rules can avoid:
   - removing a type guard (`is_string(x)`, `is_number(x)`) before a built-in that fails on the other type
     (5 of the 15);
   - removing a length guard (`count(s) > 1`), or moving its boundary, before `substring` with a computed
     offset (5 of the 15);
   - the other 5 are cases where another definition of the same rule or function already covers the input.

   Not generating the first two patterns would leave 21 real gaps out of 33 distinct mutants (64 %).

## Real gaps found

Grouped by kind; the numbers refer to the table below.

- **Exemptions never tested**: wildcard exemptions in `lib_exempt_container` (#6, #31, #37, #41, #47), a
  non-matching exemption (#4, #14, #17, #24, #40), and the `not is_exempt` check of several rules (#1, #35,
  #43, #48).
- **`ephemeralContainers` never tested**: `allowedreposv2` (#39), `selinux` (#19), `host-network-ports`
  (#27).
- **UPDATE requests never tested**: `host-probes-lifecycle` (#20, #22), `forbidden-sysctls` (#50). These
  templates skip updates on purpose, and no test checks it.
- **Boundaries**: memory limit equal to the maximum (#32), host port equal to the minimum (#38).
- **Other untested behavior**: a PDB that selects other pods (#3), the `docker/default` branch of seccomp
  (#10), AppArmor profile types other than `RuntimeDefault` (#15), a capability explicitly set to `false`
  (#29), the message of a deprecated API with no replacement (#23).

These are the candidates for upstream test PRs.

## Full table

| # | Template | Mutation | Status | Verdict | Reason |
|---|---|---|---|---|---|
| 1 | psp/host-probes-lifecycle | remove `not is_exempt(c)` | survived | Real gap | No test with an exempt container |
| 2 | general/containerlimits | remove `not is_number(orig)` | survived | Equivalent | A number then fails `get_suffix` (needs a string) |
| 3 | general/poddisruptionbudget | remove `count(matchLabels - labels) == 0` | survived | Real gap | No test with a PDB that selects other pods |
| 4 | psp/seccomp, lib | remove `exemption == img` | survived | Real gap | No test where an exemption doesn't match the image |
| 5 | psp/seccomp | remove `not contains(profile, "/")` | survived | Debatable | Adds canonical forms of already-allowed profiles; uncertain |
| 6 | general/containerresourceratios, lib | remove `endswith(exemption, "*")` | no_coverage | Real gap | Wildcard exemptions never tested |
| 7 | general/containerrequests | remove `count(mem) > 1` | survived | Equivalent | `substring` with a negative offset fails anyway |
| 8 | general/storageclass | remove `includeStorageClassesInMessage` | survived | Debatable | Changes the message wording only |
| 9 | general/containerlimits | `count(mem) > 0` → `>=` | survived | Equivalent | `substring` of an empty string fails anyway |
| 10 | psp/seccomp | remove `"docker/default" == ...` | no_coverage | Real gap | The `docker/default` branch is never tested |
| 11 | general/containerresourceratios | remove `is_number(orig)` | survived | Equivalent | `string * 1000` fails anyway |
| 12 | general/storageclass | remove `includeStorageClassesInMessage` | survived | Debatable | Message wording only |
| 13 | psp/fsgroup | remove `rule == "MayRunAs"` | survived | Debatable | `MustRunAs` checks the same ranges; uncertain |
| 14 | psp/read-only-root-filesystem, lib | remove `_matches_exemption(...)` | survived | Real gap | Any exemption list would exempt every container |
| 15 | psp/apparmor | remove `profile.type == "RuntimeDefault"` | survived | Real gap | Other profile types would be read as `runtime/default` |
| 16 | general/containerresourceratios | remove `count(mem) > 1` | survived | Equivalent | `substring` with a negative offset fails anyway |
| 17 | psp/proc-mount, lib | remove `_matches_exemption(...)` | survived | Real gap | Same as #14 |
| 18 | general/containerresourceratios | remove `count(mem) > 1` | survived | Equivalent | Same as #16 |
| 19 | psp/selinux | remove `has_field(..., "seLinuxOptions")` | no_coverage | Real gap | `ephemeralContainers` never tested |
| 20 | psp/host-probes-lifecycle | remove `not is_update(...)` | survived | Real gap | UPDATE requests never tested |
| 21 | general/storageclass | remove `allowedStorageClasses != null` | survived | Debatable | Message wording only |
| 22 | psp/host-probes-lifecycle | remove `not is_update(...)` | survived | Real gap | Same as #20, other rule |
| 23 | general/verifydeprecatedapi | remove `not match(targetAPI)` | no_coverage | Real gap | Message without replacement API never tested |
| 24 | psp/privileged-containers, lib | remove `exemption == img` | survived | Real gap | Same as #4 |
| 25 | general/horizontalpodautoscaler | remove `kind == "HorizontalPodAutoscaler"` | survived | Debatable | The constraint's `match` filters the kind |
| 26 | general/containerresourceratios | remove `is_string(mem)` | survived | Equivalent | `count` of a number fails anyway |
| 27 | psp/host-network-ports | remove `not is_exempt(c)` (ephemeral) | no_coverage | Real gap | `ephemeralContainers` never tested |
| 28 | psp/seccompv2 | remove `allow_profile.type == "Localhost"` | survived | Equivalent | Only `Localhost` entries have `localHostProfile` |
| 29 | psp/capabilities | remove `not obj[param] == false` | survived | Real gap | A value explicitly `false` is never tested |
| 30 | general/containerlimits | `count(mem) > 1` → `>=` | survived | Equivalent | Same as #16 |
| 31 | psp/seccomp, lib | remove `startswith(img, prefix)` | no_coverage | Real gap | Same as #6 |
| 32 | general/containerlimits | `mem > max_mem` → `>=` | survived | Real gap | No test with memory equal to the maximum |
| 33 | general/containerrequests | remove `not is_number(orig)` | survived | Equivalent | `endswith` of a number fails anyway |
| 34 | psp/seccomp | remove `startswith(profile, "localhost/")` | survived | Debatable | Exact matches are allowed by another definition; uncertain |
| 35 | general/containerlimits | remove `not is_exempt(container)` | survived | Real gap | No exempt container without resources in the tests |
| 36 | psp/host-filesystem | remove `allowedHostPath.readOnly` | survived | Equivalent | Another definition already allows non-read-only paths |
| 37 | psp/selinux, lib | remove `startswith(img, prefix)` | no_coverage | Real gap | Same as #6 |
| 38 | psp/host-network-ports | `hostPort < min` → `<=` | survived | Real gap | No test with a host port equal to the minimum |
| 39 | general/allowedreposv2 | remove `not image_matches(...)` (ephemeral) | no_coverage | Real gap | `ephemeralContainers` never tested |
| 40 | general/containerlimits, lib | remove `_matches_exemption(...)` | survived | Real gap | Same as #14 |
| 41 | psp/apparmor, lib | remove `exemption == img` | no_coverage | Real gap | Exact exemptions never tested in this template |
| 42 | general/noupdateserviceaccount | remove `obj.kind == "Pod"` | survived | Equivalent | Other kinds have no `spec.serviceAccountName` |
| 43 | general/containerlimits | remove `not is_exempt(container)` | survived | Real gap | Same as #35, CPU rule |
| 44 | general/uniqueserviceselector | remove `obj.apiVersion == ...` | survived | Debatable | Two services with the same name and different API versions; unlikely |
| 45 | psp/proc-mount, lib | remove `not endswith(exemption, "*")` | survived | Equivalent | A wildcard never equals an image name |
| 46 | general/containerrequests | remove `is_string(mem)` | survived | Equivalent | Same as #26 |
| 47 | psp/seccompv2, lib | remove `startswith(img, prefix)` | no_coverage | Real gap | Same as #6 |
| 48 | general/containerresourceratios | remove `not is_exempt(container)` | survived | Real gap | Same as #35 |
| 49 | psp/selinux, lib | remove `not endswith(exemption, "*")` | survived | Equivalent | Same as #45 |
| 50 | psp/forbidden-sysctls | remove `not is_update(...)` | survived | Real gap | UPDATE requests never tested |

`psp` is `pod-security-policy`; `lib` is the copied `lib_exempt_container.rego`.
