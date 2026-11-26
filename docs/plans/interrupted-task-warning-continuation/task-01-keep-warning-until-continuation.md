---
id: "01-keep-warning-until-continuation"
title: "Keep the warning until continuation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-INTERRUPTED-TASK-INDICATOR-001
acceptance_criteria:
  - AC-TASKS-INTERRUPTED-TASK-INDICATOR-001.2
  - AC-TASKS-INTERRUPTED-TASK-INDICATOR-001.3
  - AC-TASKS-INTERRUPTED-TASK-INDICATOR-001.4
  - AC-TASKS-INTERRUPTED-TASK-INDICATOR-001.5
  - AC-TASKS-INTERRUPTED-TASK-INDICATOR-001.6
  - AC-TASKS-INTERRUPTED-TASK-INDICATOR-001.7
  - AC-TASKS-INTERRUPTED-TASK-INDICATOR-001.8
  - AC-TASKS-INTERRUPTED-TASK-INDICATOR-001.9
  - AC-TASKS-INTERRUPTED-TASK-INDICATOR-001.10
system_design:
  - ../../specs/tasks/system-design/interrupted-task-indicator.md
---

# Task 01 - Keep the warning until continuation

## Summary

An agent can finish restoring an interrupted conversation and wait for input.
Keep the interruption warning visible until that session begins its next prompt.

## In scope

- Store and resolve scoped interruptions with the existing task repository.
  Keep the scalar marker and its client projection for existing readers.
- Move warning resolution to correlated prompt acceptance. Cover ordinary
  prompts, queued delivery, and initial prompts carried by a replacement launch.
  Preserve the existing boot recovery and error cleanup paths.
- Remove a deleted session's entry within its existing transaction. Publish
  committed warning changes through the existing task update path.
- Extend the current desktop and phone restart cases. Update the warning
  paragraph in the public task and workflow guide when the change ships.

## Out of scope

Changes to recovery eligibility, prompt replay, workflow transitions, process
supervision, and the appearance or placement of the shared warning icon.

## Acceptance

1. After a restart and prompt-free restoration, the task still shows its
   existing warning. A rejected, queued-only, or uncertain prompt preserves it.
2. The accepted-dispatch path clears the captured session entry. A stale
   callback and an unaffected sibling preserve it. A task with two affected
   sessions keeps its warning until both entries resolve or are deleted.
3. Desktop and phone show the same warning after reload and clear it without
   reload after an accepted follow-up. The original prompt stays interrupted.

## ASCII UI preview

UI-01, [full preview](plan.md#ascii-ui-preview), covers
AC-TASKS-INTERRUPTED-TASK-INDICATOR-001.3, .4, .6, and .8.

```text
Desktop sidebar        /!\ Fix sync   after restore, before follow-up
Phone switcher drawer  /!\ Fix sync   after restore, before follow-up
After accepted prompt  [status] Fix sync
```

Use the phone task switcher as the mobile exemplar. The existing drawer list
owns scrolling. The same task row opens the chat. Keep the translated label
and the visual distinction between the triangle and ordinary task status.

## Verification

First extend `TestSessionStartKeepsInterruptedMarkerUntilRecovery` with an
assertion that a successful boot retains the warning. Run the Go command
before changing production code. The assertion must fail at marker retention.

| Scenario | Criteria suffix | Evidence |
| --- | --- | --- |
| Startup, periodic interruption, and surviving execution | 001.2, 001.4 | Extend `TestRestartRecoveryRestoresNativeConversationWithoutPrompt` and retain the startup and orphan reconciliation assertions |
| Accepted direct and queued follow-up, refused or undispatched prompt, and replacement-owned prompt | 001.4, 001.8 | `TestTaskInterruptionPromptAcceptance` uses the real dispatch path and checks the committed task update |
| Mixed sibling states, delayed callback, deletion, legacy metadata, and archive guards | 001.2, 001.5, 001.9, 001.10 | `TestTaskInterruptionRepository` and `TestPostgresTaskInterruptionRepository` exercise the scoped writes and deletion transaction |
| Sidebar and phone switcher after restore, reload, and accepted follow-up | 001.3, 001.4, 001.5, 001.6, 001.7, 001.8 | Extend the ordinary restart case in both existing Playwright projects |

For the ordinary restart case, seed the abandoned turn as RUNNING before
restart so startup writes the warning. Let the backend restore it with the
browser closed. After the composer is ready, assert the sidebar triangle and
accessible label. On phone, open the switcher with the existing header button.

Repeat the assertion after reload. Send the existing follow-up message. After
its response settles, assert that the warning is absent from the same client
and the API projection is false. Preserve the existing prompt trace assertions.

Run from the repository root. The PostgreSQL variant uses the existing
`KANDEV_TEST_POSTGRES_DSN` fixture when it is configured. Record the SQLite
and PostgreSQL results separately. The managed Playwright runs build and
operate isolated instances. Run them sequentially.

```bash
(cd apps/backend && go test -trimpath -p 1 ./internal/orchestrator ./internal/task/service ./internal/task/repository/sqlite -run 'Test(.*TaskInterruption|SessionStartKeepsInterruptedMarkerUntilRecovery|RestartRecoveryRestoresNativeConversationWithoutPrompt|ReconcileSessionsOnStartup(Skips|Applies)|Service_OrphanedSessionReconciliationPreservesInterruptedConversation)' -count=1)
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm e2e:run --project chromium tests/session/automatic-restart-recovery.spec.ts -- --grep 'restores an ordinary interrupted session on startup')
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-automatic-restart-recovery.spec.ts -- --grep 'restores an ordinary interrupted session on startup')
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

## Files likely touched

- `apps/backend/internal/orchestrator/service.go` and
  `apps/backend/internal/task/service/archived_session_reconciliation.go`
  for the existing interruption writers.
- `apps/backend/internal/orchestrator/event_handlers_agent.go`,
  `apps/backend/internal/orchestrator/event_handlers_streaming.go`, and
  `apps/backend/internal/orchestrator/task_operations.go` for the boot and
  prompt acceptance paths. Use the existing initial prompt callbacks for
  replacement launches.
- `apps/backend/internal/task/models/models.go` and
  `apps/backend/internal/task/repository/sqlite/` for the marker and its
  conditional writes. Reuse the task row lock in `DeleteTaskSessionWithAttachments`.
- `apps/backend/internal/orchestrator/event_handlers_streaming_test.go`,
  `apps/backend/internal/orchestrator/interrupted_session_restart_test.go`,
  and the task repository interruption tests.
- `apps/web/e2e/helpers/automatic-restart-recovery.ts` and the two existing
  automatic restart specs in `apps/web/e2e/tests/session/`.
- `docs/public/tasks-and-workflows.md`, the recovery paragraph in the
  task and workflow guide.

## Dependencies

None.

## Risks

A scope error can hide a warning for a sibling. Match the session incarnation
and interruption token during the write. A persistence error after acceptance
must keep the prompt's accepted outcome.

## Parallelism

`sequential`

## Inputs

- [Interrupted task warning requirements](../../specs/tasks/requirements/interrupted-task-indicator.md)
- [Interrupted task warning design](../../specs/tasks/system-design/interrupted-task-indicator.md)
- [Existing phone task switcher badge test](../../../apps/web/e2e/tests/session/mobile-acp-prompt-error-continuity.spec.ts)

## Results

- The focused Go tests passed for orchestrator, task service, and SQLite repository.
  The PostgreSQL variant was skipped because `KANDEV_TEST_POSTGRES_DSN` was not configured.
- Frozen `pnpm install` passed.
- The Chromium ordinary-restart case passed, including the backend and Vite builds.
- The mobile-chrome ordinary-restart case passed, including the backend and Vite builds.
- `pnpm typecheck` passed.
- Public documentation tests passed (63 tests), and all 56 published pages validated.
- `git diff --check` passed.

## Review follow-up

- Protected scoped interruption state through create, metadata merge, and
  snapshot replacement. Added regressions for public metadata round trips with
  two warnings, untrusted metadata, malformed legacy deletion, and preservation
  of unrelated raw JSON values.
- Resolved scoped and ownerless legacy warnings only after the captured prompt
  is accepted. Added same-incarnation token rotation and warm model-switch
  replacement coverage; stale callbacks preserve the current warning.
- The recovery `UPDATE ... RETURNING` now includes the session incarnation so
  the recovery settlement can write its scoped marker.
- The desktop and phone restart cases both pass with the affected row visible
  and settled before checking that the warning disappears on the same page.
- Final focused Go tests passed for orchestrator, task service, and SQLite,
  including active-sweep recovery, session recovery identity, and deletion.
  The PostgreSQL variant was skipped because `KANDEV_TEST_POSTGRES_DSN` was not
  configured. The isolated SQLite `final_cancel` admission test passed after a
  broader package attempt stalled in that test.
- Web typecheck, public documentation validation, and both managed Playwright
  cases passed.
- Changed-Go lint passed separately for the orchestrator warning and SQLite
  interruption packages. A combined lint invocation exceeded its package
  analysis timeout after reporting no findings.
