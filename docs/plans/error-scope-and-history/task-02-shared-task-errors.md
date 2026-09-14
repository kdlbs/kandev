---
id: "02-shared-task-errors"
title: "Show shared errors across task tabs"
status: complete
wave: 2
depends_on:
  - 01-session-history
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-002
acceptance_criteria:
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-002.1
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-002.4
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-002.5
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-002.6
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-002.7
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-002.8
system_design:
  - ../../specs/tasks/system-design/task-launch-failure-recovery.md
---

# Task 02: Show shared errors across task tabs

## Summary

Expose shared failures independently of session errors and render them once in the task shell.

## In scope

- Add optional scope to normalized metadata and wire types. Audit existing task admission, shared preparation, bootstrap, and resume producers.
- Preserve originating session correlation without using it as scope. Legacy sessionless task metadata remains shared.
- Add `task_error` to summary model, projector/rebuild, persistence, boot/HTTP/WS projection, and web state consumers.
- Render a proposed `TaskSharedError` outside tab content, with dialog/Drawer details and existing guarded recovery actions.
- Cover no-session, preview, mobile/tablet, full desktop, maximized panel, and plugin-tab paths.
- Suppress equivalent active controls only by scope and stamp. Preserve unrelated session history and all existing authorization.
- Localize new labels in the five catalogs and generate Traditional Chinese.

## Out of scope

A global notification center, a multi-incident queue, and inferred workspace-wide broadcasts.

## Acceptance

1. A shared failure survives newer session failures, session recovery, reload, and tab switches with unchanged identity.
2. Exactly one shared control surface exists per task view. Same-stamp session controls do not duplicate it.
3. Desktop and phone expose equivalent guarded actions with no overflow and reachable no-session recovery.

## ASCII UI preview

UI-01: Desktop task, shared failure plus an independently recovered session error.

```text
+--------------------------------------------------------+
| Task title / workflow                                  | fixed
| ! Workspace preparation failed. [Recovery details]      | shared
+--------------------------------------------------------+
| Session A | Session B | Plan | Pull request | Files      | tabs
+--------------------------------------------------------+
| Earlier conversation                                 ^ |
| ! Session resume failed. 10:18                        | |
|   Loading timed out. [Recovery details]                | | scrolls
|   Recovered.                                          | |
| Agent: I resumed work...                              v |
+--------------------------------------------------------+
| Message input                                          | fixed
+--------------------------------------------------------+
```

UI-02: Phone task Chat, same two independent failures.

```text
+--------------------------------+
| < Task title       Session A v | fixed task chrome
| ! Workspace preparation failed |
| [Recovery details]             | shared on every view
+--------------------------------+
| Earlier messages             ^ |
| ! Session resume failed      | |
| Loading timed out.           | | transcript scrolls
| Recovered. [Details]         | |
| Agent: I resumed work...     v |
+--------------------------------+
| Message input                  | safe-area clearance
| Chat | Plan | Files | More     | existing navigation
+--------------------------------+
```

UI-03: Session entry states, inside either transcript.

```text
Unresolved: ! Resume failed. [Resume] [Recovery details]
Pending:    ! Resume failed. Resuming... [Details]
Recovered:  ! Resume failed. Recovered. [Details]
Older:      ! Resume failed. [Details]   (no stale actions)
```

Expanded session details wrap inline. Phone actions stack with 44-pixel targets.
A failed new attempt appends a separate error entry. Same-stamp updates retain position.

UI-04: Shared details, desktop dialog / phone inset bottom drawer.

```text
+--------------------------------+
| Workspace preparation       X  |
| Affected repository: owner/repo|
| Safe cause and bounded details |
| [Valid recovery action]        |
+--------------------------------+
```

The phone drawer has safe-area clearance, an internally scrolling body, and 44-pixel controls.
The desktop dialog uses compact controls. Closing details does not clear the shared alert.
Views map to recovery criteria 006.4/6/7/8/9 and task criteria 002.4/5/6/7.
Fixed versus scrolling regions, scope, order, and retained history are requirements.
Copy and spacing are illustrative and must use localized strings and existing tokens.
The two errors coexist only because they represent different failures.

See the [combined plan](plan.md#ascii-ui-preview). Task 02 implements the views within its scope.

## Verification

Run from the repository root. Install workspace dependencies once before the first package command if this checkout lacks them.
Use `/tdd` for changed logic and `/e2e` for browser work. First establish the regression against current behavior.

```bash
(cd apps/backend && go test ./internal/task/statussummary ./internal/task/repository/sqlite ./internal/orchestrator)
(cd apps/web && pnpm exec vitest run lib/session-recovery-presentation.test.ts components/task/task-shared-error.test.tsx components/task/task-page-content.test.tsx components/task/mobile/session-mobile-layout.test.tsx components/task/task-chat-panel.launch-error.test.tsx)
(cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/task/launch-failure-recovery.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-launch-failure-recovery.spec.ts)
```

## Files likely touched

- `apps/backend/internal/task/models/models.go` normalized errors and `internal/task/statussummary/{model,projector,projector_helpers,rebuild}.go` plus tests.
- `apps/backend/internal/orchestrator/task_launch_recovery.go`, launch failure producers, and fenced task metadata operations.
- Corresponding boot/HTTP/WS summary serialization and replacement consumers found through `TaskStatusSummary` references.
- `apps/web/lib/types/task-status-summary.ts`, `lib/session-recovery-presentation.ts`, and task summary state tests.
- Proposed `apps/web/components/task/task-shared-error.tsx` and `task-shared-error.test.tsx`.
- `apps/web/components/task/task-page-content.tsx`, `task-layout.tsx`, preview composition, mobile/tablet layouts, and launch-error context.
- Existing task recovery action components, five locale catalogs, and launch-failure E2E files.

## Dependencies

01-session-history.

## Inputs

Read both owning specs linked from the plan and the September 14 scope decision.
Use the existing launch-recovery tests as the fixture pattern.

## Risks

Preserve failure identity, sanitized details, existing recovery guards, and message ordering under reversed delivery.
The shared repository can contain other edits. Do not revert unrelated changes.

## Parallelism

`sequential`

## Results

Complete on September 14, 2026. Normalized failures now carry explicit session or task scope. Task-owned errors are projected independently as `task_error`, preserve originating session correlation, and render once above task content. Baseline restoration and fallback derivation route explicit scope before applying legacy session-ID inference, so clearing task metadata also clears the task-owned active error without a session refresh. Desktop details use a dialog. Phone details use a safe-area-aware, internally scrolling drawer with touch-sized controls. Existing recovery guards and legacy sessionless task errors remain supported.

Validation passed:

- Backend status-summary, SQLite, and orchestrator tests passed.
- Frontend summary, recovery presentation, shared-shell, task layout, mobile layout, and launch-entry tests passed.
- Frontend typecheck, lint, i18n checks, and Vite build passed.
- Cold-projector clearing and composed-panel single-representation regressions passed.
- Chromium and Mobile Chrome launch-recovery suites passed with 4 and 3 tests respectively.
