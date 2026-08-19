---
spec: docs/specs/github-pr-merge-queue/spec.md
created: 2026-08-17
status: complete
---

# Implementation Plan: GitHub PR Merge Queue

## Overview

Extend the existing GitHub merge boundary to use GitHub's asynchronous,
queue-aware merge request and return a typed merged-or-queued outcome. Then
reuse that outcome in the existing PR merge button across the desktop detail,
compact status, and mobile Review surfaces, with targeted unit and E2E proof.

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

## E2E Tests

- **Scenario:** eligible direct merge reports merged. **File:**
  `apps/web/e2e/tests/pr/pr-merge-queue.spec.ts`. **What to verify:** the desktop
  PR surface exposes the action, submits it, and shows the merged notification.
- **Scenario:** queue-required merge reports queued and does not resubmit.
  **File:** `apps/web/e2e/tests/pr/pr-merge-queue.spec.ts`. **What to verify:**
  the desktop action remains available for the queue-required state and the
  accepted queue notification is shown once.
- **Scenario:** phone Review surface provides the same queued outcome. **File:**
  `apps/web/e2e/tests/pr/mobile-pr-merge-queue.spec.ts`. **What to verify:**
  Review navigation, touch activation, queued notification, a touch-sized
  action, and no document horizontal overflow.

## Verification Results

- Backend: `make -C apps/backend test ARGS='./internal/github/...'` passed; the
  Make target executed the full backend `go test -tags fts5 ./...` suite.
- Frontend: 96 focused Vitest tests passed; `pnpm run i18n:check` and
  `pnpm run typecheck` passed.
- Desktop E2E: `pnpm e2e:run --no-build tests/pr/pr-merge-queue.spec.ts`
  passed (1 test).
- Mobile E2E: `pnpm e2e:run --no-build --project mobile-chrome
  tests/pr/mobile-pr-merge-queue.spec.ts` passed (1 test).

## Implementation Waves And Parallel Candidates

Wave 1:
- [x] [task-01-backend-queue-aware-merge](task-01-backend-queue-aware-merge.md)

Wave 2:
- [x] [task-02-frontend-merge-outcomes](task-02-frontend-merge-outcomes.md)

Wave 3:
- [x] [task-03-desktop-mobile-e2e](task-03-desktop-mobile-e2e.md)

Execution is sequential in the primary conversation unless the user explicitly
authorizes subagents.
