---
spec: docs/specs/tasks/requirements/link-existing-task-github-issue.md
created: 2026-07-31
status: implemented
---

# Implementation Plan: Unlink A Task GitHub Pull Request

## Overview

Add a workspace-authorized unlink contract for one persisted GitHub task-PR
association, broadcast the removal to connected clients, and expose it through
the existing multi-PR tab strip. The backend lands first so frontend state can
consume a stable API and WebSocket contract; desktop and mobile behavior then
share one mutation path and receive focused Playwright coverage.

## Backend

### Durable association detachment

- Update `apps/backend/internal/github/store.go` and
  `apps/backend/internal/github/models.go` so `github_task_prs` gains an
  idempotently migrated nullable `detached_at` timestamp on SQLite and
  Postgres-compatible startup paths.
- Keep detached rows as durable tombstones so a poller or later session does
  not silently rediscover an explicitly removed PR. Active association reads,
  task search, review sources, and automation list paths must filter
  `detached_at IS NULL`.
- Add an exact association lookup by ID and a workspace-scoped detach mutation.
  The mutation stamps only the requested row. Explicit URL linking of the same
  `(task_id, repository_id, pr_number)` clears the tombstone; automatic watch
  discovery does not.
- Preserve task deletion cleanup and existing task/repository/PR uniqueness.
  No GitHub API mutation occurs during detach.

### Service and HTTP contract

- Add a service entry point in
  `apps/backend/internal/github/service_pr_watch.go` (or a focused sibling
  file) that resolves the association, authorizes its stored workspace, and
  detaches it without leaking whether a cross-workspace ID exists.
- Register
  `DELETE /api/v1/github/task-prs/:associationId?workspace_id=<workspaceId>` in
  `apps/backend/internal/github/controller.go`. Return `{ "deleted": true }`
  on success and `404` for unknown or workspace-mismatched associations.
- Publish a typed removal payload containing `workspace_id`, `task_id`, and
  `association_id` only after the detach transaction commits.

### Live update contract

- Add `github.task_pr.deleted` to
  `apps/backend/internal/events/types.go` and
  `apps/backend/pkg/websocket/actions.go`.
- Subscribe it in
  `apps/backend/internal/gateway/websocket/task_notifications.go`; the payload's
  workspace ID keeps routing scoped to clients for the owning workspace.

## Frontend

### API, store, and live updates

- Add `deleteTaskPR(associationId, workspaceId)` to
  `apps/web/lib/api/domains/github-api.ts`.
- Extend the GitHub Zustand slice in
  `apps/web/lib/state/slices/github/{types.ts,github-slice.ts}` with
  `removeTaskPR(taskId, associationId)`, removing only the selected row and
  deleting an empty task bucket.
- Type `github.task_pr.deleted` in `apps/web/lib/types/backend.ts` and handle it
  in `apps/web/lib/ws/handlers/github.ts` so other tabs and windows converge.
- Expose a shared unlink mutation from
  `apps/web/hooks/domains/github/use-task-pr.ts`. It waits for the HTTP success
  before removing local state and reports failure without hiding the PR.

### Multi-PR tab close action

- Refactor `apps/web/components/github/multi-pr-ci-popover.tsx` so the tab label
  and close action are sibling semantic buttons rather than nested buttons.
  Render the close action only while the task has multiple PRs.
- Give each close action an association-specific accessible name, stop its
  pointer/click activation from selecting or opening the PR, disable it while
  its request is pending, and retain the tab with an error toast on failure.
- After a successful removal, select/focus a deterministic adjacent remaining
  tab when the multi-PR surface remains mounted. When two PRs become one, the
  existing parent surface collapses naturally to its single-PR variant and
  restores focus to that surviving trigger.
- Wire the same mutation into both
  `apps/web/components/github/pr-topbar-button.tsx` and
  `apps/web/components/github/pr-status-chip.tsx`.

### Mobile design contract

- **Desktop outcome:** hovering the topbar's multi-PR control shows the existing
  tabbed CI popover; each tab has a compact close action.
- **Mobile entry point:** tap the existing PR status chip to open
  `PRStatusChipMultiDrawer`; no new route or overlay is introduced.
- **Nearest shipped exemplar:** the current
  `PRStatusChipMultiDrawer` + `MultiPRCIPopover` composition supplies the inset
  bottom drawer, fixed header, tab hierarchy, and single internally scrolling
  body.
- **Hierarchy and action:** the selected PR remains the focal content; unlink is
  a secondary action attached to each PR tab. Mobile close targets use an
  actual minimum 44px hitbox and an association-specific accessible label.
- **Presentation rationale:** unlink is a short contextual action inside an
  already-open status surface, so the existing drawer is more direct than a
  second confirmation drawer or route.
- **Shared behavior:** API mutation, pending state, error handling, selected-PR
  fallback, and store removal are shared; only responsive geometry differs.
- **Viewport behavior:** retain the drawer's `min-h-0` internal scroll owner,
  safe-area behavior, focus return, and zero document-level horizontal
  overflow.

## Tests

- **What:** detaching one association is workspace-scoped, persists across a
  store reopen/replayed migration, active lists omit it, siblings remain, and
  explicit relinking restores it.
  **File:** focused tests beside `apps/backend/internal/github/store.go` and
  `service_pr_watch.go`.
  **How:** table-driven store/service tests using the real test database and a
  workspace authorizer.
- **What:** the HTTP endpoint returns success for the owning workspace and
  fails closed for unknown/cross-workspace IDs without mutating rows.
  **File:** `apps/backend/internal/github/controller_test.go`.
  **How:** Gin handler integration tests against the real GitHub store/service.
- **What:** a committed detach broadcasts the typed workspace-scoped removal
  notification.
  **File:**
  `apps/backend/internal/gateway/websocket/task_notifications_test.go`.
  **How:** event-bus notification test.
- **What:** API URL/method, exact store removal, WebSocket removal, pending
  close state, selected fallback, focus behavior, and failure retention.
  **Files:** `apps/web/lib/api/domains/github-api.test.ts`,
  `apps/web/lib/state/slices/github/github-slice.test.ts`,
  `apps/web/lib/ws/handlers/github.test.ts`, and the focused
  `apps/web/components/github/pr-ci-popover.automation.test.tsx`.
  **How:** Vitest unit/component tests with mocked API and store state.

## E2E Tests

- **Scenario:** GIVEN a desktop task with two linked PRs, WHEN the user hovers
  the topbar PR control and closes the older PR tab, THEN only the newer PR
  remains before and after reload.
  **File:** `apps/web/e2e/tests/pr/pr-multi-popover.spec.ts`.
  **What to verify:** close action is reachable, no sibling tab is removed, the
  topbar changes to the single-PR state, and persistence survives reload.
- **Scenario:** GIVEN the same task on a coarse-pointer phone, WHEN the user
  opens the PR status drawer and taps one tab's unlink target, THEN the same
  association is removed with a touch-sized control and no horizontal page
  overflow.
  **File:** `apps/web/e2e/tests/pr/mobile-pr-ci-chip.spec.ts`.
  **What to verify:** drawer entry, accessible close action, minimum hitbox,
  resulting single-PR chip, reload persistence, and document containment.

## Public Documentation

- Update `docs/public/sessions-and-review.md` beside the existing multi-PR
  selector guidance. Explain where unlink is available and that it removes the
  Kandev association only, leaving the remote PR and branch untouched.

## Implementation Waves And Parallel Candidates

The default execution order is sequential in the primary conversation.

- [x] [Task 01: Backend unlink contract](task-01-backend-unlink-contract.md)
- [x] [Task 02: Frontend multi-PR unlink UI](task-02-frontend-multi-pr-unlink.md)
- [x] [Task 03: Desktop and mobile E2E coverage](task-03-pr-unlink-e2e.md)
- [x] [Task 04: Public documentation](task-04-public-documentation.md)

Tasks 01 and 04 are parallel-safe because they own disjoint implementation and
documentation files, but parallel execution still requires explicit user
authorization. Tasks 02 and 03 are sequential after the backend contract and
UI respectively.

## Risks

- Filtering detached rows must cover every task-PR consumer; a missed query
  could leave an old PR visible in search, review, or automation even when the
  topbar is correct.
- Automatic PR-watch discovery and explicit manual relinking need different
  tombstone behavior so background sync cannot resurrect a user removal while
  deliberate relinking still works.
- A close button nested inside the current tab button would produce invalid
  interactive markup and broken keyboard behavior; the tab composition must be
  split without regressing roving tabindex.
- Removing the selected tab can unmount the multi-PR surface when only one PR
  remains, so focus management must tolerate both the retained and collapsed
  cases.
