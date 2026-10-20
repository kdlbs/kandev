---
id: "02-human-done-final-step"
title: "Human Done moves the task to the final step"
status: done
wave: 2
depends_on:
  - "01-move-approver-guard"
plan: "plan.md"
requirements:
  - REQ-OFFICE-HUMAN-DONE-001
acceptance_criteria:
  - AC-OFFICE-HUMAN-DONE-001.1
  - AC-OFFICE-HUMAN-DONE-001.2
  - AC-OFFICE-HUMAN-DONE-001.3
  - AC-OFFICE-HUMAN-DONE-001.4
  - AC-OFFICE-HUMAN-DONE-001.5
system_design:
  - ../../specs/office/system-design/human-done-final-step.md
---

# Task 02: Human Done moves the task to the final step

## Summary

A human Office PATCH to `done` on a task that is not on the final step moves
the task to the final step, instead of redirecting it to `in_review` and
waking reviewers. The board and status picker settle on the result, or roll
back with the reason when the move is refused.

## In scope

- Backend:
  - `UpdateTaskStatusAsHuman`, called by `applyTaskMutations` when there is
    no agent caller.
  - `TaskFinalStepMover` + setter, and the `backendapp` adapter that resolves
    the highest-position step, calls `MoveTaskWithOptions` (human actor), and
    classifies refusals.
  - `FinalStepMoveRefusedError` and the 409 without `status`.
  - The 200 `{ok, status, workflow_step}` response.
- Frontend:
  - `updateTaskStatusOrTranslateGate` returns the result and throws
    `StatusChangeRefusedError`.
  - `applyStatusDrop` and `useOptimisticTaskMutation` settle on the result
    and roll back on refusal; the board and `status-picker.tsx` show the info
    toast for a non-done result.
  - New keys `task:cannotMarkDoneCompletionCriteria`,
    `task:cannotMarkDoneMoveRefused` and `task:movedToFinalStepNotCompleted`
    in every catalog.
- E2E: the flows in the plan's *E2E tests* table, plus updates to existing
  specs that PATCH `done` from a non-final step as a human.

## Out of scope

- The agent path and the final-step Done gate (kept, AC-OFFICE-HUMAN-DONE-001.4).
- The move approver guard itself (Task 01).

## Acceptance

- `TestUpdateTaskStatusAsHuman_DoneAtWork_MovesToFinalStep` fails on current
  main (409 `workflow_step` redirect) and passes after.
  `TestUpdateTaskEndpoint_409OnWorkflowStepGate` stays as the no-mover
  fallback.
- On a refused move, the state and step are unchanged, no `task_review_requested`
  run is queued, and the PATCH returns 409 without `status`. The agent tests
  in `approval_gate_step_test.go` stay green.
- The desktop and phone E2E flows pass, and the rendered board matches UI-01
  and UI-02.

## ASCII UI preview

Excerpt from the [plan](plan.md#ascii-ui-preview), UI-01 (AC-OFFICE-HUMAN-DONE-001.1-.3):

```text
 success:  Todo | In progress | In review | Done [Fix login]
 refused:  Todo | In progress [Fix login] | In review | Done
           toast(error): Cannot mark done: awaiting approval from CEO
 no-complete final step:
           Todo | In progress [Fix login] | In review | Done
           toast(info): Moved to "Done". This step doesn't complete the task.
```

UI-02 (AC-OFFICE-HUMAN-DONE-001.5): the status picker gives the same outcomes
and toasts on desktop and phone.

## Verification

```bash
cd apps/backend && go test -tags fts5 -count=1 -run 'AsHuman|HumanDone|TestUpdateTaskEndpoint|TestUpdateTaskStatus' ./internal/office/dashboard/
cd apps/backend && go test -tags fts5 -count=1 -run 'TestOfficeFinalStepMover' ./internal/backendapp/
cd apps/backend && go test -tags fts5 -count=1 ./internal/office/... ./internal/orchestrator/...
cd apps/web && pnpm exec vitest run lib/api/domains/office-status-gate.test.ts app/office/tasks/ hooks/use-optimistic-task-mutation-settled-status.test.tsx hooks/use-optimistic-task-mutation.test.tsx
cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/office/board-done-final-step.spec.ts tests/office/approval-flow.spec.ts tests/office/tasks.spec.ts
pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/office/mobile-board-done-final-step.spec.ts
make -C apps/backend lint
```

## Files likely touched

- `apps/backend/internal/office/dashboard/human_done_final_step.go` (seam, refusal type, `UpdateTaskStatusAsHuman`) and its test
- `apps/backend/internal/office/dashboard/service.go` (seam field)
- `apps/backend/internal/office/dashboard/handler.go`
- `apps/backend/internal/office/dashboard/approval_gate_step_test.go`, `decisions_test.go`
- `apps/backend/internal/backendapp/adapters_office_final_step.go` and its test, `main.go` (wiring)
- `apps/backend/internal/task/handlers/errors.go` (exported `CompletionGateCode`, `MoveConflictCode`)
- `apps/web/hooks/use-optimistic-task-mutation.ts`
- `apps/web/lib/api/domains/office-status-gate.ts` and its test
- `apps/web/app/office/tasks/use-board-drag.ts` and its test
- `apps/web/components/task/simple/components/status-picker.tsx`
- `apps/web/src/locales/{en,pseudo,pt-pt,ja,ko,zh-cn,zh-hk,zh-tw}/task.json`
- `apps/web/e2e/tests/office/board-done-final-step.spec.ts`, `mobile-board-done-final-step.spec.ts` (new)
- `apps/backend/internal/office/AGENTS.md` (spec-table row for `human-done-final-step`)

## Dependencies

Task 01.

## Risks

- Phone coverage uses the status picker instead of a touch drag.
- The final step's entry actions run on the move. Confirm that the default
  Office `done` step's actions do not wake agents unexpectedly in the E2E
  fixture.

## Results

Done. Verification commands pass; see [plan verification results](plan.md#verification-results).
