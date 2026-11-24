---
id: "05-measure-adoption"
title: "Measure adoption and rollback"
status: in-progress
wave: 5
depends_on: ["04-route-native-checks"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-CI-PERFORMANCE-004
  - REQ-PLATFORM-CI-PERFORMANCE-005
  - REQ-PLATFORM-CI-PERFORMANCE-011
acceptance_criteria:
  - AC-PLATFORM-CI-PERFORMANCE-004.1
  - AC-PLATFORM-CI-PERFORMANCE-004.2
  - AC-PLATFORM-CI-PERFORMANCE-005.1
  - AC-PLATFORM-CI-PERFORMANCE-005.5
  - AC-PLATFORM-CI-PERFORMANCE-005.6
  - AC-PLATFORM-CI-PERFORMANCE-011.2
  - AC-PLATFORM-CI-PERFORMANCE-011.3
  - AC-PLATFORM-CI-PERFORMANCE-011.4
  - AC-PLATFORM-CI-PERFORMANCE-011.5
system_design:
  - ../../specs/platform/system-design/ci-performance.md
  - ../../specs/platform/system-design/web-native-verification.md
---

# Task 05: Measure adoption and rollback

## Summary

Accept the selected toolchain only after complete passing comparisons meet the execution targets.
Record local and hosted evidence separately, and preserve a working rollback path.

## In scope

- Capture three alternating successful baseline/candidate pairs after compatibility repairs.
- Hold application source, assertions, input inventories, runner class, CPU budget, and declared cache class constant.
- Measure complete package commands, including generation and plugin startup.
- Record dependency installation, cache restore, optional reference installation, and verification costs separately.
- Retain every attempted sample, including failures, cancellations, and incomplete reports.
- Require at least 50% lower median lint execution and 75% lower median typecheck execution.
- Require lower combined occupied frontend execution and no lost verification or resource regression.
- Collect three comparable hosted pairs through natural runs or separately authorized measurements.
- Use `.github/scripts/ci-efficiency.py` for attempt-specific workflow intervals and occupied runner time.
- Bind any dependency graph to the exact workflow revision. Leave unsupported metrics unknown.
- Rehearse restoration of prior pins and routing in an owned temporary project.
- Record adopted, rejected, or pending results without completing unrelated CI packages.
- Record acceptance of requirements 009–011 here. Do not promote the shared requirement document while its unrelated draft contracts remain unsettled.

## Out of scope

- New runners, more jobs, queue targets, billing changes, dispatches, commits, pushes, or PR operations without their separate authorization.
- Performance claims from failed candidate runs or changed source selections.
- Automatic replacement with the hybrid linter or ordinary tsrs mode.
- Generic QA, broad repository verification, or unrelated test remediation.

## Acceptance

1. Complete passing local pairs meet both execution bounds with equal inventories and required diagnostics. Evidence includes all attempts and costs.
2. Three comparable hosted pairs establish lower occupied frontend execution without lost checks, changed topology, or a resource regression.
3. Rollback restores the previous passing verification commands and dependency pins. Any unmet bound leaves adoption pending or records a rejection.

## Verification

Task 01 implements the benchmark operation. It preserves the original commands in isolated comparison projects.

```bash
python3 .github/scripts/web-toolchain-evidence_test.py
python3 .github/scripts/ci-efficiency_test.py
python3 .github/scripts/web-toolchain-evidence.py compare-types --web-root apps/web --mode fast
python3 .github/scripts/web-toolchain-evidence.py compare-lint --web-root apps/web --config apps/web/oxlint.config.mjs
python3 .github/scripts/web-toolchain-evidence.py benchmark --web-root apps/web --pairs 3 --output /tmp/kandev-native-web-adoption
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

For each actual hosted attempt, set its observed identifiers before this separate collection block.
The collection command does not dispatch a workflow.

```bash
: "${CI_RUN_ID:?Set an actual hosted run ID}"
: "${CI_RUN_ATTEMPT:?Set its observed attempt}"
python3 .github/scripts/ci-efficiency.py \
  --run-id "$CI_RUN_ID" --attempt "$CI_RUN_ATTEMPT" \
  --output "/tmp/kandev-native-web-${CI_RUN_ID}-${CI_RUN_ATTEMPT}.json"
```

Use the helper's `--graph` argument only with a graph validated against that attempt's workflow identity.
Unavailable scheduling attribution remains unknown.
Record raw reports outside version control and add curated results to this package.
The work order remains incomplete without its hosted acceptance evidence.

## Files likely touched

- `docs/plans/native-web-verification/evidence.md` and curated adoption data.
- This plan and its work-order Results sections.
- The native design lifecycle field after all criteria pass. The shared CI requirement lifecycle retains its independent scope.
- Related tool-host designs and decisions only to reconcile actual accepted behavior.

## Dependencies

Tasks 01–04.
Read both designs' measurement and rollback sections.
The earlier CI cache, browser-cache, and fifteen-minute targets retain their independent statuses.

## Risks

- Different compatibility repairs can invalidate a source-equivalent baseline.
- A lower compiler-only number can conceal installation or plugin startup costs.
- Shared-host load can distort small samples.
- Natural hosted runs can lack enough comparable attempts. Missing evidence cannot establish acceptance.

## Parallelism

`sequential`

## Results

Local acceptance completed on November 3, 2026. Hosted acceptance is pending, so
this work order remains open.

### Local acceptance

Three alternating complete-command pairs passed on identical repaired source.
Each compiler sample started from a cold incremental cache; the helper archived
the build info outside the working tree before every side.

| Measurement | Baseline median | Candidate median | Reduction |
| --- | ---: | ---: | ---: |
| Full lint | 88.592 s | 30.461 s | 65.62% |
| Complete typecheck | 110.246 s | 6.103 s | 94.46% |
| Combined occupied execution | 198.838 s | 36.564 s | 81.61% |

Both execution bounds pass. Peak process RSS was 5.0-5.4 GiB for the workspace
compiler and 1.5 GiB for fast tsrs; the scope is each direct child process, not
aggregate job memory. Shared-host load produced wide absolute variance, so this
is local evidence only.

### Rollback rehearsal

An owned temporary web project restored the previous `package.json` scripts and
the retained ESLint configuration. The prior `eslint --max-warnings 0` command
rejected a `debugger` fixture with `no-debugger`, TypeScript 5.9.3 remained
available, and the restored `tsc --noEmit` recipe matched the previous
`typecheck` script. No dependency, configuration, or source repair was deleted.

### Hosted evidence

No workflow was dispatched. Three comparable hosted pairs with lower occupied
frontend execution, unchanged topology, and no resource regression are required
before adoption can be claimed. Until they exist, Task 05 and requirements
009-011 remain pending, and the prior toolchain remains a tested rollback path.
