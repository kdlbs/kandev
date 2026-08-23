---
created: 2026-08-23
status: completed
requirements:
  - REQ-OFFICE-AUTOMATION-TARGETS-001
  - REQ-OFFICE-AUTOMATION-TARGETS-002
  - REQ-OFFICE-AUTOMATION-TARGETS-003
system_design:
  - ../../specs/office/system-design/automation-target-modes.md
legacy_specs:
  - ../../specs/office/automations-settings.md
---

# Implementation Plan: Automation Target Modes

## Overview

Add an explicit hidden automation-run or visible normal-task target, make
repository-free firings use Local scratch execution, and preserve exact
continuation/run accounting in both modes. Work is sequential because the
persisted contract and dispatch lifecycle must land before the editor and E2E
flows can rely on them.

## Scope

### In scope

- Persist target mode and explicit repository mode with compatibility migration.
- Admit and dispatch hidden repository/workflow-free runs.
- Create, continue, finalize, and delete visible normal tasks without applying
  hidden coordinator authority or hidden cleanup.
- Expose target, workflow, repository, executor, and continuity choices with
  accessible desktop/mobile copy and equal toolbar button heights.
- Add backend, frontend, mobile, export, and end-to-end regression coverage.
- Update public automation documentation and feature status.

### Out of scope

- Concurrent turns or additional sessions on a reusable target.
- Per-automation MCP capability selection.
- Automatic repository provisioning.
- Converting an existing hidden task into a visible task in place.

## Technical approach

1. Add typed `task_mode` and `repository_mode` contracts in
   `internal/automation`, including store migration, request validation, export,
   and compatibility defaults.
2. Split orchestrator task creation into hidden and visible target paths while
   sharing title rendering, continuation admission, exact run binding, and
   repository resolution. Add a visible automation task origin and use the
   existing lifecycle scratch path for empty repository lists.
3. Route terminal run accounting by exact run identity. Keep hidden
   `SurfaceAutomation` and hidden cleanup constrained to hidden origin tasks;
   leave visible tasks in the normal task lifecycle on automation deletion.
4. Extend the editor payload and types with target/repository modes, include
   Local executor profiles, make Worktree conditional on a repository, require
   workflow only for visible mode, and keep descriptions accessible on phone.
5. Add focused tests and E2E flows for both modes, then update public docs and
   verification evidence.

## Tests

- `internal/automation`: schema/default/migration, target validation,
  repository-mode resolution, export, and open-run behavior.
- `internal/orchestrator`: hidden no-repo launch, visible task creation,
  visible continuation, exact completion/stop, normal profile selection, and
  deletion ownership.
- `internal/task/repository/sqlite` and `internal/task/service`: visible
  automation origin appears in task queries and accepts Local scratch tasks.
- `apps/web/components/automations`: target/repository payloads, conditional
  validation, Local/Worktree options, descriptions, continuity, and toolbar
  button geometry.
- `apps/web/components/runs`: shared transcript and visible-run terminal paths.

## E2E tests

- Desktop: create a no-repository/no-workflow hidden automation, run it in Local
  scratch, and verify it succeeds without a repository error.
- Desktop: create a workflow-backed normal-task automation, fire it with
  `new_task`, and verify the task appears in the Kanban/sidebar and its run
  completes.
- Desktop/mobile: switch both target and continuity settings, read the visible
  explanations, exercise a reusable visible task, and assert no horizontal
  overflow. Verify Export and New Automation have equal height.

## Work orders

- [x] [Task 01: Persist target and repository contracts](task-01-persist-target-contracts.md)
- [x] [Task 02: Dispatch and maintain target lifecycles](task-02-dispatch-target-lifecycles.md)
- [x] [Task 03: Build target-mode editor and proof](task-03-editor-and-proof.md)

## Verification results

- Backend: automation/orchestrator 2,464 tests, task service/repository 1,908
  tests, MCP profile/scope/server/handlers 878 tests, lifecycle 1,948 tests,
  and backendapp compilation passed.
- Frontend: 31 focused files and 313 tests passed; typecheck, i18n check, and
  i18n ratchet passed.
- Desktop E2E: 27 automation settings, run-detail, and target-mode tests
  passed, including shared-transcript replies, exact-run stop, hidden
  no-repository execution, and visible workflow-task creation.
- Mobile E2E: 10 automation settings and detail tests passed, including the
  selected-run stop action and horizontal-overflow assertions.
- Target-mode E2E: hidden Local scratch and visible normal-task flows passed.
- Public docs: 61 validator tests and 41 published-page validations passed.
- `git diff --check` passed.

## Risks

- Reusing an empty repository list without a migration mode would silently
  change existing automations from workspace-default to scratch execution.
- A visible task accidentally marked with the hidden origin would disappear
  from boards or receive coordinator self-target restrictions.
- Shared continuation code can finalize the wrong visible run unless every
  completion and stop path carries the exact session and turn.
- The Local executor is currently hidden by automation editor filtering, so
  frontend and backend validation must change together.
