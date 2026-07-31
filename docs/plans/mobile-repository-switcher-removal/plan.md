---
spec: docs/specs/ui/mobile-task-navigation.md
created: 2026-07-31
status: complete
---

# Implementation Plan: Mobile Repository Switcher Removal

## Overview

Remove the repository pill and picker from the phone task workbench because the control does not select repository state directly: it changes the active session. Keep the existing session picker as the single mobile control for active runtime context, preserve desktop and tablet behavior, and retain the optional-hydration resilience regression.

## Frontend

### Mobile task top bar

- `apps/web/components/task/mobile/session-mobile-top-bar.tsx`: stop rendering `MobileRepoPill` in `MobileTopBarActions`. Keep task, plugin, remote-executor, Git, approval, and task-switcher actions unchanged.
- Delete `apps/web/components/task/mobile/mobile-repo-pill.tsx`, `apps/web/components/task/mobile/mobile-repos-section.tsx`, and `apps/web/components/task/mobile/mobile-repos-section.test.tsx` after confirming they have no remaining consumers.
- Keep `MobileSessionsPicker` in `apps/web/components/task/mobile/session-mobile-layout.tsx` unchanged. It remains the mobile entry point for changing sessions and therefore repository context.

No backend, API, persistence, desktop Dockview, or tablet changes are required.

## Mobile Design Contract

- **Desktop outcome and mobile entry:** desktop and tablet repository/session interactions remain unchanged. Phone users enter the task workbench normally and change runtime context through the existing session pill above Chat.
- **Nearest shipped exemplar:** `MobileSessionsPicker` in `session-mobile-layout.tsx` already uses `MobilePillButton` plus `MobilePickerSheet` for the actionable session hierarchy.
- **Hierarchy and primary action:** task identity and task-level actions stay in the fixed top bar; active-session selection stays with Chat. Repository identity is not promoted as a separate action.
- **Presentation and rationale:** no replacement drawer or navigation is added. Repository choice is not an independent mobile action in this surface, and the former picker only redirected to a representative session.
- **Scroll, viewport, safe area, and touch:** existing `h-dvh` workbench, fixed top bar, internal panel scrolling, bottom navigation, and safe-area behavior remain unchanged. Removing the pill reduces top-bar crowding.
- **Shared logic:** session state, repository bindings, and all non-mobile repository flows remain unchanged; only phone presentation code is removed.
- **Mobile proof:** a production Playwright scenario opens a multi-repository task after optional repository/session hydration fails, asserts the repository pill is absent, the session pill remains visible, the root remains usable, and document horizontal overflow is absent.

## Tests

- Delete the component test dedicated to `MobileReposSection` with the removed component.
- Do not add a replacement unit/component test: this is pure UI composition removal with no new logic. The browser scenario below is the behavioral regression test.

## E2E Tests

- **Scenario:** **GIVEN** a multi-repository task whose optional repository or session hydration fails, **WHEN** the phone task view opens, **THEN** no repository switcher is rendered, the session picker remains visible, and the task view stays usable without horizontal overflow.
- **File:** `apps/web/e2e/tests/layout/mobile-spa-resilience.spec.ts`.
- **What to verify:** replace repository-picker interaction assertions with absence of `mobile-repo-pill`, visibility of `mobile-sessions-pill`, a non-empty root, no unexpected browser errors, and no document horizontal overflow.

## Implementation

- [x] [Task 01 — Remove mobile repository switcher](task-01-remove-mobile-repository-switcher.md)

## Risks

- The existing SPA-resilience scenario currently relies on opening the repository picker. Update it without weakening its delayed/failed hydration, blank-root, browser-error, or overflow coverage.
- Repository controls outside the phone task top bar are out of scope and must remain intact.

## Verification

```bash
cd apps/web && pnpm e2e:run --project mobile-chrome -- tests/layout/mobile-spa-resilience.spec.ts --grep "keeps a multi-repository task usable without a mobile repository switcher when optional hydration fails" --workers=1
cd apps/web && pnpm run typecheck
```

## Results

- RED: after waiting for the retained session picker, the baseline production build failed as expected because `mobile-repo-pill` had count `1`, not `0`.
- GREEN: removing the top-bar render made the focused `mobile-chrome` scenario pass.
- REFACTOR: deleted the orphaned repository picker components and component test; web typecheck passed, then the final production rebuild and focused Playwright scenario passed (`1 passed`).
