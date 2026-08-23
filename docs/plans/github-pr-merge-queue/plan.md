---
spec: docs/specs/github-pr-merge-queue/spec.md
created: 2026-08-17
status: complete
---

# Implementation Plan: GitHub PR Merge Queue

## Overview

Extend the existing GitHub merge boundary to use GitHub's asynchronous,
queue-aware merge request and return a typed merged-or-queued outcome. Observe
the resulting merge-queue entry through the existing GraphQL status sync,
persist its state, one-based position, and optional estimated duration, and
project it through the existing task and PR status surfaces. Reuse the typed
outcome and the persisted status in the existing PR merge button across the
desktop detail, compact status, and mobile Review surfaces, with targeted unit
and E2E proof.

## Backend

### Provider client contract

- Update `apps/backend/internal/github/client.go` so the merge operation returns
  a typed outcome rather than only an error.
- Update `apps/backend/internal/github/gh_client.go` and
  `apps/backend/internal/github/pat_client.go` to call
  `PUT /repos/:owner/:repo/pulls/:number/merge-async` with
  `merge_action=default`, preserve the selected `merge_method`, and normalize
  GitHub's accepted, already-queued, and already-merged response shapes.
- Update `apps/backend/internal/github/mock_client.go` and
  `apps/backend/internal/github/noop_client.go` for the same contract so local
  and E2E behavior remains deterministic.

### Service and HTTP response

- Update `apps/backend/internal/github/service_pr.go` to propagate the typed
  outcome while retaining workspace scope checks, personal-write credential
  routing, merge-method fallback, and cache invalidation.
- Update `apps/backend/internal/github/controller.go` to return
  `{"status":"merged"}` or `{"status":"queued"}` and retain the existing
  operational-auth and provider-error mapping.

### Queue observation, persistence, and bounded projection

- Extend the batched GraphQL pull-request selection and normalized `PRStatus`
  with `mergeQueueEntry { state position estimatedTimeToMerge }`. A populated
  GraphQL result replaces all three queue fields atomically; queue-unaware REST
  and `gh pr view` reads preserve the last complete observation, while an
  authoritative null or terminal lifecycle state clears it.
- Add the queue state, one-based position, and optional estimate in seconds to
  every `TaskPR` schema, migration, write, restore, and update path. Extend
  the mock controller so integrated tests can seed the same contract.
- Carry queue membership through live and restart-built task status summaries
  as the bounded `queued` state. Terminal states precede queue membership;
  queue membership precedes other non-terminal states for one PR, while the
  existing aggregate ranking keeps a failing sibling above a queued sibling.

## Frontend

### Queue-aware merge action

- Update `apps/web/lib/api/domains/github-pr-api.ts` and the re-exporting API
  module to type the merged-or-queued response.
- Update `apps/web/components/github/pr-task-icon.tsx` with a tested eligibility
  predicate that permits a queue-required branch only after explicit successful
  checks and satisfied reviews, while continuing to reject drafts, conflicts,
  behind branches, changes requested, and incomplete review/check gates.
- Update `apps/web/components/github/pr-merge-button.tsx` to render for direct or
  queued eligibility, show the outcome-specific notification, suppress repeat
  submission after acceptance, and refresh the PR state.
- Add the new action and outcome copy to all locale catalogs under
  `apps/web/src/locales/*/github.json`; generate the Traditional Chinese pair
  through the repository script rather than translating them independently.

### Queue status color and surfaces

- Add the normalized queue fields to the web `TaskPR` contract and the shared
  queued semantic color `text-[#966600]`. Terminal colors remain first; an
  active queue entry is second, so hydrated failure, draft, dirty, or behind
  fields cannot overwrite queue presentation. Multi-PR aggregation still ranks
  a failing sibling above a queued sibling.
- Extend the task hover summary, compact desktop popover, phone status drawer,
  and PR detail panel with translated queue state, one-based position, and an
  optional estimate rounded up to localized whole minutes. Missing estimates
  omit only the estimate line. Recognized provider states retain their labels;
  future non-empty states use generic `Queued` copy.
- Keep queue derivation shared across desktop and mobile. Mobile display uses
  the existing drawer and Review destination; the mobile merge-action test
  separately verifies the existing 44px action target.

### Mobile design contract

- Desktop outcome: the existing PR detail header and compact status surface
  expose one merge action whose result says merged or queued.
- Mobile entry point: the task's existing Review bottom-navigation destination.
- Nearest shipped exemplar: the existing full-height mobile Review surface and
  `PRMergeButton`; retain its single internal scroll owner and shared PR state.
- Information hierarchy: PR status and blockers remain above the primary merge
  action; the action is the terminal operation for an eligible PR.
- Presentation: retain direct navigation to the full-height Review surface
  because PR review is dense primary content, not a temporary picker.
- Geometry: keep the existing dynamic-height/safe-area behavior, avoid document
  horizontal overflow, and preserve a touch target at least 44px high on phone.
- Shared logic: eligibility, mutation, result handling, and refresh remain
  shared; only the existing responsive composition differs.

## Tests

- **What:** PAT and `gh` clients send `merge-async`, `merge_action=default`, and
  the chosen merge method, and normalize direct, queued, and idempotent results.
  **Files:** `apps/backend/internal/github/pat_client_writes_test.go`,
  `apps/backend/internal/github/gh_client_commands_test.go`. **How:** focused
  HTTP/command capture tests with representative GitHub responses.
- **What:** service preserves method resolution and cache invalidation while
  returning the provider outcome. **File:**
  `apps/backend/internal/github/service_pr_test.go`. **How:** table-driven unit
  tests with the stub client.
- **What:** HTTP validation, outcome JSON, auth routing, and provider errors.
  **File:** `apps/backend/internal/github/controller_test.go`. **How:** handler
  tests for merged, queued, malformed method, and rejection paths.
- **What:** frontend API response typing and queue eligibility boundaries.
  **Files:** `apps/web/lib/api/domains/github-api.test.ts`,
  `apps/web/components/github/pr-task-icon.test.ts`. **How:** request capture and
  table-driven predicate tests.
- **What:** button notifications, accepted-state suppression, retry behavior,
  and refresh callback. **File:**
  `apps/web/components/github/pr-merge-button.test.tsx`. **How:** component tests
  with mocked API outcomes.
- **What:** queue observation, persistence, queue-unaware read preservation,
  terminal clearing, and bounded live/restart projection. **Files:**
  `apps/backend/internal/github/*merge_queue_status_test.go`,
  `apps/backend/internal/github/store_taskpr_schema_drift_test.go`,
  `apps/backend/internal/task/statussummary/merge_queue_status_test.go`, and
  `apps/backend/internal/backendapp/status_summary_adapter_test.go`. **How:**
  focused GraphQL, real-store, and projection tests.
- **What:** queue color precedence, failing-sibling aggregation, translated
  queue metadata, estimate boundaries, terminal queue clearing, and generic
  fallback copy. **Files:**
  `apps/web/components/github/pr-task-icon.test.ts`,
  `pr-task-status-summary.test.ts`, `pr-status-chip.test.tsx`,
  `pr-merge-queue-status.test.tsx`, `pr-mergeability-row.test.tsx`, and
  `pr-detail-panel.test.tsx`. **How:** pure formatter and focused component
  tests for terminal, queued, future-state, missing-estimate, sub-minute, and
  minute-boundary cases.

## E2E Tests

- **Scenario:** an eligible queue-required merge is accepted. **File:**
  `apps/web/e2e/tests/pr/pr-merge-queue.spec.ts`, action test. **What to
  verify:** the desktop PR surface exposes `Merge PR`, the merge request
  returns `{"status":"queued"}`, the success notification appears, and the
  accepted state suppresses the duplicate action.
- **Scenario:** an already queued PR exposes hydrated queue metadata. **File:**
  `apps/web/e2e/tests/pr/pr-merge-queue.spec.ts`, display test. **What to
  verify:** task icon color, task hover summary, compact popover, and detail
  notice show queued state, position, and estimated duration.
- **Scenario:** the same accepted action is touch-usable. **File:**
  `apps/web/e2e/tests/pr/mobile-pr-merge-queue.spec.ts`, action test. **What to
  verify:** Review navigation, touch activation, queued notification, a
  minimum 44px action target, and no document horizontal overflow.
- **Scenario:** an already queued PR is readable on a phone. **File:**
  `apps/web/e2e/tests/pr/mobile-pr-merge-queue.spec.ts`, display test. **What to
  verify:** the existing PR status drawer and full-height Review surface show
  queued state, position, and estimate without hover or horizontal overflow.

## Verification Results

Recorded after the remediation run:

- Backend: `cd apps/backend && go test -tags fts5
  ./internal/github/... ./internal/task/statussummary/...
  ./internal/backendapp` passed 2,193 tests across 3 packages, including the
  queue precedence regression.
- Frontend: the six-file queue-status command passed 160 tests. `pnpm run
  typecheck` and `pnpm run lint` passed. `pnpm run i18n:check` passed with
  7,223 referenced keys, 8,779 English entries, 48 orphans, and all five
  required catalogs complete.
- Desktop E2E: the fresh-build `cd apps/web && pnpm e2e:run
  tests/pr/pr-merge-queue.spec.ts` passed 2 tests in 12.2 seconds. Mobile E2E:
  `pnpm e2e:run --no-build --project mobile-chrome
  tests/pr/mobile-pr-merge-queue.spec.ts` passed 2 tests in 9.6 seconds.

## Implementation Waves And Parallel Candidates

Wave 1:
- [x] [task-01-backend-queue-aware-merge](task-01-backend-queue-aware-merge.md)

Wave 2:
- [x] [task-02-frontend-merge-outcomes](task-02-frontend-merge-outcomes.md)

Wave 3:
- [x] [task-03-desktop-mobile-e2e](task-03-desktop-mobile-e2e.md)

The detailed queue-observation task package under
`docs/plans/github-pr-merge-queue-status/` is the synchronized execution record
for the persistence, projection, queue color, metadata, and responsive display
work described above. Its E2E task and this parent plan intentionally record
the same two desktop and two mobile scenarios.

Execution is sequential in the primary conversation unless the user explicitly
authorizes subagents.
