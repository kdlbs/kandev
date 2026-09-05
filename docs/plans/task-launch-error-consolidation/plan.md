---
created: 2026-09-05
status: completed
requirements:
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001
  - REQ-TASKS-REMOTE-CONTRIBUTION-TASKS-001
system_design:
  - ../../specs/tasks/system-design/task-launch-failure-recovery.md
  - ../../specs/tasks/system-design/remote-contribution-tasks.md
legacy_specs: []
---

# Implementation Plan: Task Launch Error Consolidation

## Overview

The change first isolates pull-request refs from reused local branch names.
Then it makes recovery actions category-specific. The final work order gives
one error card ownership of the desktop and phone presentation.

## Confirmed root cause

PR #3408 uses `feature/unit-7-reject-leg-wa-waj` at
`a76c5d8de780b0b857af8195eab4772bb1a5839b`. The shared clone still has the same
local branch from PR #3294 at `f825122f586978ed5e33fe48d61e401a610318e2`.

Neither commit contains the other. The required fetch tried to update the local
branch directly. Git rejected the update as non-fast-forward, and preparation
stopped before the agent started.

The backend stored one typed launch error. The frontend also derived a previous
agent error, a failed preparation row, an empty-turn warning, a generic failed
status, and a stopped-session recovery banner from the same failed start.

## Scope

### In scope

- Preserve a reused local branch and prepare the exact pull-request head.
- Classify checkout preparation separately from base-branch errors.
- Offer only category-valid recovery actions.
- Render one matching launch-error card on desktop and phone.
- Keep bounded technical details in one disclosure.
- Prove the error and recovery flows with unit and Playwright tests.

### Out of scope

- Reconcile provider and local history after an agent starts.
- Change generic runtime-error recovery after successful agent startup.
- Add new provider-specific branch pickers.
- Change pull, push, merge, rebase, or force-push policy.

## Technical approach

### Pull-request ref isolation

Update `apps/backend/internal/worktree/manager_lifecycle.go` to fetch a
pull-request head into a PR-scoped internal ref. Verify that ref before worktree
creation. If a same-named local branch has unrelated history, create a unique
task branch from the verified ref using a task-owned suffix and bounded
collision retries. Set a source-branch upstream only when its commit matches
the verified PR start point.

Keep the named local branch unchanged. Continue to use the existing Git
admission, timeout, and non-interactive environment.

### Failure category and recovery

Add `workspace_checkout_failed` and `retry_launch` to
`apps/backend/internal/task/models/launch_errors.go`. Preserve unknown-value
filtering in the task-status projection.

Update `apps/backend/internal/orchestrator/executor/launch_failure.go` so the
failure category selects the action list. A repository row must not add branch
actions to a generic error.

Extend `task.launch.recover` in
`apps/backend/internal/orchestrator/task_launch_recovery.go` and its WebSocket
handler. `retry_launch` must validate the current stamp and relaunch without a
base-branch write.

### Single frontend owner

Derive one ownership flag in `apps/web/components/task/task-chat-panel.tsx` from
the active error, session identity, and stamp. Pass this flag to the transcript
and composer boundaries.

Update `TaskLaunchErrorEntry` with category-specific titles, cause text, a
no-change statement, and a technical-details disclosure. Keep action state and
WebSocket requests in the existing component.

When the flag is active, suppress the matching last-agent-error notice,
preparation row, empty-turn notice, failed-agent status, stopped-session banner,
and focused-task toast. Do not suppress unrelated historical or runtime errors.

### Mobile contract

Desktop and phone use the task Chat surface. The phone card stacks full-width
actions. All touch actions have at least a 44-pixel hit area.

The task transcript owns scrolling. The details disclosure wraps its bounded
content without an inner scroll region. Base-branch selection keeps the existing
`MobilePickerSheet` behavior.

## Tests

- `apps/backend/internal/worktree/manager_pr_head_collision_test.go` covers two
  pull requests that reuse one branch name with divergent histories.
- `apps/backend/internal/orchestrator/executor/executor_launch_failure_classification_test.go`
  covers category-specific actions.
- `apps/backend/internal/orchestrator/task_launch_recovery_test.go` covers
  authorized, stale, and failed `retry_launch` requests.
- Frontend tests cover one-owner arbitration, the details disclosure, action
  payloads, launch-error ownership in the composer, recovery-state reset, and
  preservation of unrelated runtime errors.

## E2E tests

- `apps/web/e2e/tests/task/launch-failure-recovery.spec.ts` proves one desktop
  error card, one retry action, and no matching duplicate surfaces.
- `apps/web/e2e/tests/task/mobile-launch-failure-recovery.spec.ts` proves the
  same result on `mobile-chrome`, 44-pixel actions, and no horizontal overflow.

These tests map to
`AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001.2`,
`AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001.7`, and
`AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001.8`.

## Work orders

- [x] [Task 01: Isolate reused pull-request refs](task-01-isolate-reused-pr-refs.md)
- [x] [Task 02: Classify launch recovery actions](task-02-classify-launch-recovery-actions.md)
- [x] [Task 03: Consolidate launch-error presentation](task-03-consolidate-launch-error-presentation.md)

## Verification results

- Backend focused race coverage passed: 1,212 tests across worktree, task
  models, status summary, and executor packages, plus 11 targeted task-launch
  recovery tests.
- Frontend focused coverage passed: 10 Vitest files and 152 tests.
- Review-remediation frontend coverage passed: 7 Vitest files and 75 tests,
  including task-wide launch-error visibility, composer ownership, and
  recovery-state reset.
- `pnpm run typecheck`, `pnpm run lint`, `pnpm run i18n:check`, and
  `pnpm run i18n:ratchet` passed.
- Managed Playwright coverage passed: desktop launch-failure recovery 3/3 and
  mobile launch-failure recovery 2/2.
- The broader orchestrator package run was attempted but exceeded its timeout
  in an existing SQLite/concurrency test; the focused race and recovery suites
  passed independently.

## Risks

- A ref-selection error can discard local-only commits. Tests must prove that
  the existing branch remains unchanged.
- Error events can arrive in a different order from task-status updates. The
  owner logic must converge after either event order.
- Old persisted generic errors can contain branch actions. Projection and UI
  normalization must not show actions that conflict with the current category.
