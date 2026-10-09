---
id: "08-reduce-scheduling-stages"
title: "Remove redundant CI scheduling stages"
status: done
wave: 3
depends_on: ["07-measure-ci-efficiency"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-CI-PERFORMANCE-003
  - REQ-PLATFORM-CI-PERFORMANCE-005
acceptance_criteria:
  - AC-PLATFORM-CI-PERFORMANCE-003.3
  - AC-PLATFORM-CI-PERFORMANCE-005.1
  - AC-PLATFORM-CI-PERFORMANCE-005.4
system_design:
  - ../../specs/platform/system-design/ci-performance.md
---

# Task 08: Remove redundant CI scheduling stages

## Summary

Combine allocation with change detection in each test workflow. Make the existing E2E report job publish the required E2E result.

## In scope

- Keep the `changes` job ID and publish the allocation plan from its hosted bootstrap. Update every consumer and aggregate dependency.
- Merge E2E report and gate responsibilities under the public check name `E2E Tests Passed`, with complete dependency and report-failure handling.
- Update runner-family inventories, workflow contracts, the merge-queue runbook, and relevant existing design references.

## Out of scope

New runner providers, changed permissions, test removal, shard-count increases, or a gate that polls while holding a runner.

## Acceptance

- A full backend/frontend/E2E workflow set schedules four fewer jobs with unchanged selected tests and public required names.
- All-pass and deliberate-skip fixtures succeed; failed planning, missing evidence, cancelled/failed jobs, unexpected skips, and report failures block the required result.
- Existing allocation percentages and protected runner placement remain intact; change detection stays hosted.

## Verification

Run from the repository root. Install workspace dependencies once in a fresh worktree.
New test and helper paths below are implementation deliverables.

```bash
python3 .github/scripts/backend-tests-workflow-contract_test.py
python3 .github/scripts/frontend-tests-workflow-contract_test.py
python3 .github/scripts/e2e-tests-workflow-contract_test.py
python3 .github/scripts/external-runner-workflow-contract_test.py
python3 .github/scripts/runner-plan_test.py
python3 .github/scripts/lint-action-pinning_test.py
actionlint .github/workflows/backend-tests.yml .github/workflows/frontend-tests.yml .github/workflows/e2e-tests.yml
zizmor .github/workflows
git diff --check
```

Use natural candidate runs or one explicitly targeted candidate rollout to collect Task 07 evidence. Do not flood the loaded fleet with duplicate full runs.

## Files likely touched

- `.github/workflows/backend-tests.yml`
- `.github/workflows/frontend-tests.yml`
- `.github/workflows/e2e-tests.yml`
- `.github/scripts/*workflow-contract_test.py`
- `docs/ci-merge-queue.md`
- `docs/specs/platform/system-design/external-e2e-runner-capacity.md`
- `docs/specs/platform/system-design/e2e-duration-aware-sharding.md`

## Dependencies

07-measure-ci-efficiency.

## Risks

The combined E2E report must run after failures and irrelevant changes without reporting a false pass. Review ruleset check identity before a production rollout.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/platform/requirements/ci-performance.md), IDs in frontmatter.
- [System design](../../specs/platform/system-design/ci-performance.md), October continuation.
- [October evidence](evidence-2026-10-08.md).
- [Plan](plan.md), acceptance gates and preserved companion statuses.

## Results

Implemented three hosted bootstrap jobs and the consolidated E2E required check.
Dependency-outcome fixtures exercise success, deliberate skips, failure, cancellation,
missing verdicts or runner-plan outputs, sidebar-only dispatch, missing blobs,
and missing report files.
Workflow contracts, runner planner tests, action pinning, and Actionlint pass.
Zizmor reports the same pre-existing Windows CMD finding on the four changed
workflows; the full workflow scan has existing findings outside this scope.
The active main ruleset (13341245) still requires E2E Tests Passed, Run Backend
Tests, and Frontend Tests Passed. All three names are preserved. Irrelevant E2E
runs initialize the report container but skip dependency installation and all
report processing; hosted measurements must include this setup cost.
No hosted gain is claimed before the changed workflows run.
