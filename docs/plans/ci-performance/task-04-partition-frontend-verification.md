---
id: "04-partition-frontend-verification"
title: "Partition frontend verification"
status: in_progress
wave: 5
depends_on: ["03-reduce-test-setup", "08-reduce-scheduling-stages"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-CI-PERFORMANCE-003
  - REQ-PLATFORM-CI-PERFORMANCE-004
  - REQ-PLATFORM-CI-PERFORMANCE-005
acceptance_criteria:
  - AC-PLATFORM-CI-PERFORMANCE-003.1
  - AC-PLATFORM-CI-PERFORMANCE-003.2
  - AC-PLATFORM-CI-PERFORMANCE-003.3
  - AC-PLATFORM-CI-PERFORMANCE-004.1
  - AC-PLATFORM-CI-PERFORMANCE-004.2
  - AC-PLATFORM-CI-PERFORMANCE-005.3
system_design:
  - ../../specs/platform/system-design/ci-performance.md
---

# Task 04: Partition frontend verification

## Summary

Benchmark two and four unit-test shards after setup optimization.
Adopt the smallest passing candidate and preserve the stable required frontend gate.

## In scope

- Retain static checks, unsharded tests, and build in `frontend` until the
  adoption gate passes; keep the measured `frontend_tests` matrix as a candidate.
- Existing runner-plan family schema, unique candidate shard reports, and the
  stable required gate.
- Compare test identities, median critical path, queue delay, retries, and runner minutes.

## Out of scope

- E2E shard changes, protected runner changes, paid-capacity activation, skipped checks, or trigger-level path filters.

## Acceptance

- Shard reports form an exact partition of the unsharded test selection, with unchanged pretest and production-environment guards.
- The gate blocks failures, cancellations, and failed detection; deliberate skips and all-pass runs succeed.
- Adopt sharding only with at least 30% lower median frontend execution critical path and at most 10% extra runner minutes. Otherwise retain unsharded CI and record the rejection.

## Verification

Run commands from the repository root. Install workspace dependencies first in a fresh worktree.

```bash
python3 .github/scripts/frontend-tests-workflow-contract_test.py
python3 .github/scripts/external-runner-workflow-contract_test.py
python3 .github/scripts/runner-plan_test.py
python3 .github/scripts/lint-action-pinning_test.py
actionlint .github/workflows/frontend-tests.yml
(cd apps/web && pnpm run test --shard=1/2 --reporter=json --outputFile=/tmp/kandev-ci-shard-1.json)
(cd apps/web && pnpm run test --shard=2/2 --reporter=json --outputFile=/tmp/kandev-ci-shard-2.json)
git diff --check
```

Run the same commands with shard indexes 1 through 4 for the four-shard candidate.
Compare JSON file/test identities against the unsharded Task 03 report at the same snapshot.
Use three comparable hosted runs per candidate for the performance decision; record all failed attempts too.
Do not mark the performance criterion complete from local serial runs.

## Files likely touched

- `.github/workflows/frontend-tests.yml`
- `.github/scripts/frontend-tests-workflow-contract_test.py`
- `.github/scripts/external-runner-workflow-contract_test.py`
- `docs/specs/platform/system-design/external-e2e-runner-capacity.md (job inventory only)`

## Dependencies

Tasks 03 and 08. Use the optimized setup and consolidated bootstrap.

## Risks

Serial local shard commands prove selection only. Hosted comparable samples must establish speed and cost before adoption.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/platform/requirements/ci-performance.md), acceptance IDs in frontmatter.
- [System design](../../specs/platform/system-design/ci-performance.md), corresponding implementation boundary.
- [Plan](plan.md), baseline and companion-package status.
- Existing workflow contract tests under `.github/scripts/`.

## Results

Historical state recorded 2026-09-12. Hosted adoption remains incomplete. The frontend job keeps lint, typecheck, i18n checks,
ratchets, the unsharded unit suite, and build. The two-instance
`frontend_tests` matrix remains a measured candidate and is not enabled until
the hosted adoption gate passes. `Frontend Tests Passed` continues to require
the runner plan, change detection, and the single frontend job; failures and
cancellations fail the gate while deliberate skips pass.

Local partition evidence, using the corrected `pnpm test -- --shard=...`
invocation, is:

- shard 1/2: 1,000 files and 8,225 passed tests;
- shard 2/2: 999 files, 8,954 passed tests, and 4 pending tests;
- union: all 1,999 files and 17,136 assertion identities from the unsharded
  report, with zero overlap and no missing or unexpected identities;
- workflow, runner-planner, action-pinning, and frontend contract tests passed.

The four-shard candidate and three comparable hosted runs were not executed.
The 30% critical-path and 10% runner-minute adoption decision therefore stays
open; the two-shard workflow remains a candidate pending hosted proof.

## October continuation

Compare one, two, and four partitions only after Task 03 reduces setup work.
Use the optimized unsharded suite as the baseline for both critical path and runner-minute comparisons.
The 10% allowance replaces the earlier 25% allowance because the existing fleet is saturated.
Adopt the smallest passing candidate; retain one partition if none passes.

The current source has one frontend verification job. There is no active `frontend_tests` matrix.
Earlier two-shard counts above are local historical evidence, not an implemented workflow.
After adoption, keep static checks and build in `frontend`, with native Vitest file shards alongside it.
Use the package script so pretest generation runs. Preserve all existing static, SDK, i18n, and ratchet checks.
Update the required gate and reports to fail on any missing, failed, or cancelled required partition.

Normalize report identities by repository-relative file and full title, retaining duplicate multiplicity and outcome.
Exclude the project label from identity when comparing environment migrations; retain it as provenance.
Record parameterized cases, skips, and dynamic test expansion explicitly. Equal totals alone do not prove parity.
The sequence of hosted runs must be bounded; do not saturate CI with simultaneous baseline and candidate suites.

### October rollout decision

Retain the single frontend job in this change. Setup reduction and four fewer
scheduling jobs improve occupied-capacity use without adding parallel demand.
The available evidence does not establish the required hosted two/four-shard
critical-path gain or the 10% occupancy limit. No shard matrix is enabled.
This is a conservative deferral, not a measured rejection of sharding; the
work order remains in progress until comparable hosted evidence exists.
