---
id: "02-move-error-surface"
title: "Surface the destination executor refusal on move"
status: done
wave: 2
depends_on:
  - "01-admission-preflight-and-rollback"
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-001
acceptance_criteria:
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.11
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.16
system_design:
  - ../../specs/tasks/system-design/workflow-profile-session-lifecycle.md
  - ../../specs/tasks/system-design/workflow-profile-readiness.md
---

# Task 02: Surface the destination executor refusal on move

## Summary

Map `workflowmove.ErrDestinationExecutorUnavailable` to a stable move-error
code on HTTP and WebSocket, and render localized copy for it. Prove the
user-visible flow end to end.

## In scope

1. **Handlers** (`apps/backend/internal/task/handlers/errors.go`):
   `moveConflictCode` returns a new `moveConflictCodeExecutorUnavailable =
   "task_move_executor_unavailable"` for the sentinel. HTTP then returns 409
   with `{code, error}`. The `error` text must not contain the profile ID, so
   use a fixed message. `moveEntryOptionsWSError`, or the `wsMoveTask` error
   path, returns `ws.ErrorCodeConflict` with the same code. Also cover the bulk
   move handlers, which use `handleSelectedMoveError`.
2. **Frontend** (`apps/web/components/task/task-move-error-message.ts`): add
   `task_move_executor_unavailable: "task:taskMoveErrorExecutorUnavailable"`.
   Add the key to `en` and to the six complete locales (`pt-pt`, `zh-cn`,
   `zh-hk`, `zh-tw`, `ja`, `ko`). For the Traditional Chinese pair, use
   `pnpm run i18n:zh-hant`. Do not use em dashes.
3. **E2E** (`apps/web/e2e/`): create an executor profile and a two-step
   workflow whose second step uses a different agent profile with start `new`
   and auto-start. Create a task on the profile and let it reach waiting.
   Delete the profile through the API, then move the card in the UI. Assert
   the localized error appears, the card stays on step 1, and the task state
   is not `SCHEDULING`. Run it at both desktop and mobile viewports per the
   existing move-error specs.

## UI preview

See `UI-01: Move refusal` in [plan.md](plan.md#ascii-ui-preview). Only the
error string changes. The existing banner and toast are reused unchanged on
desktop and phone.

## Out of scope

Recovery actions in the error (no new buttons), and runner rebinding.

## Acceptance

1. HTTP move returns 409 `task_move_executor_unavailable`, and WS move returns
   CONFLICT with the same code. Neither leaks the profile ID.
2. Desktop and phone show the localized message, and the card stays on its
   step.
3. `i18n:check` and `i18n:ratchet` pass.

## Commands

```bash
cd apps/backend && go test -trimpath ./internal/task/handlers/ -run 'Move' -count=1
cd apps/web && pnpm exec vitest run components/task/task-move-error-message.test.ts
cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet && pnpm run typecheck
cd apps/web && pnpm e2e:run -- <new-spec-path>
```

## Likely files

- `apps/backend/internal/task/handlers/errors.go` and its move-error test
- `apps/web/components/task/task-move-error-message.ts` and its test
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko}/task.json`
- `apps/web/e2e/tests/...` (a new spec next to the existing task-move specs)

## Results

- HTTP and WebSocket moves now return a stable executor-unavailable conflict
  without exposing the deleted profile ID. Bulk selected moves share the same
  safe HTTP mapping.
- Added localized error copy and the helper mapping for all seven supported
  locales, with Traditional Chinese generated through `i18n:zh-hant`.
- `go test -trimpath ./internal/task/handlers/ -run 'Move' -count=1` passed.
- `pnpm exec vitest run components/task/task-move-error-message.test.ts` passed.
- `pnpm run i18n:check`, `pnpm run i18n:ratchet`, and `pnpm run typecheck` passed.
- Desktop and mobile Playwright flows passed with `pnpm e2e:run --host --shards 1` for `chromium` and `mobile-chrome`. The managed runner built the backend and Vite E2E bundle for both runs.
- `python3 scripts/list-docs.py validate` and `python3 scripts/lint-spec-files.py --all` passed.
- Review follow-up: executor refusals from whole-step bulk moves now return the same safe 409 mapping as selected bulk moves. The backend fallback message now refers neutrally to the task's executor, and the unreachable duplicate WebSocket classification was removed.
- Added a whole-step bulk HTTP regression for the safe conflict code and message. `go test -trimpath ./internal/orchestrator/... ./internal/task/handlers/ -count=1`, changed-file Go lint, and `make -C apps/backend build` passed after the review fixes.
