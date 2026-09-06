---
created: 2026-09-06
status: complete
requirements:
  - REQ-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-001
  - REQ-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-002
  - REQ-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-003
system_design:
  - ../../specs/platform/system-design/external-e2e-runner-capacity.md
legacy_specs: []
---

# Implementation Plan: External CI Runner Capacity

## Overview

Add two provider-neutral runner tiers, one burst-mode switch, and an optional
deterministic percentage rollout to the normal Linux CI workflows. Protect
runner placement with workflow and planner contract tests. Update the
merge-queue runbook with the Ubicloud configuration, activation, rollback, and
measurement procedure.

## Scope

### In scope

- Route E2E, backend, frontend, architecture-lint, action-pinning, and
  harness-lint control jobs through the light runner tier.
- Route E2E build/shard/report jobs and backend/frontend Linux test jobs through
  the standard runner tier.
- Use `KANDEV_CI_EXTERNAL_ENABLED` as the single activation switch for both tiers.
- Use `KANDEV_CI_EXTERNAL_PERCENT` to allocate 0 to 100 percent of eligible
  job instances when burst mode is active.
- Keep configured tier labels unchanged while burst mode is inactive.
- Keep `playwright_image`, `e2e-containers`,
  `e2e-kubernetes-compatibility`, and `desktop-e2e` explicitly on
  `ubuntu-latest`.
- Keep backend Postgres/service, Windows, and other protected jobs on
  `ubuntu-latest`.
- Add workflow and planner contract coverage for eligible and protected jobs.
- Document the Ubicloud standard-2 and standard-4 pilot, manual rollback, and
  trust boundary in the merge-queue runbook.

### Out of scope

- Installing the Ubicloud GitHub App or changing live repository Actions
  variables from repository code.
- Moving Docker/Kind, compatibility, image-resolution, desktop, Postgres
  service, Windows, release, publishing, signing, deployment, or
  `pull_request_target` jobs.
- Adding a heavy tier before the first pilot supplies CPU or memory evidence.
- Weighted or adaptive allocation based on historical runtime or queue metrics.
- Guaranteeing an exact percentage of total compute across unrelated workflows.
- Changing shard counts, test selection, worker counts, timeouts, artifact
  flow, permissions, required checks, or merge-queue rules.

## Technical approach

### Workflow placement

For tier-only jobs, use the light expression in the existing E2E workflow:

```yaml
runs-on: ${{ vars.KANDEV_CI_EXTERNAL_ENABLED == 'true' && vars.KANDEV_CI_RUNNER_LIGHT || 'ubuntu-latest' }}
```

Use the same expression with `KANDEV_CI_RUNNER_STANDARD` for standard jobs.
Apply the same contract to the backend, frontend, architecture-lint,
action-pinning, and harness-lint workflows. Each tier variable is the full
runner label.

For percentage rollout, add a read-only `runner-plan.py` planner job on
`ubuntu-latest`. It validates the percentage and emits runner assignments for
each eligible job family. Matrix families receive exactly
`floor(N * percentage / 100)` external assignments. Singleton families use a
stable hash bucket. Downstream jobs consume only `github` or `external` from
planner outputs, then map `external` to the configured tier label.

If burst mode is inactive, eligible jobs use GitHub-hosted capacity. If burst
mode is active, each job uses its configured tier. An empty tier uses
`ubuntu-latest`.

### Contract tests

Extend the E2E, backend, and frontend workflow contract tests with focused
assertions for every affected job block. Add equivalent assertions for the
architecture-lint, action-pinning, and harness-lint workflows where their
placement is source-visible.

Assert `runs-on: ubuntu-latest` in every protected block. Also assert that
protected jobs do not reference the burst, percentage, or tier variables. Keep
Ubicloud labels out of the workflow and contract tests.

Add `.github/scripts/runner-plan_test.py` for percentage validation, matrix
floor allocation, singleton stability, rerun stability, empty labels, and
fail-closed malformed input.

### Operator runbook

Add an external-runner section to `docs/ci-merge-queue.md`. Explain that the
workflow change is merged while burst mode is inactive. List both tier
variables, their initial Ubicloud values, and the protected job categories.

Explain that one variable activates or deactivates paid capacity. State that
variable changes affect only new jobs. Define the comparison and rollback
signals. Link the GitHub and Ubicloud source documentation.

## Tests

- `AC-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-001.1` through `.4`: the workflow
  contract test verifies the toggle, two tier variables, and GitHub fallback.
- `AC-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-001.5`: existing E2E workflow
  contract tests remain unchanged and pass, protecting shard, cache, report,
  and gate behavior.
- `AC-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-001.6`: the design and runbook
  document visible failure plus manual clear-and-rerun recovery. GitHub owns
  label scheduling behavior.
- `AC-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-002.1` and `.2`: the focused
  contract test verifies that protected jobs retain `ubuntu-latest`. Action
  pinning and `zizmor` verify that the workflow adds no unpinned action or new
  permission path.
- `AC-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-002.3`: post-merge pilot evidence
  records selected labels, queue delay, job duration, and reliability for at
  least three representative runs.
- `AC-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-003.1` through `.6`:
  `runner-plan_test.py` and workflow contract tests verify 0/50/100 behavior,
  matrix floor allocation, stable singleton allocation, rerun stability, and
  fail-closed malformed input.

## Work orders

- [x] [Task 01: Add Opt-in E2E Runner Selection](task-01-add-opt-in-e2e-runner-selection.md)
- [x] [Task 02: Extend Tiered Runners to Linux CI](task-02-extend-tiered-runners-to-linux-ci.md)
- [x] [Task 03: Add Deterministic Percentage Rollout](task-03-deterministic-percentage-rollout.md)

## Verification results

- `python3 .github/scripts/e2e-tests-workflow-contract_test.py`: 14 tests passed.
- `python3 .github/scripts/external-runner-workflow-contract_test.py`: 5 tests passed.
- `python3 .github/scripts/runner-plan_test.py`: 8 tests passed.
- `python3 .github/scripts/frontend-tests-workflow-contract_test.py`: 4 tests passed.
- `python3 .github/scripts/lint-action-pinning_test.py`: 9 tests passed.
- `python3 .github/scripts/lint-action-pinning.py`: all 21 workflow files passed.
- `actionlint` v1.7.12: passed for all six touched workflows.
- `zizmor` v1.25.2: no findings for all six touched workflows.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.

The full `zizmor .github/workflows` scan can still report findings in existing
workflows outside this change. The six touched workflows report no findings.

The Ubicloud pilot remains an operator action after merge. It needs at least
three representative runs before a heavier tier or protected job moves.

Tasks 01, 02, and 03 are complete. The Ubicloud pilot remains an operator
action after merge.

## Risks

- A misspelled or unavailable tier label can leave an eligible job queued
  until an operator clears the variable and reruns it.
- An active burst with an empty tier label silently uses `ubuntu-latest` for
  that class. The runbook must include a pre-activation configuration check.
- An external runner image can differ from GitHub's host image even when the
  normal build and shard jobs use repository-owned containers.
- External transparent caching can change cache performance and poisoning risk.
  Branch protection must remain enabled for the public repository.
- Moving the report and gate removes a small amount of GitHub-hosted demand but
  also makes their prompt completion depend on external capacity.
- Runner capacity can reduce queue delay without improving merge-group
  serialization or slow test execution.
- A planner job adds a small GitHub-hosted dependency before eligible jobs can
  start. Singleton percentage allocation is a cohort approximation, not an
  exact per-run share.
