---
spec: docs/specs/integrations/requirements/github-pr-merge-queue.md
created: 2026-08-22
status: done
---

# Implementation Plan: GitHub PR Merge Queue Status

## Overview

Extend the existing batched GitHub PR sync to observe and persist merge queue
state, position, and estimated merge duration, then carry the queued semantic
through the bounded task projection and the metadata through the existing PR
status surfaces. Reuse the current task indicator, compact popover/drawer, and
review detail compositions; no new polling path, endpoint, or queue-management
action is needed.

## Backend

### GitHub queue observation

- Update `apps/backend/internal/github/graphql.go` so `batchedPRResult` decodes
  `mergeQueueEntry { state position estimatedTimeToMerge }`, `prFieldsBlock`
  requests it for numbered and branch queries, and `convertBatchedPRResult`
  normalizes the provider enum while retaining GitHub's one-based position and
  optional estimate in seconds.
- Update `apps/backend/internal/github/models.go` so `PRStatus` carries the
  normalized queue state, position, and estimate plus one internal populated
  guard, and `TaskPR` carries the three persisted queue fields.
- Keep `newPRStatus` in `apps/backend/internal/github/client_helpers.go`
  queue-unaware. Its populated guard remains false, so REST and `gh pr view`
  feedback reads cannot erase a GraphQL observation.

### Persistence and synchronization

- Add `merge_queue_state TEXT NOT NULL DEFAULT ''`, nullable
  `merge_queue_position INTEGER`, and nullable
  `merge_queue_estimated_time_to_merge_seconds INTEGER` to the fresh
  `github_task_prs` schema and the idempotent existing-database migration in
  `apps/backend/internal/github/store.go`.
- Thread the field through `taskPRColumns`, `taskPRColumnsQualified`,
  `CreateTaskPR`, `ReplaceTaskPR`, `RestoreTaskPR`, and `UpdateTaskPR` so every
  association path round-trips the complete queue entry and schema replay
  remains safe.
- Extend `taskPRSyncState`, `prepareTaskPRSyncState`,
  `taskPRChangedFields`, and `SyncTaskPR` in
  `apps/backend/internal/github/service_pr_watch.go`. A populated GraphQL
  result atomically replaces the stored queue state, position, and estimate; an
  unpopulated read preserves all three; and any merged or closed lifecycle
  state clears all three.
- Extend `associateTaskPRRequest` and `buildTaskPRFromRequest` in
  `apps/backend/internal/github/mock_controller.go` so E2E fixtures can seed a
  queue state, position, and estimate and publish the normal
  `github.task_pr.updated` event.

### Bounded task status

- Add merge queue state to the PR observation and rebuild input in
  `apps/backend/internal/task/statussummary`, including live event projection,
  restart rebuild, and aggregate-state derivation.
- Add `queued` to the bounded PR aggregate states with a rank between `ready`
  and `passing`: a more attention-worthy sibling still wins for multi-PR
  tasks, while a single queued PR retains the queue color.
- Pass `TaskPR.MergeQueueState` through
  `apps/backend/internal/backendapp/status_summary_adapter.go` so restart
  hydration and live events converge on the same aggregate state.

## Frontend

### Types and semantic color

- Add the normalized `MergeQueueState` union and all three TaskPR queue fields
  to `apps/web/lib/types/github.ts`.
- Add the shared `queued` semantic to
  `apps/web/components/integrations/change-request-task-status-color.ts`, using
  the exact `text-[#966600]` GitHub merge-queue color. This color is distinct
  from the current yellow CI-in-progress state. Add the same queued tone to the
  shared task-summary presentation. Preserve the existing worst-status
  aggregation for tasks with several open PRs.
- Update `apps/web/components/github/pr-task-icon.tsx` and
  `apps/web/components/github/pr-status-chip.tsx` so an open queued PR exposes
  the queue color/status and a dedicated glyph before non-terminal review,
  check, and mergeability colors. Terminal states still win.

### Hover, compact status, and review detail

- Extend the shared summary-row data in
  `apps/web/components/integrations/change-request-task-status-summary.tsx`
  with an optional localized detail line. Use it from the merge row in
  `apps/web/components/github/pr-task-status-summary.tsx` to show queue state,
  one-based position, and the available estimate. Recognized values
  distinguish queued, awaiting checks, mergeable, unmergeable, and locked;
  unknown non-empty values use generic queued copy.
- Add a reusable queue-status notice under
  `apps/web/components/github/` that formats position and GitHub's estimate in
  localized minute/hour units. Values below 60 seconds use a localized
  sub-minute label; larger values round up to the next whole minute. Render it
  from `pr-mergeability-row.tsx` and `pr-detail-panel.tsx`. This places the same
  queue information in the compact desktop popover, phone status drawer, and
  full PR detail panel without creating another data fetch.
- Add queue labels, descriptions, position, and pluralized duration copy to all
  five `apps/web/src/locales/*/github.json` catalogs. Generate `zh-hk` and
  `zh-tw` with `pnpm run i18n:zh-hant`.

### Mobile design contract

- **Desktop outcome:** task and compact PR indicators use the queue color;
  hover/popover content and PR detail show translated queue state, position,
  and the available estimate.
- **Mobile entry points:** tap the existing composer PR status chip for its
  drawer, or open the existing full-height Review destination.
- **Nearest shipped exemplars:** `PRStatusChipDrawer` contributes the inset,
  internally scrolling status drawer; the existing mobile Review surface
  contributes direct navigation and full-height review geometry.
- **Information hierarchy:** queue status replaces the mergeability line for
  an active entry and sits below review/check information in compact status;
  position and estimate form one subordinate metadata line. The detail notice
  stays below the PR header and above review content.
- **Presentation and rationale:** retain the current drawer and direct Review
  navigation because this is content-only status inside already established
  surfaces, not a new temporary choice or primary destination.
- **Geometry and shared logic:** no scroll owner, safe-area, breakpoint, or
  touch-target changes. Queue derivation, copy, and state are shared between
  viewports; only the existing responsive wrappers differ.

## Tests

- **What:** GraphQL selection and decoding distinguish every known queue enum,
  position, a present or absent estimated duration, `null`, and an unknown
  future state. **File:**
  `apps/backend/internal/github/graphql_merge_queue_status_test.go`. **How:** query snapshot and
  table-driven conversion tests.
- **What:** sync atomically persists queue entry transitions and metadata,
  preserves all fields across queue-unaware feedback reads, clears them on an
  authoritative `null`, and clears them for terminal PRs. **File:**
  `apps/backend/internal/github/service_pr_merge_queue_status_test.go`. **How:**
  focused service tests against the real store.
- **What:** fresh schema, same-database replay, and every TaskPR write/read path
  round-trip the complete queue entry. **Files:**
  `apps/backend/internal/github/store_taskpr_schema_drift_test.go` and
  `apps/backend/internal/github/store_merge_queue_status_test.go`. **How:** real
  SQLite store tests.
- **What:** live projection and restart rebuild derive the same `queued`
  aggregate and preserve multi-PR attention ranking. **Files:**
  `apps/backend/internal/task/statussummary/projector_test.go`,
  `apps/backend/internal/task/statussummary/rebuild_test.go`, and
  `apps/backend/internal/backendapp/status_summary_adapter_test.go`. **How:**
  table-driven projection and adapter tests.
- **What:** queue membership maps to `#966600` on the task icon and compact
  chip. The hover merge row, compact queue notice, and detail notice show the
  translated status and metadata. Terminal and non-queued states retain their
  existing behavior.
  **Files:**
  `apps/web/components/github/pr-task-icon.test.ts`,
  `pr-task-status-summary.test.ts`, `pr-status-chip.test.tsx`,
  `pr-merge-queue-status.test.tsx`, `pr-mergeability-row.test.tsx`, and
  `pr-detail-panel.test.tsx`. **How:** pure formatter and focused component
  tests, including absent, sub-minute, minute, and hour estimate boundaries.

## E2E Tests

- **Scenario:** GIVEN an eligible queue-required PR, WHEN a desktop user
  activates `Merge PR`, THEN the merge API returns `{"status":"queued"}`,
  the success notification appears, and the accepted state suppresses the
  duplicate action. **File:** the action test in
  `apps/web/e2e/tests/pr/pr-merge-queue.spec.ts`.
- **Scenario:** GIVEN a linked open PR with an active merge queue entry, WHEN a
  desktop user views the task and PR detail, THEN the task indicator has queued
  semantics, its hover summary and compact popover show queue state, position,
  and estimated merge duration, and the detail notice shows the same data.
  **File:** the display test in
  `apps/web/e2e/tests/pr/pr-merge-queue.spec.ts`.
- **Scenario:** GIVEN an eligible queue-required PR on a phone viewport, WHEN
  the user opens Review and activates the action, THEN the queued notification
  appears, the action target is at least 44px high, and the document has no
  horizontal overflow. **File:** the action test in
  `apps/web/e2e/tests/pr/mobile-pr-merge-queue.spec.ts`.
- **Scenario:** GIVEN the same queued PR on a phone viewport, WHEN the user
  opens the PR status drawer and the Review destination, THEN both expose queue
  status, position, and the available estimate without hover, and the document
  retains no horizontal overflow. **File:** the display test in
  `apps/web/e2e/tests/pr/mobile-pr-merge-queue.spec.ts`.

## Verification Results

Passed:

- `cd apps/backend && go test -tags fts5 ./internal/github/... ./internal/task/statussummary/... ./internal/backendapp` passed 2,193 tests across 3 packages. The focused RED run recorded the expected missing-contract compile failures, and the focused GREEN run passed the queue precedence tests.
- `cd apps/web && pnpm test -- components/github/pr-task-icon.test.ts components/github/pr-task-status-summary.test.ts components/github/pr-status-chip.test.tsx components/github/pr-merge-queue-status.test.tsx components/github/pr-mergeability-row.test.tsx components/github/pr-detail-panel.test.tsx` passed 160 tests across 6 files.
- `cd apps/web && pnpm run typecheck` and `cd apps/web && pnpm run lint` passed. `cd apps/web && pnpm run i18n:check` passed with 7,223 referenced keys, 8,779 English entries, 48 orphans, and all five required catalogs complete.
- `node scripts/validate-public-docs.test.mjs` passed 61 tests, and `node scripts/validate-public-docs.mjs` validated 41 published pages. `docs/public/integrations.md` documents queue state, position, and estimate surfaces.
- A fresh `cd apps/web && pnpm e2e:run tests/pr/pr-merge-queue.spec.ts` build completed the backend, Vite production bundle, and plugin. The final desktop run passed 2 tests, including the merge-action and queue-display scenarios, and `pnpm e2e:run --no-build --project mobile-chrome tests/pr/mobile-pr-merge-queue.spec.ts` passed 2 matching tests.
- Previously captured display assets remain valid: `apps/web/.pr-assets/pr-merge-queue--desktop-pr-merge-queue-status.png` and `apps/web/.pr-assets/mobile-pr-merge-queue--mobile-pr-merge-queue-status.png`. Both were inspected, compressed with the `pngquant-bin` fallback, and preserved for PR media publication; action coverage is behavioral and adds no capture asset.
- `cd apps/web && pnpm e2e:clean` removed E2E results, reports, PR assets, and shard logs. The E2E scenarios use mock GitHub state and created no external GitHub writes.

## Implementation Waves And Parallel Candidates

Wave 1:
- [x] [Task 01: Backend queue-state projection](task-01-backend-queue-state.md)

Wave 2:
- [x] [Task 02: Frontend queued PR status](task-02-frontend-queued-status.md)

Wave 3:
- [x] [Task 03: Responsive merge-queue E2E](task-03-responsive-merge-queue-e2e.md)

Execution is sequential in the primary conversation unless the user explicitly
authorizes subagents.
