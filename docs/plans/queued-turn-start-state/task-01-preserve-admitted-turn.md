---
id: "01-preserve-admitted-turn"
title: "Preserve admitted turn state"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-RUNTIME-STATE-PUBLICATION-ORDER-001
acceptance_criteria:
  - AC-TASKS-RUNTIME-STATE-PUBLICATION-ORDER-001.1
  - AC-TASKS-RUNTIME-STATE-PUBLICATION-ORDER-001.2
  - AC-TASKS-RUNTIME-STATE-PUBLICATION-ORDER-001.5
  - AC-TASKS-RUNTIME-STATE-PUBLICATION-ORDER-001.6
  - AC-TASKS-RUNTIME-STATE-PUBLICATION-ORDER-001.8
system_design:
  - ../../specs/tasks/system-design/runtime-state-publication-order.md
---

# Task 01: Preserve admitted turn state

## Summary

Repair the successful on_turn_start lifecycle for an already-admitted queued
turn. Keep the task and session projections consistent while that turn runs.

## In scope

- Write the failing `TestSendNowWorkflowTransitionPreservesRunningState`
  regression before changing production code.
- Exercise the real Send Now claim, prompt admission, workflow transition,
  repository writes, and lifecycle publication path. Inspect state inside
  the fake executor before its prompt call returns.
- Add the corresponding ordinary FIFO and pre-admission transition cases.
- Correct the shared transition branch with existing turn/session ownership.
- Add desktop and mobile queue regressions described in the plan.
- Verify profile switching, WIP deferral, cancellation, failed prompts, and
  real completion retain their existing behavior.

## Out of scope

Frontend production changes, queue policy changes, persistence migrations,
Office state ownership, and live-instance intervention.

## Acceptance

1. Same-session Send Now and FIFO transitions keep the admitted turn RUNNING
   and task IN_PROGRESS, with one dispatched prompt and no false idle events.
2. Preparation without an admitted turn, changed profiles, WIP deferral, and
   real settlement remain correct. The fix preserves claim serialization.
3. Desktop State grouping and the mobile task surface show active work without
   a reload; genuine completion restores the expected idle state.

## Verification

Run the new backend regression first and record its expected state failure.
Then implement and run the focused package cases from `apps/backend`:

```bash
rtk go test -tags fts5 ./internal/orchestrator -run 'TestSendNowWorkflowTransitionPreservesRunningState' -count=1
rtk go test -race -tags fts5 ./internal/orchestrator -run 'Test.*(SendNow|SendQueuedNow|Queued|Workflow|OnTurnStart|PromptTask|Cancel)' -count=1
```

Before the first pnpm command in a fresh worktree, run from `apps`:

```bash
rtk pnpm install --frozen-lockfile
```

Run sequentially from `apps/web`; each managed run builds its artifacts:

```bash
rtk pnpm e2e:run --project chromium tests/chat/message-queue.spec.ts -- --grep 'Send Now'
rtk pnpm e2e:run --project mobile-chrome tests/chat/mobile-message-queue-management.spec.ts -- --grep 'Send Now'
```

Use the same test title fragment for the added mobile scenario. Confirm test
discovery and record command results. Do not run against the user's instance.

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/queue_send_now.go` (only if context is needed)
- `apps/backend/internal/orchestrator/event_handlers_agent.go` (paired FIFO path)
- `apps/backend/internal/orchestrator/task_operations.go` (admission context only)
- `apps/backend/internal/orchestrator/queue_send_now_workflow_state_test.go`
- `apps/backend/internal/orchestrator/workflow_e2e_test.go`
- `apps/web/e2e/tests/chat/message-queue.spec.ts`
- `apps/web/e2e/tests/chat/mobile-message-queue-management.spec.ts`

## Dependencies

None.

## Risks

An unconditional removal of idle settlement can break replacement-session
preparation. An unconditional post-hook RUNNING write can revive a cancelled
or superseded turn. Preserve ownership instead of repairing state afterward.

## Parallelism

`sequential`

## Inputs

- [Confirmed trace and repair approach](plan.md).
- [Runtime-state requirements](../../specs/tasks/requirements/runtime-state-publication-order.md).
- [Runtime-state design](../../specs/tasks/system-design/runtime-state-publication-order.md).
- `queue_send_now_test.go`: existing real queue claim and executor fixture.
- `workflow_e2e_test.go`: existing workflow engine fixture and turn-start rows.
- Read scoped backend/web guidance and the TDD/E2E skills before implementation.

## Results

Implemented in the successful on_turn_start branch. A same-session transition
preserves an admitted RUNNING state. Pre-admission and replacement-session
preparation retain their waiting-state behavior.

The Send Now and FIFO tests failed before the fix and pass afterward. Their
fake provider inspects state synchronously at dispatch, so no channel or
timing-dependent pause is necessary. The tests also reject false idle events
and duplicate prompts or user messages.

The focused race-enabled groups passed, including prompt cancellation,
workflow transitions, profile changes, and WIP handling. Desktop and mobile
Send Now suites each passed both selected tests. Both new browser cases
verify active state without a reload and normal settlement after completion.
Focused ESLint and Prettier checks passed.

The shared browser fixture is
`apps/web/e2e/tests/chat/message-queue-workflow-helpers.ts`. It flushes mock
text at a tool boundary and scopes mobile navigation to the Tasks drawer.
No frontend production changes or live-instance mutations were necessary.
See [verification details](plan.md#verification-results).
