---
id: "01-add-opt-in-e2e-runner-selection"
title: "Add opt-in E2E runner selection"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-001
  - REQ-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-002
acceptance_criteria:
  - AC-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-001.1
  - AC-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-001.2
  - AC-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-001.3
  - AC-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-001.4
  - AC-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-001.5
  - AC-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-001.6
  - AC-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-002.1
  - AC-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-002.2
  - AC-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-002.3
system_design:
  - ../../specs/platform/system-design/external-e2e-runner-capacity.md
---

# Task 01: Add Opt-in E2E Runner Selection

## Summary

Add two runner tiers and one burst-mode switch to the five eligible E2E jobs.
Protect the placement boundary with a workflow contract test. Document the
Ubicloud configuration, activation, and rollback procedure.

## In scope

- Update two light and three standard `runs-on` values in the E2E workflow.
- Assert eligible and protected job placement in the E2E workflow contract
  test.
- Add external-runner setup, rollback, security, and measurement guidance to
  the merge-queue runbook.

## Out of scope

- Changing live GitHub or Ubicloud settings.
- Moving any protected or credential-bearing job.
- Changing workflow behavior beyond runner placement.
- Adding a heavy runner tier.

## Acceptance

- When burst mode is inactive, all five eligible jobs use `ubuntu-latest`.
- When burst mode is active, two jobs use the light tier and three use the
  standard tier. An empty tier label uses `ubuntu-latest`.
- The four protected E2E jobs remain explicitly on `ubuntu-latest`, and the
  workflow adds no secret or permission.
- Contract tests and operator guidance make the initial placement, rollback,
  and pilot measurements explicit.

## Verification

```bash
python3 .github/scripts/e2e-tests-workflow-contract_test.py
python3 .github/scripts/lint-action-pinning_test.py
python3 .github/scripts/lint-action-pinning.py
actionlint .github/workflows/e2e-tests.yml
zizmor .github/workflows/e2e-tests.yml
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `.github/workflows/e2e-tests.yml`
- `.github/scripts/e2e-tests-workflow-contract_test.py`
- `docs/ci-merge-queue.md`
- `docs/specs/platform/requirements/external-e2e-runner-capacity.md`
- `docs/specs/platform/system-design/external-e2e-runner-capacity.md`
- `docs/plans/external-e2e-runner-capacity/plan.md`
- `docs/plans/external-e2e-runner-capacity/task-01-add-opt-in-e2e-runner-selection.md`

## Dependencies

None.

## Risks

- String-based workflow contract extraction depends on stable job boundaries.
  The test must use the existing `job_block` helper.
- A live provider pilot is the only proof of image and capacity compatibility.
  Local checks prove the workflow contract, not external availability.

## Parallelism

`sequential`

## Inputs

- `REQ-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-001` and
  `REQ-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-002`.
- The external E2E runner capacity system design.
- ADR 2026-09-06 for the opt-in placement and trust boundary.
- Existing E2E job topology and workflow contract tests.

## Results

- Added `KANDEV_CI_EXTERNAL_ENABLED` expressions to the two light jobs and three
  standard jobs in `.github/workflows/e2e-tests.yml`.
- Kept the Playwright image, Docker/Kind, Kubernetes compatibility, and desktop
  jobs on `ubuntu-latest`.
- Added the eligible and protected job assertions to the E2E workflow contract
  test. The full contract suite passes with 14 tests.
- Added the burst activation, tier values, rollback, security boundary, and
  pilot measurement procedure to `docs/ci-merge-queue.md`.
- Action pinning tests and lint pass. `actionlint` v1.7.12 passes through
  `go run`. Targeted `zizmor` reports no findings.
- The Ubicloud pilot is not run in CI code changes. Operators must collect the
  three-run comparison after merge before expanding the pilot.
