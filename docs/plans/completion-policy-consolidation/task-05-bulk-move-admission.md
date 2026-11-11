---
id: "05-bulk-move-admission"
title: "Bulk move preflight and partial results"
status: done
wave: 3
depends_on: ["02-completion-policy-hook"]
plan: "plan.md"
requirements:
  - REQ-TASKS-COMPLETION-005
acceptance_criteria:
  - AC-TASKS-COMPLETION-005.5
  - AC-TASKS-COMPLETION-005.6
system_design:
  - ../../specs/tasks/system-design/task-completion.md
---

# Task 05: Bulk move preflight and partial results

## Summary

Check every task in a bulk move before writing any, and report a structured
partial result when a concurrent change refuses a task after the check.
Resolves #5254.

## In scope

- `preflightBulkMove` in `service_workflow.go`, called after
  `acquireBulkMoveStepLocks` in both `BulkMoveSelectedTasks` and
  `BulkMoveTasks`. It runs `validateTaskMove`, the completion gate check, and,
  for a human actor, the registered completion policies over the read pool. It
  collects every refusal. Completion checks apply only when WIP admission will
  complete the task; queued moves preserve the current state.
- `BulkMoveRefusedError` (zero writes) and `BulkMovePartialError`
  (`MovedTaskIDs` and one refusal).
- Refusal codes come from typed errors, not message text. A write failure after
  any move returns the partial-result shape, and public messages do not expose
  internal repository errors.
- HTTP: 409 with `{code, moved_task_ids, refusals[]}` for both selected and
  source-step bulk moves. Source-step bulk moves no longer return a generic
  500 for refusals.
- Web: parse the body in `bulkMoveSelectedTasks`. Show the `UI-01` toast in
  `use-task-workflow-move.ts`. Reconcile cards from `moved_task_ids`. Add copy in
  all seven locales (`pnpm run i18n:zh-hant` for the Traditional Chinese pair).
- Desktop and mobile Playwright specs for the preflight refusal.

## Out of scope

- A single transaction for the whole batch.
- Single-task move error UI.

## Acceptance

- With one refused task in a batch of three, no task moves and the response
  lists the refused task, for both bulk functions.
- A barrier test that adds an approver after the preflight returns
  `bulk_move_partial` with the tasks that moved before the refusal.
- Desktop and phone show the `UI-01` toast and the board shows the actual
  placement of every task.

## ASCII UI preview

`UI-01: Bulk move result toast` (full preview in [plan.md](plan.md#ascii-ui-preview)).

```text
+------------------------------------------------------+
| (!) No tasks were moved                              |
|     "Fix login redirect": approval pending (Alice)   |
+------------------------------------------------------+
| (!) Moved 3 of 5 tasks                               |
|     "Fix login redirect" was not moved: approval     |
|     pending (Alice). Remaining tasks were not moved. |
+------------------------------------------------------+
```

## Verification

```bash
(cd apps/backend && go test -run 'BulkMove' ./internal/task/service/... ./internal/task/handlers/...)
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run hooks/use-task-workflow-move.test.ts lib/api/domains && pnpm run typecheck && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run -- tests/kanban/task-multi-select-bulk-move-refused.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome -- tests/kanban/mobile-task-multi-select-bulk-move-refused.spec.ts)
bash scripts/lint-go-changed $(git diff --name-only origin/main -- '*.go')
```

## Files likely touched

- `apps/backend/internal/task/service/service_workflow.go`, new `service_bulk_move_preflight_test.go`
- `apps/backend/internal/task/handlers/errors.go`, `task_http_handlers.go`, new `bulk_move_error_test.go`
- `apps/backend/internal/task/dto/` (bulk move error body)
- `apps/web/lib/api/domains/kanban-api.ts`, `apps/web/hooks/use-task-workflow-move.ts` and its test
- `apps/web/src/locales/{en,pseudo,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko}/task.json`
- New `apps/web/e2e/tests/kanban/task-multi-select-bulk-move-refused.spec.ts`, `mobile-task-multi-select-bulk-move-refused.spec.ts`

## Dependencies

Task 02 (the policy `Querier` used by the preflight).

## Risks

- Preflight cost while holding step locks on a whole-column `BulkMoveTasks`.
  Record the duration for a 200-task fixture in Results.
- Existing `service_bulk_move_*` lock-order and drift tests depend on the
  current step order. The preflight must run after the post-lock reordering.

## Parallelism

`sequential`

## Inputs

- System design: "Bulk move admission".
- `/mobile-parity` and `/e2e` skills. Existing `tests/kanban/task-multi-select.spec.ts` patterns.

## Results

Added whole-batch preflight, typed refusal and partial-result responses, and
localized desktop and mobile feedback. Web tests (31), typecheck, i18n checks,
and both browser E2E specs passed. The 200-task preflight benchmark completed
in 1.484 seconds.
Follow-up review made completion preflight honor target WIP queueing, classified
move refusals with typed errors, and ensured every write failure after a prior
move returns moved task IDs with a safe generic message. Regression coverage
confirms arbitrary repository text is not used as a refusal code or sent to
the client. Backend verification passed: focused BulkMove service and handler
tests, the full backendapp suite, and the backend build. Changed Go lint passed
for the task, Office, orchestrator, workflow, backendapp, and analytics groups.
Documentation validation, specification lint, and `git diff --check` also
passed. PostgreSQL cases could not run because KANDEV_TEST_POSTGRES_DSN is
unset.
