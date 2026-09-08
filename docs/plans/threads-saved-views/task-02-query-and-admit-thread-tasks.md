---
id: "02-query-and-admit-thread-tasks"
title: "Query and admit Threads tasks"
status: done
wave: 2
depends_on:
  - "01-persist-thread-views"
plan: "plan.md"
requirements:
  - REQ-UI-THREADS-SAVED-VIEWS-002
  - REQ-UI-THREADS-SAVED-VIEWS-003
acceptance_criteria:
  - AC-UI-THREADS-SAVED-VIEWS-002.1
  - AC-UI-THREADS-SAVED-VIEWS-002.3
  - AC-UI-THREADS-SAVED-VIEWS-002.4
  - AC-UI-THREADS-SAVED-VIEWS-002.5
  - AC-UI-THREADS-SAVED-VIEWS-002.6
  - AC-UI-THREADS-SAVED-VIEWS-002.7
  - AC-UI-THREADS-SAVED-VIEWS-002.8
  - AC-UI-THREADS-SAVED-VIEWS-002.9
  - AC-UI-THREADS-SAVED-VIEWS-002.10
  - AC-UI-THREADS-SAVED-VIEWS-002.11
  - AC-UI-THREADS-SAVED-VIEWS-002.13
  - AC-UI-THREADS-SAVED-VIEWS-003.1
  - AC-UI-THREADS-SAVED-VIEWS-003.2
  - AC-UI-THREADS-SAVED-VIEWS-003.3
  - AC-UI-THREADS-SAVED-VIEWS-003.4
  - AC-UI-THREADS-SAVED-VIEWS-003.5
  - AC-UI-THREADS-SAVED-VIEWS-003.6
  - AC-UI-THREADS-SAVED-VIEWS-003.7
  - AC-UI-THREADS-SAVED-VIEWS-003.8
  - AC-UI-THREADS-SAVED-VIEWS-003.9
  - AC-UI-THREADS-SAVED-VIEWS-003.10
  - AC-UI-THREADS-SAVED-VIEWS-003.11
system_design:
  - ../../specs/ui/system-design/threads-saved-views.md
  - ../../specs/ui/system-design/threads-conversation-deck.md
  - ../../specs/platform/system-design/viewport-bounded-session-delivery.md
---

# Task 02: Query and Admit Threads Tasks

## Summary

Build a pure, bounded task query for Threads. Apply scope, filters, sort, deep
links, and the column limit before the board mounts task shells.

## In scope

- Project candidates from active-workspace workflow snapshots and compact task
  summaries.
- Add the primary agent profile ID to the bounded task DTO and map agent and
  executor identity into snapshot state.
- Add table-driven filters for every specified Threads dimension.
- Add deterministic comparators with task-ID tie-breakers.
- Return matching, admitted, and hidden counts.
- Reserve one capped slot for a valid deep-link target.
- Reset stable order for query changes and Reapply sort.
- Preserve order for live events and fill open slots from sorted matches.
- Remove the global active-workflow constraint from Threads.
- Pass admitted candidates only to `ThreadsBoard`.

## Out of scope

- Filter-editor UI and saved-view controls.

## Acceptance

- The canonical query has the same eligible-task outcome as the current deck.
- Selected IDs survive temporary task ineligibility.
- All clauses use AND and values inside one clause use OR.
- A limit of 3 mounts no more than three task shells.
- A hidden valid deep-link target is admitted without changing saved state.
- Live activity does not reorder surviving columns.

## Verification

Write failing projection, filter, comparator, admission, and stable-order tests
first. Then run:

```bash
(cd apps && pnpm --filter @kandev/web test -- --run lib/threads/thread-view-query.test.ts lib/threads/active-threads.test.ts hooks/domains/threads/use-stable-thread-order.test.ts components/threads/threads-board.test.tsx app/threads/threads-page-client.test.tsx)
(cd apps/web && pnpm run typecheck)
```

## Files likely touched

- `apps/web/lib/threads/thread-view-query.ts`
- `apps/web/lib/threads/thread-view-query.test.ts`
- `apps/web/lib/threads/active-threads.ts`
- `apps/web/lib/state/slices/kanban/kanban-mappers.ts`
- `apps/web/hooks/domains/threads/use-stable-thread-order.ts`
- `apps/web/app/threads/threads-page-client.tsx`
- `apps/web/components/threads/threads-board.tsx`
- `apps/web/components/threads/threads-board.test.tsx`
- `apps/backend/internal/task/dto/dto.go`
- `apps/backend/internal/task/handlers/task_http_handlers.go`
- `apps/backend/internal/backendapp/boot_state.go`

## Dependencies

- Task 01 supplies normalized view and draft state.

## Risks

- Do not fetch task or session detail to evaluate a filter.
- Keep the viewport detail-window rules inside the admitted task set.

## Parallelism

`sequential`
