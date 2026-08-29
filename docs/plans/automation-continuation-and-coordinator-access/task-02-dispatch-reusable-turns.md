---
id: "02-dispatch-reusable-turns"
title: "Dispatch reusable automation turns"
status: completed
wave: 2
depends_on: ["01-persist-continuation-policy"]
plan: "plan.md"
spec: "../../specs/office/requirements/automation-continuity.md"
---

# Task 02: Dispatch Reusable Automation Turns

## Acceptance

- Every firing owns a `triggered` run before publication; reuse mode admits at most one active turn
  and binds the exact accepted task/session/turn.
- First creation, normal resume, launch-identity replacement, and non-resumable fallback record the
  correct thread action without renaming a reused task.
- Reconciliation fails an exact bound run that has no execution, open turn, or pending blocker;
  exact-run stop cancels only its bound turn and releases the slot.

## TDD scenarios

1. RED: Race two firings and prove one reusable turn plus one visible concurrency skip.
2. RED: Cover creation, resume, fallback classes, title behavior, and launch-identity rotation.
3. RED: Complete two bound turns in one task and prove terminal events update only their run; prove
   a manual reply changes no run.
4. RED: Cover pre-dispatch restart repair, dead bound-run repair, live blocker preservation, and
   exact-run cancellation.
5. GREEN: Add atomic admission, run-ID events, reusable dispatch, exact binding/finalization, and
   recovery action.
6. REFACTOR: Keep fresh/reuse branches behind one typed dispatch result and exact-run owner.

## Verification

- `cd apps/backend && go test -tags fts5 ./internal/automation ./internal/orchestrator`
- `git diff --check`

## Files likely touched

- `apps/backend/internal/automation/service.go`
- `apps/backend/internal/automation/store.go`
- `apps/backend/internal/automation/scheduler_test.go`
- `apps/backend/internal/automation/handlers.go`
- `apps/backend/internal/automation/handlers_test.go`
- `apps/backend/pkg/websocket/actions.go`
- `apps/backend/internal/orchestrator/event_handlers_automation.go`
- `apps/backend/internal/orchestrator/event_handlers_automation_test.go`
- `apps/backend/internal/orchestrator/event_handlers_streaming.go`
- `apps/backend/internal/orchestrator/task_operations.go`

## Dependencies

Task 01 defines the policy, exact binding, title, and open-status contracts.

## Inputs

- Runtime state machine, failure modes, and exact-run recovery scenarios.
- Existing `StartTask`, `PromptTask`, cancellation, and startup-reconciliation paths.

## Parallelism

Parallel-safe with Tasks 04 and 06 after Task 01: this task owns automation/orchestrator firing
paths, Task 04 owns MCP scope/registration, and Task 06 owns frontend editor files.

## Output contract

Report race ownership, bindings, fallback reasons, title behavior, reconciliation, cancellation,
files changed, and exact test output.

## Risks

- A task- or session-only terminal lookup can settle the wrong run in a shared thread.

## Results

Implemented reusable session dispatch, replacement on missing or incompatible launch identity, exact-turn completion, and no-reset/no-rebase worktree behavior. Verified with the orchestrator suite: 2,060 tests passed.
