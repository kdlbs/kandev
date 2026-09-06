---
id: "03-deterministic-percentage-rollout"
title: "Add deterministic percentage rollout"
status: done
wave: 2
depends_on:
  - "02-extend-tiered-runners-to-linux-ci"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-002
  - REQ-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-003
acceptance_criteria:
  - AC-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-002.1
  - AC-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-002.2
  - AC-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-003.1
  - AC-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-003.2
  - AC-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-003.3
  - AC-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-003.4
  - AC-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-003.5
  - AC-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-003.6
system_design:
  - ../../specs/platform/system-design/external-e2e-runner-capacity.md
---

# Task 03: Add Deterministic Percentage Rollout

## Summary

Add a read-only planner that assigns a deterministic percentage of eligible job
instances to external tiers. Use exact floor allocation for matrix families,
stable hash cohorts for singleton jobs, and fail-closed validation for
operator input.

## In scope

- Add `.github/scripts/runner-plan.py` and focused unit tests.
- Add planner jobs and outputs to the eligible workflows without changing
  their test graph or protected-job boundaries.
- Feed validated runner assignments to matrix and singleton jobs through
  `needs` outputs and `fromJSON` where required.
- Document `KANDEV_CI_EXTERNAL_PERCENT`, 0/50/100 behavior, warnings, and
  rerun stability in the merge-queue runbook.
- Extend workflow contract tests for planner wiring and protected jobs.

## Out of scope

- Automatic activation, adaptive or weighted allocation, or same-run retry.
- Exact compute-share balancing across unrelated workflows.
- Any protected host-Docker, Kind, Kubernetes, service, Windows, release,
  deployment, publishing, signing, or credential-bearing job.

## Acceptance

- Burst off or percentage `0` assigns all eligible jobs to `ubuntu-latest`.
- Percentage `100` assigns all eligible jobs with non-empty tier labels to the
  configured tier; empty labels still use `ubuntu-latest`.
- At 50 percent, a fourteen-instance E2E matrix receives seven external
  assignments and a two-instance backend matrix receives one.
- A rerun of the same workflow run receives the same assignments. Singleton
  jobs approach the configured percentage across different runs.
- Missing input means `0`. Malformed or out-of-range input emits a visible
  warning and assigns all eligible jobs to `ubuntu-latest`.
- Protected jobs never consume planner output and remain on GitHub-hosted
  runners.

## Verification

```bash
python3 .github/scripts/runner-plan_test.py
python3 .github/scripts/e2e-tests-workflow-contract_test.py
python3 .github/scripts/frontend-tests-workflow-contract_test.py
python3 .github/scripts/lint-action-pinning_test.py
python3 .github/scripts/lint-action-pinning.py
python3 scripts/lint-spec-files.py --all
actionlint .github/workflows/e2e-tests.yml .github/workflows/backend-tests.yml .github/workflows/frontend-tests.yml .github/workflows/architecture-lint.yml .github/workflows/lint-action-pinning.yml .github/workflows/lint-harness-files.yml
zizmor .github/workflows/e2e-tests.yml .github/workflows/backend-tests.yml .github/workflows/frontend-tests.yml .github/workflows/architecture-lint.yml .github/workflows/lint-action-pinning.yml .github/workflows/lint-harness-files.yml
git diff --check
```

## Files likely touched

- `.github/scripts/runner-plan.py`
- `.github/scripts/runner-plan_test.py`
- Eligible workflow files and their contract tests.
- `docs/ci-merge-queue.md`

## Dependencies

Task 02 must be complete so all eligible job classes use the same tier
contract before planner outputs are introduced.

## Risks

- The planner adds a small GitHub-hosted dependency before eligible jobs start.
- A stable singleton cohort is an approximation across runs, not an exact
  per-run percentage.
- Workflow outputs must be constrained to approved labels so operator input
  cannot inject an arbitrary runner expression.

## Parallelism

`sequential`

## Results

- Added `.github/scripts/runner-plan.py` with deterministic matrix and
  singleton allocation.
- Added planner tests for unset, 0, 50, 100, empty-label, rerun, and invalid
  percentage behavior.
- Added planner jobs and enum-safe outputs to all eligible workflows.
- Updated the merge-queue runbook with percentage operation and rollback.
- Planner, workflow contract, action-pinning, actionlint, specification, and
  diff checks passed.
