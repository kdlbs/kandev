---
created: 2026-11-13
status: complete
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-001
system_design:
  - ../../specs/tasks/system-design/workflow-profile-session-lifecycle.md
  - ../../specs/tasks/system-design/workflow-profile-readiness.md
  - ../../specs/executors/system-design/per-user-ownership.md
legacy_specs: []
---

# Implementation Plan: Destination executor admission for workflow switches

## Overview

Fixes [#5307](https://github.com/kdlbs/kandev/issues/5307). Suppose a card's
executor profile has been deleted and the user moves the card to a step that
needs a new agent session. Today `POST /tasks/:id/move` returns 200 and the
card shows on the new step, but no session is created and the task stays in
`SCHEDULING` indefinitely.

After this fix, the move is refused before the step changes, with a localized
reason. If a switch fails after it has started, the task returns to its
earlier state.

The behavior is specified by AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.11
(stop the switch, keep the current session recoverable, show the error),
001.14 (automatic transitions), and the new 001.16.

## Root cause

The issue's trace matches current `main` (`841d12d1dd0`):

1. `switchSessionForStepWithPoliciesAndCandidateAndRoute`
   (`apps/backend/internal/orchestrator/event_handlers_workflow.go`) writes
   task state `SCHEDULING` before it prepares the destination.
2. `prepareWorkflowReplacementSession` passes the source session's
   `ExecutorProfileID` to `PrepareSessionForExistingEnvironment`.
   `prepareSessionAttempt` → `prepareExecutorAdmission` →
   `task/service.PrepareExecutorAdmission` → `executorAdmissionProfile` →
   `GetExecutorProfile` returns `ErrExecutorProfileNotFound`. This is the
   intended refusal: a binding to a deleted profile confers no compute
   (per-user ownership design, "Autonomous attribution"). The executor-config
   layer (`applyProfile`) tolerates the missing row, so the issue's WARN line
   comes from there. The ERROR line comes from admission.
3. `maybySwitchSessionForProfile` handles the error by calling
   `setSessionWaitingForInput` on the source session. That session is already
   `WAITING_FOR_INPUT`, so `setSessionWaitingForInputWithHook` returns before
   `writeTaskReviewState`. Nothing restores the task from `SCHEDULING`.
4. The manual-move preflight (`PreflightWorkflowStepMove` →
   `preflightFixedWorkflowStepCredentials` /
   `preflightExplicitWorkflowStepCredentials`) checks only managed Git
   credentials. With no credential broker it returns nil, so the move commits
   and the HTTP handler returns 200. Step entry runs after the commit.

## Decision

Reject the move. Do not fall back to another profile. The per-user ownership
design says that a deleted-profile binding confers no compute for any caller.
Substituting the executor's default profile would silently grant compute and
change runtime configuration (prepare script, env, secrets). The issue lists
rejection as an acceptable outcome. No ADR is needed: this applies existing
admission policy at an earlier boundary.

## Scope

In scope:

- Executor admission in the shared step-entry preflight for fresh and reused
  destinations. Manual moves and automatic transitions both use that preflight.
- Typed error, HTTP 409 and WebSocket CONFLICT mapping, and localized move
  error copy.
- Task-state rollback when any switch fails after `SCHEDULING`.

Out of scope:

- Rebinding a materialized task to a new runner. Runner switch is
  pre-materialization only, so an orphaned task's existing conversation stays
  refused by prompt admission, as the ownership design intends.
- Blocking executor-profile deletion while sessions reference it.
- Changing `applyProfile`'s tolerance of missing rows.

## Work orders

| # | Work order | Depends on | Status |
| --- | --- | --- | --- |
| 01 | [Admission preflight and SCHEDULING rollback](task-01-admission-preflight-and-rollback.md) | none | done |
| 02 | [Move error surface](task-02-move-error-surface.md) | 01 | done |

The work orders are sequential. WO-02 consumes the sentinel that WO-01
introduces.

## ASCII UI preview

`UI-01: Move refusal`. Entry point: kanban drag, or the task-detail stepper
move to a step with a new-session agent profile, for a task whose executor
profile was deleted. Only the error copy changes. The banner and toast
primitives already exist.

```text
Before (move "accepted", card stuck)      After (move refused, card stays)
+--------------------------------+        +------------------------------------------+
| P2 · Review                    |        | ! This task's executor profile is no     |
|  [card]  Preparing agent...    |        |   longer available, so this step's agent |
|  (no session ever appears)     |        |   cannot start.                          |
+--------------------------------+        +------------------------------------------+
                                          | P1 · Plan                                |
                                          |  [card]  Waiting for input               |
                                          +------------------------------------------+
```

The phone layout is the same: the existing move-error toast shows the same
string, and the existing mobile move sheet keeps its layout. The wording is
illustrative. The final English copy and its six translations go through
`t()`.

## Verification strategy

- Regression (fails before the fix):
  `TestSwitchSessionForStep_DeletedExecutorProfileRestoresTaskState` asserts
  the task returns to its pre-switch state, not `SCHEDULING`.
  `TestPreflightWorkflowStepMove_RejectsDeletedExecutorProfile` asserts the
  sentinel is returned.
- Service: the move with a rejecting preflight leaves the step unchanged.
- Handler: HTTP 409 and WS CONFLICT with `task_move_executor_unavailable`.
- Frontend: the `task-move-error-message` unit test maps the code.
- E2E: a Playwright spec deletes the task's executor profile, moves the card,
  and sees the localized error. The card stays on its step and the task is
  not `SCHEDULING`.

## Risks

- **Over-refusal.** Admission also refuses a deleted executor, a missing
  environment, or revoked private-profile ownership. All of those already fail
  later at preparation, so refusing earlier changes when the error appears,
  not whether the move works.
- **Reused-destination admission.** Reuse admits the reused session's own
  bindings, which matches the existing credential preflight.
  `TestSwitchSessionForStepUsesReusableSessionExecutorProfileForCredentialAdmission`
  guards this choice.
- **Concurrency.** The rollback is a CAS from `SCHEDULING` only, so a
  concurrent state writer is never overwritten.
