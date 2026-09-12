---
id: "04-partition-frontend-verification"
title: "Partition frontend verification"
status: in_progress
wave: 4
depends_on: ["03-reduce-test-setup"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-CI-PERFORMANCE-003
  - REQ-PLATFORM-CI-PERFORMANCE-004
acceptance_criteria:
  - AC-PLATFORM-CI-PERFORMANCE-003.1
  - AC-PLATFORM-CI-PERFORMANCE-003.2
  - AC-PLATFORM-CI-PERFORMANCE-003.3
  - AC-PLATFORM-CI-PERFORMANCE-004.1
  - AC-PLATFORM-CI-PERFORMANCE-004.2
system_design:
  - ../../specs/platform/system-design/ci-performance.md
---

# Task 04: Partition frontend verification

## Summary

Benchmark two and four unit-test shards after setup optimization.
Adopt the smallest passing candidate and preserve the stable required frontend gate.

## In scope

- Retain static checks/build in `frontend`; add a measured `frontend_tests` matrix.
- Existing runner-plan family wiring, unique shard reports, and complete gate dependencies.
- Compare test identities, median critical path, queue delay, retries, and runner minutes.

## Out of scope

- E2E shard changes, protected runner changes, paid-capacity activation, skipped checks, or trigger-level path filters.

## Acceptance

- Shard reports form an exact partition of the unsharded test selection, with unchanged pretest and production-environment guards.
- The gate blocks failures, cancellations, and failed detection; deliberate skips and all-pass runs succeed.
- Adopt sharding only with at least 30% lower median frontend execution critical path and at most 25% extra runner minutes. Otherwise retain unsharded CI and record the rejection.

## Verification

Run commands from the repository root. Install workspace dependencies first in a fresh worktree.

```bash
python3 .github/scripts/frontend-tests-workflow-contract_test.py
python3 .github/scripts/external-runner-workflow-contract_test.py
python3 .github/scripts/runner-plan_test.py
python3 .github/scripts/lint-action-pinning_test.py
actionlint .github/workflows/frontend-tests.yml
(cd apps/web && pnpm test --shard=1/2 --reporter=json --outputFile=/tmp/kandev-ci-shard-1.json)
(cd apps/web && pnpm test --shard=2/2 --reporter=json --outputFile=/tmp/kandev-ci-shard-2.json)
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

Task 03.

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

Implemented 2026-09-12. The static frontend job keeps lint, typecheck, i18n
checks, ratchets, and build. A two-instance `frontend_tests` matrix runs the
unit suite with the production environment, unique JSON reports, and the
existing external-runner planner. `Frontend Tests Passed` now requires the
runner plan, change detection, static job, and every unit-test matrix result;
failures and cancellations fail the gate while deliberate skips pass.

Local partition evidence, using the corrected `pnpm test -- --shard=...`
invocation, is:

- shard 1/2: 1,000 files and 8,225 passed tests;
- shard 2/2: 999 files, 8,954 passed tests, and 4 pending tests;
- union: all 1,999 files and 17,136 assertion identities from the unsharded
  report, with zero overlap and no missing or unexpected identities;
- workflow, runner-planner, action-pinning, and frontend contract tests passed.

The four-shard candidate and three comparable hosted runs were not executed.
The 30% critical-path and 25% runner-minute adoption decision therefore stays
open; the two-shard workflow is recorded as a candidate pending hosted proof.
