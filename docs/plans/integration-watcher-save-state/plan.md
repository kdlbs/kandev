---
spec: docs/specs/ui/requirements/settings-manual-save.md
created: 2026-07-29
status: complete
---

# Implementation Plan: Integration Watcher Save State

## Overview

Watcher enabled-state saves pass through a shared frontend draft hook. GitHub
review-watch updates currently violate their frontend contract by returning an
acknowledgement instead of the updated watch, so the store does not advance and
the cleared draft reveals stale state. The repair aligns that endpoint with all
other watcher updates, makes the shared hook retain a clean saved baseline until
authoritative items reconcile, and proves the user-visible flow without a
refresh.

## Confirmed root cause

- `updateReviewWatch()` is typed as returning `ReviewWatch`, but
  `httpUpdateReviewWatch` returns `{"updated": true}`.
- `useReviewWatches.update` forwards that acknowledgement to
  `updateReviewWatch` in the Zustand slice. Because the response has no `id`,
  the cached row is not replaced.
- `useWatcherEnabledDrafts.save` removes the successful local draft immediately.
  When its `items` input is still stale, the rendered row falls back to the
  pre-save `enabled` value until a reload fetches the persisted record.
- The shared hook is used by GitHub, GitLab, Jira, Linear, and Sentry watcher
  settings, so it must tolerate delayed authoritative-list reconciliation.

## Backend

### GitHub review-watch update response

- Update `apps/backend/internal/github/controller.go` so
  `httpUpdateReviewWatch` loads and returns the updated `ReviewWatch`, matching
  the existing issue-watch handler and the TypeScript client contract.
- Add a focused HTTP regression in
  `apps/backend/internal/github/controller_test.go` that updates `enabled` and
  asserts the response contains the same watch `id` with the new value.

No database, authorization, or public route changes are required.

## Frontend

### Shared watcher saved baseline

- Update
  `apps/web/components/integrations/use-watcher-enabled-drafts.ts` to distinguish
  unsaved drafts from successfully persisted values that have not yet appeared
  in `items`.
- Render and toggle against the latest saved baseline, keep the contributor
  clean after a successful save, preserve edits made during an in-flight save,
  and remove the temporary saved baseline once the authoritative item matches
  or disappears.
- Extend
  `apps/web/components/integrations/use-watcher-enabled-drafts.test.tsx` with
  regressions for stale post-save items and later authoritative reconciliation.

### Mobile design contract

This is state reconciliation inside existing watcher tables and cards. It does
not change composition, navigation, overlays, scrolling, touch targets, or
viewport behavior. Desktop and mobile continue sharing the same watcher state
hook and action handlers. The nearest mobile exemplar is the existing GitLab
watch card in `mobile-gitlab-parity.spec.ts`, whose touch-sized pause control
already covers the shared mobile interaction.

## Tests

- **What:** GitHub review-watch update returns the authoritative updated row.
  **File:** `apps/backend/internal/github/controller_test.go`.
  **How:** SQLite-backed Gin handler test through the real controller/service/store.
- **What:** A successful watcher save stays visually changed and clean while
  incoming items are stale, then reconciles when they catch up.
  **File:**
  `apps/web/components/integrations/use-watcher-enabled-drafts.test.tsx`.
  **How:** Vitest hook harness under `SettingsSaveProvider`.
- **What:** Failed saves and in-flight newer edits remain dirty.
  **File:**
  `apps/web/components/integrations/use-watcher-enabled-drafts.test.tsx`.
  **How:** retain and extend the existing failure/retry coverage as needed.

## E2E Tests

- **Scenario:** GIVEN an active GitHub review watch, WHEN the user pauses it and
  saves, THEN the row stays Paused without reload; WHEN the user enables and
  saves again, THEN it stays Active without reload.
  **File:** `apps/web/e2e/tests/integrations/github-workspace-settings.spec.ts`.
  **What to verify:** status text, clean Save changes state, and persisted API
  value after each save.
- Run the existing mobile GitLab watcher pause scenario as change-aware proof
  that the shared hook retains mobile capability and touch behavior.

## Implementation Tasks

1. [x] [Task 01: Return the updated GitHub review watch](task-01-github-review-watch-response.md)
2. [x] [Task 02: Reconcile shared watcher saved state](task-02-shared-watcher-saved-baseline.md)
3. [x] [Task 03: Prove pause and resume without refresh](task-03-watcher-save-e2e.md)

Execution is sequential in the primary conversation. No task is delegated
without explicit user authorization.

## Verification

After each task's targeted checks:

```bash
make fmt
make typecheck test lint
```

The E2E managed runner rebuilds backend and frontend production artifacts before
the focused Playwright checks.

## Risks

- Clearing a saved overlay too early recreates the flicker; retaining it forever
  can mask a later authoritative update. Reconciliation therefore clears only
  when the matching item reaches the saved value or the item disappears.
- Concurrent edits during Save must remain dirty and must not be replaced by
  the completed request.
