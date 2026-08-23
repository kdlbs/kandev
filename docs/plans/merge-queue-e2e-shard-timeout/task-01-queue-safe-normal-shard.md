---
id: "01-queue-safe-normal-shard"
title: "Give the normal E2E shard a queue-safe budget"
status: completed
depends_on: []
plan: plan.md
spec: ../../specs/e2e-duration-aware-sharding/spec.md
parallelism: sequential
---

# Task 01: Give the normal E2E shard a queue-safe budget

## Acceptance

- The normal `e2e` matrix job uses a 35-minute job timeout.
- The container, desktop, report, and individual Playwright test timeouts stay
  unchanged.
- The workflow contract test fails if the normal queue budget regresses to the
  old 25-minute cap.
- The aggregate gate continues to reject cancelled jobs.

## Files

- `.github/workflows/e2e-tests.yml`
- `.github/scripts/e2e-tests-workflow-contract_test.py`
- `docs/specs/e2e-duration-aware-sharding/spec.md`
- `docs/plans/merge-queue-e2e-shard-timeout/plan.md`

## TDD sequence

1. Add the contract assertion and run it to prove the old 25-minute workflow
   fails the new queue-budget requirement.
2. Change only the normal matrix timeout to 35 minutes.
3. Re-run the contract test and `git diff --check`.

## Operational verification

The merge-group run for the pushed PR must complete the normal shard matrix,
report merge, and `E2E Tests Passed` gate without a timeout removal event.

## Implementation result

- Changed only the normal `e2e` matrix job from 25 to 35 minutes.
- Added the queue-budget contract assertion.
- The RED contract run failed on the old timeout; the GREEN run passed all 5
  contract tests.
- `git diff --check` passed.
