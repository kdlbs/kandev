---
spec: docs/specs/agents/runtime-updates.md
created: 2026-08-22
status: complete
---

# Implementation Plan: Managed Runtime Version Picker

## Overview

Improve the existing managed-runtime update dialog without changing its API or
selection semantics. The dialog will show a compact status summary and quick
latest/default choices, while a searchable browser exposes the complete stable
catalogue on demand in both the desktop dialog and mobile drawer.

## Frontend

### Compact update dialog

- Refine `apps/web/components/settings/agent-runtime-update-control.tsx` so
  the status summary does not repeat the up-to-date label and the full version
  list is not rendered in the initial view.
- Keep current/effective/default values visible as compact metadata and retain
  the existing approval, preview, and job state wiring.
- Keep the initial preview free of body overflow on desktop and mobile. Allow
  the shared body to scroll only for long version lists or streamed output,
  while preserving 44px mobile actions and a reachable footer.

### Version browser

- Add a focused picker component beside the existing control for quick latest
  and Kandev-default actions plus an explicit browse trigger.
- Use the shared command/search primitives for version filtering and 44px
  selectable rows. Preserve latest, active, and default markers and call the
  existing target/default callbacks.
- Keep the browser inside the existing dialog body/drawer so desktop and mobile
  share selection state and one internal scroll owner.

### Localization

- Add the browse, search, quick-choice, and empty-search copy to all required
  agent locale catalogues. Generate Traditional Chinese updates with the
  repository i18n command.

## Tests

- **What:** The initial dialog exposes compact quick choices and does not render
  all version rows.
  **File:** `apps/web/components/settings/agent-runtime-update-control.test.tsx`.
  **How:** Render a long preview and assert the browse trigger plus quick
  actions; assert a distant version is absent before browsing.
- **What:** Searching the browser filters the catalogue and selecting a result
  uses the existing target-preview callback.
  **File:** `apps/web/components/settings/agent-runtime-update-control.test.tsx`.
  **How:** Open the browser, fill the search field, assert matching/nonmatching
  rows, then select the matching row and assert the callback.
- **What:** The browser works in the mobile drawer with contained long content.
  **File:** `apps/web/e2e/tests/settings/mobile-agent-runtime-update.spec.ts`.
  **How:** Use the mobile fixture to open the update drawer, browse/search, and
  select a version while asserting no document horizontal overflow.

## E2E Tests

- **Scenario:** A long catalogue opens as a compact dialog with quick choices,
  and browsing exposes searchable versions.
  **File:** `apps/web/e2e/tests/settings/agent-runtime-update.spec.ts`.
  **What to verify:** The summary and browse trigger are visible, the full list
  appears only after browsing, and a search result can be selected.
- **Scenario:** The same browse/search/selection flow works on Pixel 5.
  **File:** `apps/web/e2e/tests/settings/mobile-agent-runtime-update.spec.ts`.
  **What to verify:** The drawer contains the browser, touch rows are reachable,
  and document horizontal overflow remains zero.

## Verification Results

- `cd apps && pnpm --filter @kandev/web test -- --run components/settings/agent-runtime-update-control.test.tsx` passed (6 tests).
- `cd apps/web && pnpm run typecheck` passed.
- `cd apps/web && pnpm run i18n:check` passed; all five locale catalogues are complete.
- Changed-file ESLint passed with zero warnings or errors.
- `cd apps/web && pnpm e2e:run --project chromium e2e/tests/settings/agent-runtime-update.spec.ts` passed (15 tests).
- `cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/settings/mobile-agent-runtime-update.spec.ts` passed (5 tests).
- `git diff --check` passed.
- The compactness refinement passed the rendered no-initial-overflow checks on
  desktop and mobile; streamed output still produced a scrollable body.
- Fresh desktop and mobile preview screenshots were validated and compressed
  for PR publication.
- The Traditional Chinese generator was attempted but refused to write because of the existing `dynamicProfileSettings` residual warning in both Traditional Chinese catalogues. The five new values were added in the generator's OpenCC-equivalent form, and the full i18n check passed afterward.

## Implementation Waves And Parallel Candidates

Wave 1, sequential:

- [x] [Task 01: Implement the compact searchable picker](task-01-compact-searchable-picker.md)

## Risks

- The picker must reuse the existing callbacks and not change backend target
  validation or preview semantics.
- A mobile browser must not create a second page-level scroll container or
  hide the primary update action behind an unreachable surface.
