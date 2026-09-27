---
created: 2026-09-27
status: done
requirements:
  - REQ-UI-SIDEBAR-LAST-ACTIVITY-SORT-001
system_design:
  - ../../specs/ui/system-design/sidebar-last-activity-sort.md
legacy_specs: []
---

# Implementation Plan: Preserve Activity During Recovery Cleanup

## Overview

Keep an idle task's last activity time stable when startup clears recovery markers left by a completed manual workflow move. One focused backend work order adds a failing restart regression, preserves the task update timestamp during marker cleanup, and verifies both live projection and durable reconstruction. The existing sidebar consumes the corrected value without a UI change.

## Root cause and reproduction

At the 2026-09-27 restart, 12 tasks carried `manual_move_lifecycle_completed`. The startup lifecycle sweep processed all 12. `ClearManualMoveLifecycleMarkersIfCompleted` set `tasks.updated_at` to the current time while removing the marker. Its `task.updated` event made the status-summary projector advance `last_activity_at`, and reconstruction would also read the new task timestamp. Four sampled sidebar tasks had matching task and activity timestamps at 10:41:48 UTC; two sampled tasks had no new message or turn.

Reproduce in a test by creating a task and status summary with an older activity time, adding a completed manual-move marker, then running `recoverTaskLifecycleToken`. Before correction, both `tasks.updated_at` and `last_activity_at` advance at cleanup. The failing regression belongs in `apps/backend/internal/orchestrator/lifecycle_sweep_startup_test.go`, backed by a repository contract in `apps/backend/internal/task/repository/sqlite/task_metadata_cas_test.go`.

## Scope

### In scope

- Preserve the task update timestamp during completed manual-move marker cleanup.
- Retain marker convergence, feeder reconciliation, and the generation guard against concurrent newer moves.
- Verify live summary activity and durable activity reconstruction stay stable on restart.

### Out of scope

- Changing the **Updated** sort, other metadata writes, manual-move activity itself, or sidebar rendering.
- Automatically rewriting timestamps already advanced on existing installations without a trustworthy historical task-edit record.

## Technical approach

In `apps/backend/internal/task/repository/sqlite/task.go`, remove the `updated_at` assignment from both dialect branches of `ClearManualMoveLifecycleMarkersIfCompleted`. Keep the conditional `updated_at = completedAt` and completed-marker predicates. The orchestrator continues to publish `task.updated` after a successful clear; its unchanged source timestamp leaves `last_activity_at` unchanged. A real task edit or newer move still writes a new generation and remains protected from a stale clear.

Do not suppress the event only in the projector: `LoadTaskLastActivity` reads `tasks.updated_at`, so a rebuild would reintroduce the false activity. No schema, wire, localization, or frontend change is required.

## Tests

| Acceptance criterion | Evidence |
| --- | --- |
| `AC-UI-SIDEBAR-LAST-ACTIVITY-SORT-001.9` | New orchestrator restart integration test for unchanged task and summary activity; SQLite and PostgreSQL repository contract for marker clearing, reconstructed activity, and stale-generation refusal. |
| `AC-UI-SIDEBAR-LAST-ACTIVITY-SORT-001.3` and `.7` | Existing sidebar sort unit and desktop Playwright coverage confirms the corrected summary value is ordered and displayed. |

Existing `TestRecoveryDoesNotClearPendingMarkerFromNewManualMove` and `TestRecoveryRetriesWhenNewManualMoveAlreadyCompleted` guard concurrency behavior. Test a real subsequent task update separately so preserving cleanup time does not suppress qualifying activity.

## E2E tests

Run the existing desktop Chromium scenario in `apps/web/e2e/tests/task/sidebar-filter.spec.ts` and the mobile Chromium scenario in `apps/web/e2e/tests/task/mobile-sidebar-views.spec.ts` for the displayed last-activity order and time (`AC-UI-SIDEBAR-LAST-ACTIVITY-SORT-001.3`, `.7`). The mobile scenario uses the existing task-switcher drawer and shared sort state. The restart trigger in `.9` is covered by the backend integration test because the browser flow does not own lifecycle recovery. No rendered UI changes are planned.

## Work orders

- [x] [Task 01: Preserve activity during completed move cleanup](task-01-preserve-activity-during-cleanup.md)

## Verification results

Implementation and work-order checks completed. The repository tests cover
SQLite marker cleanup and reconstruction; the PostgreSQL contract is present
but was skipped because `KANDEV_TEST_POSTGRES_DSN` was unset. The orchestrator
regression passed through the real status-summary projector, and the existing
desktop and mobile last-activity E2E scenarios passed.

## Risks

- Removing the timestamp write must not weaken the conditional clear against a concurrent newer manual move. Keep and test the existing generation predicate in SQLite and PostgreSQL.
- Previously advanced task and summary timestamps cannot be rolled back safely from current records. This package prevents recurrence; any historical repair needs a separate evidence-backed plan.
