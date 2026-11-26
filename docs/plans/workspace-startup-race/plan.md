---
created: 2026-11-25
status: complete
requirements:
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004
system_design:
  - ../../specs/tasks/system-design/task-launch-failure-recovery.md
legacy_specs: []
---

# Prevent workspace reads from failing task startup

## Overview

The initial launch can inherit a temporary error from a concurrent workspace
read. Fix that result handling in one backend work order. Use the
[task launch requirement](../../specs/tasks/requirements/task-launch-failure-recovery.md#req-tasks-task-launch-failure-recovery-004-abandoned-workspace-materialization-recovery)
and the
[launch design](../../specs/tasks/system-design/task-launch-failure-recovery.md#workspace-requests-during-initial-launch).

## Reproduction

At 21:04:32 UTC on November 25, the backend accepted a Worktree launch for a
task linked to PR #5521. The diff and commit-list requests arrived during
startup. Kandev recorded `session workspace not ready: task environment is not attachable`
and set the session and task to `FAILED`. The event log gives the order of
the requests and failure. The owner of the shared operation is unverified.

A temporary Go overlay test on `9ee282c20a36` held
`GetOrEnsureExecution` inside the shared session operation. The test released
the read after `Manager.Launch` joined it. The reader saw a `creating`
environment. Launch inherited the reader's error in three out of three runs.
The control registered one execution in each of three runs.

## Scope

### In scope

- Keep an authorized initial launch eligible while a read reports that its
  workspace is still unavailable.
- Share one runtime registration across concurrent callers for the same
  session. Honor each caller's cancellation and deadline.

### Out of scope

- PR import and branch selection.
- Manual or automatic retry policy for sessions that enter `FAILED`.
- Workspace recovery, re-election, and cleanup policy.

## Technical approach

`validateBoundWorkspaceEnvironment` rejects workspace access to a
`creating` environment. `GetOrEnsureExecution` and
`coalesceSessionLaunch` can share that result because both use the same
session key. The orchestrator then treats the read error as a launch error
and releases the materialization claim.

Give the refusal for `creating` a private error identity. Keep the existing
`ErrSessionWorkspaceNotReady` contract for workspace callers.
When launch receives that specific error from a workspace operation, rejoin
the same session operation within the original context. A launch that owns
the operation runs its regular checks. Share its result with concurrent
launches. Real launch failures follow the current failure path.

This fix applies before agent startup for every executor that uses this
launch path. The Git actions use the existing not-ready response. The
recorded Worktree case supplies the focused regression.

## Tests

| Criteria | Evidence |
| --- | --- |
| AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.12 | `TestFreshLaunchContinuesAfterWorkspaceReadNotReady` in `recovery_launch_coalescing_test.go` controls the request order and checks the read result and single runtime registration. Passed 20 race-enabled runs. |
| AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.1 | `TestLaunchPreparedSessionReleasesOwnedClaimOnEarlyError` in `executor_materialization_release_test.go` checks claim release on a genuine launch error. Passed. |

## E2E test

For AC-004.12, run the existing Chromium test
“creates task from PR URL with worktree executor and verifies branch” in
`apps/web/e2e/tests/task/create-task-github-url.spec.ts`. The test checks
the agent response and checked-out branch. The Go regression controls the
request order.

## Work orders

- [x] [Task 01. Keep launch active during workspace reads](task-01-preserve-workspace-launch.md)

## Verification results

- The temporary overlay reproduced the baseline failure on all three
  concurrent runs; all three launch-only controls passed.
- The new race-enabled regression passed 20 runs, including three successive
  reader-owned refusals before launch admission. Launch registered one runtime;
  its two live waiters shared that execution. The workspace caller retained
  `ErrSessionWorkspaceNotReady`, and a canceled waiter returned
  `context.Canceled`.
- The five lifecycle compatibility tests and the executor claim-release test
  passed.
- `make -C apps/backend build` and changed-file Go lint passed.
- The Chromium PR-worktree scenario passed: one test, one worker.

## Risks

An overly broad retry can repeat a genuine launch failure or bypass an
ownership check. The private error identity limits the retry to the expected
workspace response. The shared session key serializes runtime creation.
