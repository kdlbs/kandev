---
spec: docs/specs/ui/requirements/sidebar-last-activity-sort.md
created: 2026-08-17
status: completed
---

# Implementation Plan: Sidebar Last Activity Sort

## Overview

Add a durable task activity timestamp to the bounded task-status summary. Keep
summary freshness separate, then expose the activity timestamp through a new
saved-view sort on desktop and mobile.

The work also closes the observed GitHub refresh loop. REST and batched GraphQL
status snapshots must converge on one semantic task-PR value, and diagnostics
must name fields that cause a real update.

## Confirmed current behavior

- `apps/web/components/task/task-session-sidebar-item.ts` maps summary
  `updated_at` to `TaskSwitcherItem.updatedAt`.
- `apps/web/lib/sidebar/apply-view.ts` uses `updatedAt` for the saved-view
  `updatedAt` comparator.
- `apps/web/components/task/task-item-stats-row.tsx` renders the same value as
  the row's relative time.
- `apps/backend/internal/task/statussummary/projector.go` stamps
  `TaskStatusSummary.UpdatedAt` when any semantic summary field changes.
- The projector subscribes to `github.task_pr.updated`, so provider status can
  refresh the visible time of an idle task.
- Sidebar views persist arbitrary sort strings through portable user settings.
  Frontend `KNOWN_SORT_KEYS` controls which keys survive migration.
- Desktop and phone surfaces share `SortPicker`, `applySort`, saved-view state,
  and user-settings synchronization.

## Architecture

### Timestamp ownership

Extend `TaskStatusSummary` with semantic `LastActivityAt`. Keep `UpdatedAt` as
transport and projection freshness.

Add a narrow `TaskActivityRepository` to the task service. Its batch query
returns the maximum of:

- `tasks.created_at` and `tasks.updated_at`
- user-authored `task_session_messages.created_at`
- non-reserved user-owned `queued_messages.queued_at`
- `task_session_turns.started_at` and non-null `completed_at`

The query accepts task IDs, returns one map, and uses the existing task/message
and turn indexes. Add an index only if query-plan evidence shows a missing
leading task key on a production-sized fixture.

### Live projection and repair

Add `lastActivityAt` to projector state and persisted semantic JSON. Rehydrate
it from the stored summary and a narrow activity loader.

Subscribe to `task.created`, `task.state_changed`, `turn.started`, and
`turn.completed` in addition to the existing task, message, and queue events.
Apply timestamps as follows:

- `task.created`, `task.updated`, and `task.state_changed`: payload
  `updated_at`, with `created_at` as the creation fallback
- `message.added`: `created_at` only for `author_type=user`
- successful user-owned queue admission: `queued_at`
- `turn.started`: `started_at`
- `turn.completed`: `completed_at`

Use a monotonic maximum. Foreground-activity task events carry the task row's
unchanged timestamp and therefore cannot advance task activity.

Task-list reconciliation batch-loads activity for all requested tasks. It
repairs missing fields and preserves a newer stored value during compare-and-set
retries. The complete replacement summary continues to use the existing event.

### GitHub refresh stability

Normalize GraphQL check-rollup states to the documented `success`, `failure`,
`pending`, or empty contract before `SyncTaskPR` compares fields. Exercise the
same semantic fixture through REST feedback and batched GraphQL conversion.

Refactor the `SyncTaskPR` comparison into a helper that returns changed field
names. Publish only when that list is non-empty. Log the names at debug level
with task and PR identity, without old or new values.

### Saved-view and row behavior

Add `lastActivityAt` to `SortKey`, `KNOWN_SORT_KEYS`, `SORT_OPTIONS`, and the
comparator table. Thread `lastActivityAt` from `status_summary` into
`TaskSwitcherItem`, with task update and creation fallbacks.

When the effective view uses this key, rows display activity time. Other views
retain the existing time selection. Keep stable input order for equal values.

Add localized **Last activity** copy to English, Portuguese, Simplified
Chinese, Hong Kong Chinese, Taiwan Chinese, and pseudo catalogs. Run the
Traditional Chinese generator instead of translating both variants by hand.

## Mobile design contract

- **Outcome and entry:** desktop opens the Tasks section view editor. Phones
  open `SessionTaskSwitcherSheet`, then the existing filter gear.
- **Exemplar:** keep
  `apps/web/components/task/mobile/session-task-switcher-sheet.tsx` and the
  shared `SidebarFilterPopover` composition.
- **Hierarchy and presentation:** the sort remains a short temporary choice in
  the existing selector. No route, drawer, or nested overlay is added.
- **Interaction:** the existing selector row is the touch target. Selection
  updates the shared draft and live task order.
- **Scroll and geometry:** the task list remains the only vertical scroll body.
  The existing drawer and portaled selector remain inside the Pixel 5 viewport
  with no document horizontal overflow.
- **Shared state:** saved-view state, activity mapping, comparator, direction,
  persistence, and rollback are shared between desktop and mobile.

## Tests

Use Red-Green-Refactor for each implementation task. Run each focused test
before production edits and record the expected failure. Run the same test
after the smallest implementation, then refactor while it remains green.

### Backend

- GitHub conversion tests cover every GraphQL rollup state and prove an
  equivalent REST/GraphQL sequence emits no second task-PR update.
- Activity repository tests cover tasks without sessions, queued user prompts,
  active turns, completed turns, multiple sessions, and batched task IDs.
- Summary model tests cover semantic JSON, equality, compatibility with absent
  activity, and monotonic timestamps.
- Projector tests cover every included source, including state-only task
  transitions. They prove focus-adjacent status, PR, Git, queue, agent-message,
  and older replay events do not advance activity.
- Rebuild tests cover field backfill, no N+1 reads, compare-and-set retry, and
  preservation of a newer stored timestamp.

### Frontend

- Comparator tests cover ascending, descending, equal times, and fallback.
- Migration, wire, hydration, and live user-settings tests preserve
  `lastActivityAt` in saved views and drafts.
- Sidebar mapping and rendered-row tests select activity time only for the new
  sort.
- Locale checks cover the new label in every required catalog.

### E2E

- Desktop creates a view, selects **Last activity**, verifies order and row
  time, saves it, reloads, and verifies persistence.
- The backend regression proves a provider status refresh does not change the
  activity timestamp. The desktop flow verifies that resulting sort order.
- Mobile selects the same option from the task-switcher drawer and verifies
  order, persistence, touch reachability, viewport containment, scroll
  ownership, and zero horizontal overflow.

## Verification commands

```bash
make -C apps/backend test lint build
cd apps && pnpm install --frozen-lockfile
cd apps && pnpm --filter @kandev/web exec vitest run lib/sidebar/apply-view.test.ts lib/state/slices/ui/ui-slice-migration.test.ts lib/state/slices/ui/sidebar-view-wire.test.ts components/task/task-session-sidebar-item.test.ts
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint
cd apps/web && pnpm run i18n:zh-hant && pnpm run i18n:check && pnpm run i18n:ratchet
cd apps/web && pnpm e2e:run tests/task/sidebar-filter.spec.ts -- --grep "last activity"
cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/task/mobile-sidebar-views.spec.ts -- --grep "last activity"
```

## Implementation waves

- [Task 01: Stabilize GitHub task-PR refreshes](task-01-stabilize-github-pr-refresh.md), wave 1
- [Task 02: Add durable activity reconstruction](task-02-add-activity-reconstruction.md), wave 1
- [Task 03: Project task activity live](task-03-project-live-task-activity.md), wave 2, after Task 02
- [Task 04: Add the saved-view sort](task-04-add-sidebar-sort.md), wave 3, after Task 03
- [Task 05: Prove desktop and mobile behavior](task-05-prove-sidebar-activity-sort.md), wave 4, after Tasks 01 and 04

This package does not authorize implementation subagents. Implementation stays
in the user-controlled primary session.

## Implementation result

All five tasks are complete. The implementation includes durable activity
reconstruction, live bounded projection, GitHub refresh normalization, the
desktop/mobile saved-view sort, localized catalogs, and production-build
browser coverage.

Targeted checks passed during implementation:

- Backend focused GitHub, activity repository, summary, rebuild, projector, and
  backend-app tests.
- Frontend focused tests, typecheck, lint, i18n completeness, and i18n ratchet.
- One desktop Chromium E2E test and one mobile Chromium E2E test, both through
  the managed production-build runner.

The full backend test, lint, and build matrix passed as the final pre-PR gate.

## Risks and boundaries

- `updated_at` and `last_activity_at` must remain separate in semantic equality,
  persistence, DTOs, and UI mapping.
- Event timestamps can arrive out of order. Every live and rebuild path needs a
  monotonic maximum.
- Counting all agent messages recreates high-frequency task-list traffic.
  Turn start and completion are the bounded agent milestones.
- Existing saved views and default view creation must not change.
- The GitHub normalization test must exercise both real conversion paths. A
  handcrafted `PRStatus` pair does not detect future adapter drift.
- Public documentation does not describe sidebar sort choices today. No public
  documentation update is required.
