---
spec: docs/specs/ui/requirements/workspace-active-first-order.md
created: 2026-08-14
status: complete
---

# Implementation Plan: Active workspace first in Settings

## Overview

Add one pure, stable display-order helper that moves the workspace matching
`workspaces.activeId` to the front while preserving every other item's order.
Use that helper at the two existing Settings consumers: the branched sidebar
builder and the workspace cards page. The feature has no backend, API, store
shape, persistence, or copy changes.

## Frontend

### Shared display ordering

- `apps/web/lib/settings/workspace-display-order.ts`: add a generic helper that
  returns a new array, moves one matching active id to index zero, and leaves
  the input order intact when the id is null, unknown, or already first.
- `apps/web/lib/settings/workspace-display-order.test.ts`: cover active-middle,
  active-first, null/unknown active id, empty input, and stable non-active order.

### Settings sidebar tree

- `apps/web/components/app-sidebar/sections/settings/settings-menu-branches.ts`:
  apply the shared helper inside `buildWorkspacesBranch` before building the
  existing workspace nodes. Keep the current active badge assignment and all
  workspace tab/integration ordering unchanged.
- `apps/web/components/app-sidebar/sections/settings/settings-menu-branches.test.ts`:
  add a builder-level assertion that an active workspace in the middle is the
  first workspace node and that the remaining nodes preserve order.

### Workspaces page

- `apps/web/app/settings/workspace/workspaces-page-client.tsx`: subscribe to
  the existing active workspace id, derive the ordered list for rendering, and
  keep creation state updates and card actions unchanged.

### Mobile design contract

- Desktop outcome: the active workspace is the first actionable record in the
  Settings sidebar tree and the first workspace card on the management page.
- Phone entry point: `/settings` remains the existing Settings index; in a tree
  menu mode, the user expands the existing Workspaces branch. The management
  page remains the existing vertical card list.
- Nearest shipped exemplar: `apps/web/components/settings/settings-page-nav.tsx`
  and `apps/web/e2e/tests/settings/mobile-settings-sidebar.spec.ts`, which use
  the same Settings tree on a phone. Reuse their current navigation and scroll
  behavior; this change only changes record order.
- Hierarchy, primary action, presentation, scroll owner, safe areas, and touch
  targets remain unchanged. Shared ordering is derived from store state and is
  used by both viewport compositions; no mobile-only state or control is added.

## Tests

- **What:** the shared ordering contract moves only the active workspace and
  preserves all other relative order, including missing-active fallbacks.
  **File:** `apps/web/lib/settings/workspace-display-order.test.ts`.
  **How:** focused Vitest unit tests against the pure helper.
- **What:** the Settings sidebar branch receives the same active-first order.
  **File:** `apps/web/components/app-sidebar/sections/settings/settings-menu-branches.test.ts`.
  **How:** build a branch from plain workspace records and assert node ids.
- **What:** the desktop Settings page and sidebar visibly show the active
  workspace first when another workspace is created ahead of it.
  **File:** `apps/web/e2e/tests/settings/workspace-display-order.spec.ts`.
  **How:** create a second workspace through the fixture API, render the
  existing active workspace on `/settings/workspaces`, opt into the existing
  accordion menu mode, and assert the first card and first record row by
  workspace id/name.
- **What:** the phone Settings index exposes the same active-first workspace
  branch order without horizontal overflow.
  **File:** `apps/web/e2e/tests/settings/mobile-workspace-display-order.spec.ts`.
  **How:** use the `mobile-chrome` project, expand the existing Workspaces
  branch, assert record order, and assert document width remains contained.

## E2E Tests

- **Scenario:** GIVEN a non-active workspace is inserted before the active
  workspace in live data, WHEN the desktop workspace page renders, THEN the
  active workspace card is first.
  **File:** `apps/web/e2e/tests/settings/workspace-display-order.spec.ts`.
- **Scenario:** GIVEN the same two workspaces and an accordion settings menu,
  WHEN the Workspaces branch is expanded, THEN the active workspace row is
  first and its active badge remains visible.
  **File:** `apps/web/e2e/tests/settings/workspace-display-order.spec.ts`.
- **Scenario:** GIVEN a phone Settings index in accordion mode, WHEN the
  Workspaces branch is expanded, THEN the active workspace row is first and the
  document has no horizontal overflow.
  **File:** `apps/web/e2e/tests/settings/mobile-workspace-display-order.spec.ts`.

## Verification Results

- `cd apps && pnpm install --frozen-lockfile` — passed.
- RED focused Vitest run — 30 tests, 1 expected active-order failure before
  implementation; the helper-plus-builder run then showed 35 tests, 1 expected
  builder-order failure before wiring.
- RED desktop and phone E2E runs — both failed on the expected active-order
  assertion before the consumer wiring.
- `cd apps && pnpm --filter @kandev/web test -- --run lib/settings/workspace-display-order.test.ts components/app-sidebar/sections/settings/settings-menu-branches.test.ts` — 2 files, 35 tests passed.
- `cd apps/web && pnpm run typecheck` — passed.
- `cd apps/web && pnpm run lint` — passed.
- `cd apps/web && pnpm e2e:run --project chromium tests/settings/workspace-display-order.spec.ts` — 1 test passed with a fresh build.
- `cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-workspace-display-order.spec.ts` — 1 test passed with a fresh build.
- Disposable managed PR capture — 1 test passed with fresh desktop and 390px
  phone screenshots; both assets were visually inspected, compressed, and kept
  only in ignored `apps/web/.pr-assets/` for PR publication.
- `git diff --check` — passed.

## Implementation Waves And Parallel Candidates

Wave 1 — sequential:

- [x] [task-01-active-workspace-first](task-01-active-workspace-first.md)

The helper, its consumers, and their focused browser evidence share the same
display-order contract and should be implemented and verified as one task.

## Open Questions

None.
