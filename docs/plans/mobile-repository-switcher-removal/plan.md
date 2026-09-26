---
spec: docs/specs/ui/requirements/mobile-task-navigation.md
created: 2026-07-31
status: complete
---

# Implementation Plan: Mobile Repository Switcher Removal

## Overview

Remove the repository pill and picker from the phone task workbench because the control does not select repository state directly: it changes the active session. Keep the existing session picker as the single mobile control for active runtime context, identify repository-bound sessions inside that control for multi-repository tasks, preserve desktop and tablet behavior, and retain the optional-hydration resilience regression.

## Frontend

### Mobile task top bar

- `apps/web/components/task/mobile/session-mobile-top-bar.tsx`: stop rendering `MobileRepoPill` in `MobileTopBarActions`. Keep task, plugin, remote-executor, Git, approval, and task-switcher actions unchanged.
- Delete `apps/web/components/task/mobile/mobile-repo-pill.tsx`, `apps/web/components/task/mobile/mobile-repos-section.tsx`, and `apps/web/components/task/mobile/mobile-repos-section.test.tsx` after confirming they have no remaining consumers.
- Keep `MobileSessionsPicker` in `apps/web/components/task/mobile/mobile-sessions-section.tsx` as the mobile entry point for changing sessions and therefore repository context. When loaded sessions span repositories, include the bound repository slug in each session row and in the active-session pill; leave single-repository presentation unchanged. Derive that distinction from required session data rather than the optional workflow snapshot.

No production backend, API, persistence, desktop Dockview, or tablet changes are required. The E2E-only session seed route accepts `repository_id` so Playwright can model sessions bound to different repositories.

## Mobile Design Contract

- **Desktop outcome and mobile entry:** desktop and tablet repository/session interactions remain unchanged. Phone users enter the task workbench normally and change runtime context through the existing session pill above Chat.
- **Nearest shipped exemplar:** `MobileSessionsPicker` in `mobile-sessions-section.tsx` already uses `MobilePillButton` plus `MobilePickerSheet` for the actionable session hierarchy.
- **Hierarchy and primary action:** task identity and task-level actions stay in the fixed top bar; active-session selection stays with Chat. Repository identity is not promoted as a separate action.
- **Presentation and rationale:** no replacement drawer or navigation is added. Repository choice is not an independent mobile action in this surface, and the former picker only redirected to a representative session. Multi-repository context is secondary text within the retained session hierarchy.
- **Scroll, viewport, safe area, and touch:** existing `h-dvh` workbench, fixed top bar, internal panel scrolling, bottom navigation, and safe-area behavior remain unchanged. Removing the pill reduces top-bar crowding.
- **Shared logic:** session state, repository bindings, and all non-mobile repository flows remain unchanged; only phone presentation code is removed.
- **Mobile proof:** production Playwright scenarios cover optional repository/session hydration failure and selecting a session bound to another repository. They assert the repository pill is absent, the session pill remains usable and repository-aware, selection updates active context, and document horizontal overflow is absent.

## Tests

- Delete the component test dedicated to `MobileReposSection` with the removed component.
- Extend the existing `MobileSessionsPicker` component coverage for two same-agent sessions bound to different repositories, including the accessible pill label, visible row metadata, and selected-session action.

## E2E Tests

- **Scenario:** **GIVEN** a multi-repository task whose optional repository or session hydration fails, **WHEN** the phone task view opens, **THEN** no repository switcher is rendered, the session picker remains visible, and the task view stays usable without horizontal overflow.
- **File:** `apps/web/e2e/tests/layout/mobile-spa-resilience.spec.ts`.
- **What to verify:** replace repository-picker interaction assertions with absence of `mobile-repo-pill`, visibility of `mobile-sessions-pill`, a non-empty root, no unexpected browser errors, and no document horizontal overflow.
- **Repository-aware session scenario:** open a task with its primary session, seed a secondary-repository session while the page and WebSocket are live, select it through `MobileSessionsPicker`, and verify API hydration, row labels, active pill, selected row, and the horizontal-overflow contract in `apps/web/e2e/tests/session/mobile-multi-repository-session-picker.spec.ts`.

## Implementation

- [x] [Task 01 — Remove mobile repository switcher](task-01-remove-mobile-repository-switcher.md)

## Risks

- The existing SPA-resilience scenario currently relies on opening the repository picker. Update it without weakening its delayed/failed hydration, blank-root, browser-error, or overflow coverage.
- Repository controls outside the phone task top bar are out of scope and must remain intact.

## Verification

```bash
cd apps/web && pnpm e2e:run --project mobile-chrome -- tests/layout/mobile-spa-resilience.spec.ts --grep "keeps a multi-repository task usable without a mobile repository switcher when optional hydration fails" --workers=1
cd apps/web && pnpm e2e:run tests/session/mobile-multi-repository-session-picker.spec.ts -- --project=mobile-chrome
cd apps && pnpm --filter @kandev/web test -- --run components/task/mobile/mobile-sessions-section.test.tsx
cd apps/backend && go test ./internal/office/testharness
cd apps/web && pnpm run typecheck
cd apps/web && pnpm lint
```

## Results

- RED: after waiting for the retained session picker, the baseline production build failed as expected because `mobile-repo-pill` had count `1`, not `0`.
- GREEN: removing the top-bar render made the focused `mobile-chrome` scenario pass.
- REFACTOR: deleted the orphaned repository picker components and component test; web typecheck passed, then the final production rebuild and focused Playwright scenario passed (`1 passed`).
- REVIEW RED: Codex identified that same-label sessions on different repositories were indistinguishable; the focused component test and production mobile E2E both failed on the missing repository-aware pill label.
- REVIEW GREEN: multi-repository session rows and the active pill now include the canonical repository slug, the E2E-only seed route preserves `repository_id`, and the focused component suite (`8 passed`) plus cross-repository mobile Playwright scenario (`1 passed`) pass.
- REVIEW 2 RED: removing the workflow-task repository snapshot from the focused component fixture reproduced the missing repository labels.
- REVIEW 2 GREEN: repository labels now derive from required loaded session bindings plus the repository store, independent of optional workflow hydration. The focused component suite (`8 passed`), web typecheck, web lint, and production mobile Playwright scenario (`1 passed`) pass.
- REVIEW VERIFICATION: `go test ./internal/office/testharness` and full `pnpm lint` both pass.
- REVIEW 3 RED: a focused session-slice test showed that a partial live-session upsert left an already-loaded task session list marked authoritative (`true`, expected `false`).
- REVIEW 3 GREEN: an unknown live-session upsert now invalidates the task list for API hydration, while updates to known sessions remain cached. Focused slice/hook/picker tests (`39 passed`), typecheck, full lint, production build, and the live-seeded mobile Playwright scenario (`1 passed`) pass.
- REVIEW 4 RED: a focused hook test showed a failed stale-list hydration replaced the existing and partial live session rows with an empty list.
- REVIEW 4 GREEN: non-forced hydration failures now preserve the current store snapshot and mark it loaded, avoiding immediate retry loops while reconnect/foreground refresh retains a recovery path. Focused slice/hook/picker tests (`40 passed`), typecheck, full lint, production build, and the live-seeded mobile Playwright scenario (`1 passed`) pass.
- REVIEW 5 RED: a deferred hook test showed that an older successful list response dropped a live session added while the request was in flight and did not schedule another fetch.
- REVIEW 5 GREEN: list loads now retain sessions introduced after the request began and queue one forced authoritative reload to hydrate them. Request reconciliation was extracted to keep the hook within lint limits; focused slice/hook/picker tests (`41 passed`), typecheck, full lint, production build, and the live-seeded mobile Playwright scenario (`1 passed`) pass.
