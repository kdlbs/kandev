---
created: 2026-09-21
status: done
requirements:
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-004
system_design:
  - ../../specs/tasks/system-design/additional-session-workspace-reuse.md
legacy_specs: []
---

# Implementation Plan: Concurrent Session Worktree Visibility

## Overview

Two sessions of one task can run agents against the same physical worktree at
the same time. Kandev permits this by design, but records nothing when it
happens, so an operator learns about it only by noticing two models in the UI
or by finding corrupted files in the repository.

This plan makes the condition observable at the two seams that start an agent
process against an already-attached workspace. It adds no lock and no refusal.

The confirmed path is:

1. A workflow step pins an agent profile, and the source step's
   `profile_session_end_policy` is the default `park`
   (`NormalizeWorkflowProfileSessionEndPolicy`,
   `apps/backend/internal/task/models/models.go:1313`).
2. `parkSessionForProfileSwitchClaimLocked` moves the source session to
   `WAITING_FOR_INPUT` and stops its runtime
   (`apps/backend/internal/orchestrator/workflow_profile_session_lifecycle.go:151`).
   The conversation stays answerable.
3. The destination step's session runs in the same task environment. The
   worktree is keyed by `UNIQUE(task_environment_id, repository_id)`
   (`apps/backend/internal/task/repository/sqlite/base_migrations.go:1138`),
   not by session.
4. A message to the parked session resumes it. Admission is per session only:
   `validateAndLockResume` takes `getSessionLock(session.ID)` and
   `rejectRunningResume` consults `GetExecutionBySession(session.ID)`
   (`apps/backend/internal/orchestrator/executor/executor_resume.go:1173`,
   `:1270`). No seam consults sibling sessions.
5. Both agents write the shared worktree. Nothing is logged or counted.

## Scope

### In scope

- Count sibling sessions in a working state at
  `Executor.LaunchPreparedSession` (agent start) and `Executor.resumeSession`.
- Emit a structured warning and an expvar counter when the count is non-zero.
- Record a skip with its reason when the sibling read fails.
- Document the park interaction in `docs/public/tasks-and-workflows.md`.
- Go unit coverage for co-resident, solitary, and read-failure cases.

### Out of scope

- Refusing, deferring, or serializing a second concurrent launch. The active
  requirement states that multiple agents may write the shared workspace
  concurrently and that this implies no task-wide writer lock
  (`docs/specs/tasks/requirements/additional-session-workspace-reuse.md`,
  "Migrated source detail" and "Out of scope"). Changing that is a product
  decision and belongs on the feature board.
- Changing the default `profile_session_end_policy` from `park`.
- Any board, task-card, or session-tab indicator.
- Changing `is_primary` from routing metadata into an admission gate.
- Repairing already-corrupted files in any user repository.

## Technical approach

### Observation point

`LaunchPreparedSession` and `resumeSession` are the two functions that start an
agent process for a session bound to an existing task environment. Both already
hold the per-session lock and both already call
`admitWorktreeRecovery(ctx, task.ID)`, so the task ID is in hand at the point
where the process is about to start.

The existing `hasOtherWorkingSessions`
(`apps/backend/internal/orchestrator/executor/executor_execute.go:554`) already
lists a task's sessions and applies `isRuntimeWorkingSessionState`. Generalize
it into a helper that returns the sibling IDs and a read-failure signal, and
keep its current boolean caller on that helper so both users share one
definition of "working".

### Counters

Add `apps/backend/internal/orchestrator/executor/session_coresidency_metrics.go`
modelled on `apps/backend/internal/orchestrator/office_stall_metrics.go`:
`session_coresidency_admitted_total` labelled by `site`, and
`session_coresidency_observation_skipped_total` labelled by `reason`. Labels are
closed sets; session and task identifiers appear in the log entry only.

## Tests

- `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-004.1`: a launch and a resume
  with one working sibling each record co-residency naming that sibling.
- `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-004.2`: the observation does not
  change the admission result; a task with no working sibling records nothing.
- `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-004.3`: a failing
  `ListTaskSessions` records a skip with its reason and still admits.
- `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-004.4`: recorded label keys are
  the declared closed set and carry no identifier values.

## E2E tests

None. The change adds a backend structured log, two expvar counters, and
documentation. Nothing a user sees or can do changes, so there is no Playwright
scenario to assert and no mobile composition to check.

## Work orders

- [x] [Task 01: Observe concurrent worktree sessions](task-01-observe-concurrent-worktree-sessions.md)

## Verification results

See task-01's "Definition-of-Done receipts" and "Pre-existing failure proof"
sections for full commands and output. Summary: `apps/backend` fmt/lint/test
green except 4 `internal/worktree` tests reproduced identically against
`origin/main` (pre-existing, unrelated package); root `lint-format` and
`apps/web`'s `i18n:ratchet` green (no UI files touched); spec validation
green. Committed as `06f16e718` on `feature/two-live-sessions-ca-nal`.

## Risks

- `ListTaskSessions` runs once per agent start/resume — a new read on that
  path, not reused from elsewhere. Implemented without an early-stop
  optimization: a task's live-session count is small in practice and the read
  happens once per launch, not per turn, so the collect-all approach was kept
  for simplicity over the plan's original "stop at first working sibling"
  idea. Revisit only if a task with a pathologically large session history is
  observed to matter.
- Observing inside the per-session lock must not perform a blocking call that
  could extend lock hold time materially. The read is the same bounded
  repository call the path already makes.
- An operator who reads the warning may expect Kandev to have prevented the
  overlap. The log wording states the condition is permitted ("Kandev permits
  concurrent sessions on one task") and avoids language implying a failure to
  prevent it; pinned by
  `TestObserveSessionCoresidency_WorkingSiblingLogsWarningAndIncrementsCounter`.
