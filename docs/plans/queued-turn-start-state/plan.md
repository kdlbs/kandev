---
created: 2026-09-09
status: complete
requirements:
  - REQ-TASKS-RUNTIME-STATE-PUBLICATION-ORDER-001
system_design:
  - ../../specs/tasks/system-design/runtime-state-publication-order.md
legacy_specs: []
---

# Implementation Plan: Queued turn-start state

## Overview

Keep an admitted queued turn running when its start moves the workflow step.
One work order repairs the backend lifecycle and verifies the visible result.
This is conformance with the existing runtime-state contract, especially
AC-TASKS-RUNTIME-STATE-PUBLICATION-ORDER-001.5. Requirements need no change.

## Confirmed cause and evidence

The reported task is `9f724e85-e643-4ee8-a60b-24efdb98f01b`; the affected
session is `8bce4cee-e40c-44f5-817b-c8e5ee48c925`.
Retained backend evidence was read from
`/root/.kandev/logs/backend-logs-2026-09-09-000007.log`, lines 66351–66417.
Times below are UTC on 2026-09-09; retained log timestamps use UTC+01:00.

| Time | Backend event |
| --- | --- |
| 08:23:53.227 | Send Now claims one queued message. |
| 08:23:53.240 | The session is published as RUNNING. |
| 08:23:53.244 | Turn `42f8517d-aee0-435e-a22d-b9b5ac5c65f2` starts. |
| 08:23:53.254 | The workflow evaluates an on_turn_start transition. |
| 08:23:53.264 | Task update publishes the destination step and RUNNING session. |
| 08:23:53.267 | The session changes from RUNNING to WAITING_FOR_INPUT. |
| 08:23:53.280 | Task state changes to REVIEW, retaining the destination step. |
| 08:23:53.281 | The executor sends the queued prompt to the agent. |

`promptSendNowClaim` calls `promptTask` with an `afterClaim` callback.
Prompt admission first sets the session running and creates its turn.
The callback then calls `processOnTurnStartViaEngine`.
`applyEngineTransitionWithCommitMode`, in `transitionLifecycleOnTurnStart`
mode, unconditionally calls `setSessionWaitingForInput` after profile
preparation. That helper also reconciles the task to Review. Prompt dispatch
continues afterward without restoring the admitted session state.

The browser receives and applies these server events. The earlier primary
session mismatch briefly delays card updates, but the primary session is
correct before the final reversal. No delayed cancellation event is needed
to explain the defect. Metadata-only state events (`newState=-`) are separate.

The existing `workflow_e2e_test.go` table includes running on_turn_start
transitions but omits `ExpectState` on those rows.

## Scope

### In scope

- Preserve current-turn ownership and running state across a successful
  same-session on_turn_start transition.
- Verify Send Now and the ordinary queued-dispatch caller of the same hook.
- Preserve pre-admission transitions, profile preparation, WIP admission,
  failure cleanup, and legitimate end-of-turn settlement.
- Prove desktop and mobile receive the same corrected authoritative state.

### Out of scope

- Client-side state overrides, new state values, and changes to grouping.
- Queue ordering, cancellation policy, schema changes, and Office ownership.
- Changes to the reported live task or its agent process.

## Technical approach

Repair the on_turn_start lifecycle branch in
`apps/backend/internal/orchestrator/event_handlers_workflow.go`.
Distinguish a transition for an already-admitted turn from preparation for a
future prompt. Preserve the admitted turn when the effective session remains
the same. Keep profile-switch and failure behavior explicit; do not remove
all waiting-state writes across lifecycle modes.

The implementation uses the session state and effective session ID already
available in the lifecycle branch. No new context parameter or turn registry
is necessary. The per-session cancellation guard and queue claim order remain
unchanged. The hook does not write RUNNING afterward, so it cannot revive a
cancelled turn.

The implementation must not depend on a later activity event healing the state.
The existing task publication order and client freshness design remain valid.

## Tests

Add `queue_send_now_workflow_state_test.go` with
`TestSendNowWorkflowTransitionPreservesRunningState`. Use the real queue,
workflow engine, prompt admission, and repository with a controllable executor.
Inspect state synchronously inside the fake provider dispatch, before it
returns. Assert session RUNNING, task IN_PROGRESS, the target workflow step,
an active turn, and one prompt. Capture lifecycle publications and reject any
waiting/review reversal before dispatch returns.

Add paired tests for the pre-admission hook and ordinary FIFO callback.
Retain coverage for profile changes, WIP deferral, failed dispatch, explicit
cancellation, and genuine turn completion. Strengthen the existing workflow
table's on_turn_start rows with state assertions.

These checks cover AC-TASKS-RUNTIME-STATE-PUBLICATION-ORDER-001.1, .2, .5,
and .8. Desktop/mobile evidence covers .6.

## E2E tests

Extend `apps/web/e2e/tests/chat/message-queue.spec.ts` with
`Send Now keeps a workflow transition running`. Start from a settled session
with a queued slow prompt and a Review-to-In-Progress on_turn_start action.
Click Send Now and assert the running composer and State-grouped sidebar
before completion, without reloading. Assert legitimate settlement afterward.

Extend `apps/web/e2e/tests/chat/mobile-message-queue-management.spec.ts` with
the same scenario using the existing touch queue control and task switcher.
Shared data semantics are the mobile requirement; layout and navigation stay
within the existing composition. Reuse the existing queue test fixtures.

## Work orders

- [x] [Task 01: Preserve admitted turn state](task-01-preserve-admitted-turn.md)

## Verification results

The Send Now and FIFO regressions first failed with WAITING_FOR_INPUT instead
of RUNNING at provider dispatch. Both pass after the lifecycle fix. The
pre-admission case and strengthened workflow table also pass.

The following targeted orchestrator groups passed with `-race -tags fts5`:

- `Test.*(SendNow|SendQueuedNow|Queued|Workflow|OnTurnStart|PromptTask|Cancel)`
- `Test.*(ApplyEngineTransition|PrepareWorkflowStepSession|SwitchSessionForProfile|WIP|Wip)`
- `Test(SwitchSessionForStep|ProcessOnEnter_ProfileSwitch)`

Desktop Chromium: two Send Now tests passed. Mobile Chrome: two Send Now tests
passed. These include the new workflow regression and existing priority-order
coverage on each platform. The new cases verify the running composer, State
grouping, authoritative session/task state, and legitimate completion without
a reload.

The initial browser failures exposed test-fixture issues: buffered mock text
needed a tool boundary, and the mobile filter trigger needed drawer scope.
The corrected fixtures pass. Focused ESLint and Prettier checks also pass.

Managed E2E runs used disposable containers and mock agents. The first run
built the artifacts. Later runs reused those artifacts with `--no-build`.
The runners removed their containers after each run. The reported live task,
session, and runtime were not changed.

Public docs need no change. This fix restores the behavior already described
in `docs/public/sessions-and-review.md` and the runtime-state specification.

## Risks

- The shared hook runs both before and after prompt admission; a blanket change
  could make a prepared session busy without a dispatched prompt.
- Profile switching can replace the effective session. Never transfer the old
  session's running state to an unstarted replacement.
- WIP deferral and genuine cancellation must still prevent dispatch or settle
  the correct turn.
