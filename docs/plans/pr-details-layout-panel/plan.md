---
spec: docs/specs/ui/requirements/task-layout-profiles.md
created: 2026-07-31
status: complete
---

# Implementation Plan: Layout-owned PR Details Panel

## Overview

Replace this branch's global Appearance preference for pull-request tab placement with the existing task-layout profile system. Promote the canonical `pr-detail` runtime panel to the reusable **PR Details** panel, put it beside Agent in the built-in Default layout's `CENTER_GROUP`, and let users place or remove it with `Settings > General > Layouts`. Keep review identity synchronization separate from layout mutation: runtime code may update which review the canonical panel renders, but only layouts decide whether the panel exists and where it lives.

No backend schema, user-settings field, endpoint, or ADR is required. `users.settings.saved_layouts` already owns portable layout placement, and `docs/specs/ui/requirements/task-layout-profiles.md` owns the product contract.

## Architecture boundary

- `LayoutState` owns the canonical `pr-detail` panel's existence, tab order, group, geometry, and initial active tab.
- Review-domain state owns only the canonical panel's provider and primary PR/MR key.
- Keyed `pr-detail|...` and `mr-detail|...` panels remain task-specific transient review tabs; the Layout editor never serializes them.
- Existing custom profiles and environment-scoped task layouts are not backfilled. Fresh environments and Reset Layout use the updated built-in/default profile.
- Mobile and tablet task Review surfaces remain unchanged. Mobile settings continues to edit desktop profiles through the existing responsive Layout editor.

## Frontend

### Remove the parallel global setting

- Restore the backend user-settings contract in `apps/backend/internal/user/**` and `apps/backend/internal/backendapp/**` by removing this branch's `pr_panel_placement` model, DTO, persistence, boot-state, and validation changes.
- Remove `prPanelPlacement` from frontend HTTP/SSR/store/WebSocket mappings in `apps/web/lib/types/**`, `apps/web/lib/ssr/user-settings.ts`, `apps/web/lib/state/slices/settings/**`, `apps/web/lib/ws/handlers/users.ts`, and related tests.
- Remove the Pull Request Tabs card and draft/save wiring from `apps/web/components/settings/general-settings.tsx` and `apps/web/components/settings/editors-settings-state.tsx`.
- Delete `apps/web/lib/state/pr-panel-placement.ts` and its tests. Remove placement options from `pr-topbar-button.tsx`, `dockview-add-panel-items.tsx`, and the Dockview action signatures.

### Register PR Details as reusable layout content

- In `apps/web/lib/state/layout-manager/constants.ts`, add `pr-detail` to `REUSABLE_PANEL_IDS` and rename its registry title to `PR Details`. Keep `mr-detail` supported for legacy/keyed runtime layouts, but do not make it reusable.
- In `apps/web/lib/state/layout-manager/presets.ts`, add `pr-detail` after Chat in `CENTER_GROUP` for `defaultLayout()`. Keep Chat active by default, and keep `RIGHT_TOP_GROUP` limited to Files and Changes. Add `pr-detail` to `compactLayout()` because compact desktop has one tab group.
- Update the built-in Default description in `apps/web/lib/layout/layout-profiles.ts` and let its existing reusable-panel validation accept canonical `pr-detail` while rejecting keyed review IDs.
- Register a side-effect-free `pr-detail` placeholder in `apps/web/components/settings/layouts/layout-editor.tsx`; the editor's existing reusable-panel catalog then exposes **PR Details** for add, move, split, remove, keyboard, and touch actions.

### Separate review identity from layout structure

- Replace `useAutoPRPanel` in `apps/web/components/task/dockview-session-tabs.ts` and `useAutoMRPanel` in `dockview-auto-mr-panel.ts` with one focused `dockview-review-panel-sync.ts` hook. Its pure helper updates params on an existing canonical `pr-detail` only; it never adds, closes, moves, or activates a panel.
- Prefer a linked GitHub PR over a GitLab MR as today, clear stale opposite-provider params during task/provider changes, and clear both identities when no review exists so `ReviewDetailPanelComponent` renders its normal empty state.
- Remove obsolete PR-panel-offered session-storage state from `apps/web/lib/local-storage.ts`.
- Remove PR/MR panels as implicit Agent-session anchors in `dockview-session-tabs.ts`. Their group is now layout-owned and may intentionally be remote from Agent.
- Refactor `addPRPanel` and `addMRPanel` in `apps/web/lib/state/dockview-panel-actions.ts`: focus an exact existing panel in place; reuse the canonical panel when it already renders the requested key; otherwise add a keyed panel to the canonical `pr-detail` group, falling back to `centerGroupId` when canonical is absent. Never move an existing tab and never create a new split.
- Mount the unified synchronization hook from `dockview-desktop-layout.tsx`. Keep `ReviewDetailPanelComponent` as the provider-aware renderer for canonical `pr-detail`, and keep keyed provider-specific components compatible with restored layouts.

## Tests

- `apps/web/lib/state/layout-manager/presets.test.ts`: exact Default Agent-group, active Agent tab, and right-top order plus compact membership.
- `apps/web/lib/state/layout-manager/merger.test.ts`: Plan-to-Default session replacement preserves Agent before PR Details and keeps Agent selected.
- `apps/web/lib/layout/layout-profiles.test.ts`: canonical `pr-detail` accepted; keyed PR/MR IDs rejected.
- `apps/web/components/settings/layouts/layout-editor-actions.test.ts`: PR Details appears once in the add catalog and participates in existing panel operations.
- `apps/web/components/task/dockview-review-panel-sync.test.ts`: GitHub/GitLab/empty identity transitions mutate params only and ignore a missing canonical panel.
- `apps/web/lib/state/dockview-panel-actions.test.ts`: canonical-group anchoring, center fallback, matching canonical reuse, and focus-in-place without relocation.
- `apps/web/components/task/dockview-session-tabs.test.ts`: review groups are not treated as Agent anchors.
- Remove obsolete global-setting, placement-geometry, auto-add/remove, and offered-session tests instead of preserving superseded behavior.

## E2E

- Extend `apps/web/e2e/tests/settings/layout-profiles.spec.ts` to prove the built-in Default contains PR Details beside Agent and a saved move changes fresh/reset desktop placement.
- Extend `apps/web/e2e/tests/settings/mobile-layout-profiles.spec.ts` to add or move PR Details with touch controls and assert no document-level horizontal overflow.
- Replace `apps/web/e2e/tests/pr/pr-detail-auto-show.spec.ts` with focused layout-owned coverage: canonical content follows the active task, no-review state leaves the tab present, and removing the canonical tab is not undone by linked-review updates.
- Update `pr-detail-manual-open.spec.ts` and `pr-detail-dedup.spec.ts` for canonical-group anchoring, center fallback, exact-tab focus, and no relocation.
- Remove global Appearance-setting flows from `mobile-general-settings.spec.ts` and `settings-manual-save.spec.ts`; keep their unrelated coverage intact.

## Mobile design contract

- Desktop outcome: Default layout shows PR Details as a background tab beside Agent in the center group; custom placement uses existing Dockview editor controls.
- Phone entry: `Settings > General > Layouts`, using the existing settings drawer and Layout editor surface.
- Nearest mobile exemplar: `apps/web/e2e/tests/settings/mobile-layout-profiles.spec.ts`; no new drawer, overlay, or navigation model is introduced.
- Scroll ownership: existing settings page remains the vertical scroll owner; editor controls must not add horizontal page scrolling.
- Touch parity: existing contextual add/move/split/remove controls must expose PR Details. Mobile/tablet task-detail Review navigation does not change.

## Public documentation

- Update `docs/public/sessions-and-review.md`: remove Appearance-based placement instructions and describe PR Details as a reusable Layout panel, including built-in Default placement, fresh/reset scope, and existing-layout behavior.

## Implementation waves

Wave 1:

- [x] [Task 01 - Reusable panel and settings cleanup](task-01-reusable-panel-and-settings-cleanup.md)

Wave 2 (depends on Wave 1):

- [x] [Task 02 - Review synchronization and opening behavior](task-02-review-sync-and-opening.md)

Wave 3 (depends on Wave 2):

- [x] [Task 03 - E2E, docs, and verification](task-03-e2e-docs-verification.md)

## Risks

- Existing saved layouts intentionally lack `pr-detail`; auto-inserting it would violate task-layout precedence and user ownership.
- Dockview parameter updates merge values. Provider switches must explicitly clear stale `prKey`, `mrKey`, and provider markers.
- Existing Agent-restoration logic assumes review tabs were Agent siblings. Leaving that heuristic intact could restore Agent into the configured PR Details group.
- Default and compact preset changes affect fresh environments and Reset Layout; ordering must keep Agent and PR Details together without focus theft.
- This branch currently changes backend and frontend settings contracts. Cleanup must remove the entire unused contract so no dead API field or persistence path remains.
- E2E runs use production Vite assets; every frontend E2E run must use the managed runner with a fresh build after source changes.

## Verification

```bash
make -C apps/backend test
cd apps && pnpm --filter @kandev/web test lib/state/layout-manager/presets.test.ts lib/layout/layout-profiles.test.ts components/settings/layouts/layout-editor-actions.test.ts components/task/dockview-review-panel-sync.test.ts components/task/dockview-session-tabs.test.ts lib/state/dockview-panel-actions.test.ts components/task/review-panel-provider.test.ts
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
cd apps/web && pnpm e2e:run tests/settings/layout-profiles.spec.ts tests/pr/pr-detail-layout.spec.ts tests/pr/pr-detail-manual-open.spec.ts tests/pr/pr-detail-dedup.spec.ts
cd apps/web && pnpm e2e:run tests/settings/mobile-layout-profiles.spec.ts -- --project=mobile-chrome
```

## Completion evidence

- 2026-07-31: focused frontend unit suite passed (165 tests); frontend typecheck and lint passed.
- 2026-07-31: `make -C apps/backend test` and both public-doc validators passed.
- 2026-07-31: fresh production-build desktop PR/Layout E2E suite passed (8 tests), and focused desktop Layout capture passed (4 tests).
- 2026-07-31: mobile Layout E2E passed (2 tests), including touch selection and no horizontal overflow.
- 2026-07-31: Default PR Details placement moved beside Agent. Focused preset unit test,
  fresh production desktop E2E, mobile Layout E2E (2 tests), typecheck, lint, and public-doc
  validation passed.
- 2026-07-31: CI exposed Plan-to-Default focus theft: live session tabs were appended after
  PR Details. The merge now replaces Chat in place and preserves Agent selection; focused
  unit tests, the fresh-build 14-test workflow E2E suite, typecheck, lint, and public-doc
  validation passed.
