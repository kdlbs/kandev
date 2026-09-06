---
id: "02-extend-tiered-runners-to-linux-ci"
title: "Extend tiered runners to Linux CI"
status: done
wave: 1
depends_on:
  - "01-add-opt-in-e2e-runner-selection"
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
  - AC-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-001.7
  - AC-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-002.1
  - AC-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-002.2
system_design:
  - ../../specs/platform/system-design/external-e2e-runner-capacity.md
---

# Task 02: Extend Tiered Runners to Linux CI

## Summary

Apply the existing light and standard runner contract to the read-only Linux
jobs that can add merge-queue delay in backend, frontend, architecture-lint,
action-pinning, and harness-lint workflows. Keep host-service, container,
Windows, release, deployment, and credential boundaries on GitHub-hosted
runners.

## In scope

- Route backend control jobs through the light tier and backend Linux test jobs
  through the standard tier.
- Route frontend control jobs through the light tier and the frontend test job
  through the standard tier.
- Route architecture-lint, action-pinning, and harness-lint source-only jobs
  through the light tier.
- Extend workflow contract tests to assert eligible and protected placement.
- Update the merge-queue runbook with the complete eligible-job table.

## Out of scope

- Percentage allocation. That is Task 03.
- Moving backend Postgres/service or Windows jobs.
- Moving Docker/Kind, Kubernetes compatibility, image, desktop, release,
  publishing, signing, deployment, or credential-bearing jobs.
- Changing test selection, matrices, artifacts, permissions, or required gates.

## Acceptance

- With burst mode inactive, every eligible job selects `ubuntu-latest`.
- With burst mode active and a non-empty tier label, eligible jobs select their
  light or standard label. Empty labels use `ubuntu-latest`.
- Protected jobs remain explicitly on `ubuntu-latest` and do not reference the
  burst or tier variables.
- Existing job names, dependencies, matrices, tests, artifacts, timeouts,
  permissions, and required conclusions remain unchanged.

## Verification

```bash
python3 .github/scripts/e2e-tests-workflow-contract_test.py
python3 .github/scripts/frontend-tests-workflow-contract_test.py
python3 .github/scripts/lint-action-pinning_test.py
python3 .github/scripts/lint-action-pinning.py
python3 scripts/lint-harness-files.test.py
python3 scripts/lint-spec-files.py --all
actionlint .github/workflows/e2e-tests.yml .github/workflows/backend-tests.yml .github/workflows/frontend-tests.yml .github/workflows/architecture-lint.yml .github/workflows/lint-action-pinning.yml .github/workflows/lint-harness-files.yml
zizmor .github/workflows/e2e-tests.yml .github/workflows/backend-tests.yml .github/workflows/frontend-tests.yml .github/workflows/architecture-lint.yml .github/workflows/lint-action-pinning.yml .github/workflows/lint-harness-files.yml
git diff --check
```

## Files likely touched

- `.github/workflows/backend-tests.yml`
- `.github/workflows/frontend-tests.yml`
- `.github/workflows/architecture-lint.yml`
- `.github/workflows/lint-action-pinning.yml`
- `.github/workflows/lint-harness-files.yml`
- `.github/scripts/e2e-tests-workflow-contract_test.py`
- `.github/scripts/frontend-tests-workflow-contract_test.py`
- Additional workflow contract tests as needed for source-only jobs.
- `docs/ci-merge-queue.md`

## Dependencies

Task 01 must be complete so the shared variable names and E2E contract are
already established.

## Risks

- Container jobs can use an external VM only when the container workflow does
  not require host Docker or service containers. Protected jobs stay on
  GitHub-hosted runners until measured evidence exists.
- Short lint jobs can add planner or external capacity overhead if the fleet is
  unavailable. Manual burst rollback remains the recovery path.

## Parallelism

`sequential`

## Results

- Added planner-backed light and standard runner selection to the backend,
  frontend, architecture-lint, action-pinning, and harness-lint workflows.
- Kept Postgres service, Windows, Docker/Kind, Kubernetes, image, desktop,
  release, deployment, and credential-bearing jobs on their existing runners.
- Added workflow contract coverage for eligible jobs and protected boundaries.
- The renamed activation variable is `KANDEV_CI_EXTERNAL_ENABLED`.
- Contract, action-pinning, harness-lint, actionlint, specification, and diff
  checks passed.
