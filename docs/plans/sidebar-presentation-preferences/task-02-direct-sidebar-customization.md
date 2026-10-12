---
id: "02-direct-sidebar-customization"
title: "Customize directly from the sidebar"
status: done
wave: 2
depends_on:
  - "01-sidebar-preferences"
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-CUSTOMIZATION-001
  - REQ-UI-SIDEBAR-CUSTOMIZATION-004
  - REQ-UI-SIDEBAR-CUSTOMIZATION-005
  - REQ-UI-SIDEBAR-CUSTOMIZATION-006
  - REQ-UI-NAV-HIERARCHY-004
  - REQ-UI-NEEDS-YOU-INBOX-001
acceptance_criteria:
  - AC-UI-SIDEBAR-CUSTOMIZATION-001.1
  - AC-UI-SIDEBAR-CUSTOMIZATION-001.2
  - AC-UI-SIDEBAR-CUSTOMIZATION-001.4
  - AC-UI-SIDEBAR-CUSTOMIZATION-004.3
  - AC-UI-SIDEBAR-CUSTOMIZATION-005.2
  - AC-UI-SIDEBAR-CUSTOMIZATION-005.5
  - AC-UI-SIDEBAR-CUSTOMIZATION-006.1
  - AC-UI-SIDEBAR-CUSTOMIZATION-006.2
  - AC-UI-SIDEBAR-CUSTOMIZATION-006.3
  - AC-UI-SIDEBAR-CUSTOMIZATION-006.4
  - AC-UI-SIDEBAR-CUSTOMIZATION-006.5
  - AC-UI-SIDEBAR-CUSTOMIZATION-006.6
  - AC-UI-SIDEBAR-CUSTOMIZATION-006.7
  - AC-UI-SIDEBAR-CUSTOMIZATION-006.8
  - AC-UI-NAV-HIERARCHY-004.1
  - AC-UI-NEEDS-YOU-INBOX-001.1
  - AC-UI-NEEDS-YOU-INBOX-001.3
system_design:
  - ../../specs/ui/system-design/sidebar-customization.md
  - ../../specs/ui/system-design/navigation-hierarchy.md
  - ../../specs/ui/system-design/needs-you-inbox-03.md
---

# Task 02: Customize directly from the sidebar

## Summary

Add one right-click customization menu and handle-free drag ordering to the
desktop navigation region. Save visibility/order through the existing workspace
layout contract and presentation choices through Task 01's account preferences.
Extend Inbox customization and provide keyboard/phone alternatives.

## In scope

- Read the plan, paired specs, `/tdd`, `/mobile-parity`, `/e2e`, and scoped guidance.
- Make eligible Inbox entries optional layout nodes in defaults, catalog,
  validation, projection, Settings editor, regular/Office navigation, and phone navigation.
- Materialize missing Inbox identities without losing existing order, hidden
  values, unavailable references, or feature/mode eligibility.
- Add shared menu triggers to optional top-level labels and blank navigation
  space. Checkboxes include hidden entries, custom groups, and eligible plugins.
- Add fast-icon and New Task style controls, explicit reorder alternatives,
  and Open sidebar layout settings as the final action.
- Use installed dnd-kit sortable primitives for whole-row label dragging. Render
  no drag handles, grips, hover handles, or edit-mode handles. Change only the
  active drag cursor to `grabbing`, show an insertion marker, and restore on cleanup.
- Preserve normal clicks, modified links, disclosure behavior, quick actions,
  provider shortcuts, and task-row context menus. Suppress completed-drag clicks.
- Reuse `toggleNodeVisibility`, `moveSection`, `sidebar_layout_state`, expected
  revisions, and shared SSR/WS mapping. Serialize direct writes per workspace;
  preserve Settings drafts and isolate rollback/conflict state from saved state.
- Keep first-edit and all-hidden recovery available. Add visible phone Customize
  access to an inset Drawer with checkboxes, preferences, and Move up/down.
- Add targeted tests, locale copy, public tutorial instructions, and recorded results.

## Out of scope

Task-row dragging, provider child reordering, nested shortcut editing from the
sidebar, new endpoints, separate database/browser layouts, changing feature
eligibility, hiding Tasks or fixed settings/header chrome, commits, and publication.

## Acceptance

1. Direct menu edits and Settings manipulate the same database-backed layout
   and preferences. Inbox participates in visibility/order, hidden entries are
   restorable, and eligibility remains independent. Reload and another client
   see saved changes without overwriting unrelated nodes or user settings.
2. Desktop top-level rows reorder directly with no handles in any state. Active
   drag uses `grabbing`; click behavior, cursor cleanup, drop indicators,
   cancellation, and one acknowledged write per changed drop are correct.
3. Failures, revision conflicts, rapid successive edits, dirty Settings drafts,
   and workspace changes preserve authoritative state. Keyboard and phone users
   can perform the same edits without dragging or long pressing.

## ASCII UI preview

Use UI-05, UI-06, and UI-07 from the [full plan](plan.md#ascii-ui-preview).
They map to AC-UI-SIDEBAR-CUSTOMIZATION-006.1 through 006.8.

```text
UI-05: Right-click menu             UI-06: Direct drag, no handles
+-------------------------------+    Home
| [x] New Task                  |    Inbox
| [x] Home                      |    Canvases                     >
| [x] Inbox                     |    ------ insertion marker ------
| [x] Automations               |    Automations                  >
| [x] Canvases                  |    Integrations                 >
| [x] Integrations              |
| [ ] Hidden custom group       |  Cursor: grabbing during active drag only.
|-------------------------------|
| [ ] Show fast action icons    |  UI-07: Phone customization drawer
| New Task style |  [Visibility checkbox rows]
| Move selected entry up/down   |  Reorder entry [Canvases v]
|-------------------------------|  [Move up] [Move down]
| Open sidebar layout settings  |  [Fast icons] [New Task style]
+-------------------------------+  Open sidebar layout settings (last)
```

Styles are radio entries; the settings action stays last. Labels and ASCII
spacing are illustrative; no drag handle is allowed even on hover/focus. The
phone drawer has one scroller, safe-area clearance, 44px targets, and a visible
Customize opener. It does not depend on long pressing or touch dragging.

## Verification

Use Red-Green-Refactor for normalization, mutation sequencing, response guards,
and changed conditional rendering. Proposed new test/component paths below are
work-order outputs; existing dependencies are shared with Task 01.

```bash
(cd apps/backend && go test ./internal/user/models ./internal/user/service ./internal/user/handlers ./internal/user/store)
(cd apps/web && pnpm exec vitest run lib/sidebar/layout-operations.test.ts lib/sidebar/layout-projection.test.ts lib/sidebar/mobile-layout.test.ts components/settings/sidebar-layout-editor-save.test.ts components/settings/sidebar-layout-editor.test.tsx lib/sidebar/sidebar-customization.test.ts hooks/domains/sidebar/use-sidebar-customization.test.ts components/app-sidebar/sidebar-draggable-navigation.test.tsx components/navigation/mobile-canvases-section.test.tsx)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/sidebar-direct-customization.spec.ts tests/settings/sidebar-customization.spec.ts tests/layout/navigation-hierarchy.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-sidebar-direct-customization.spec.ts tests/settings/mobile-sidebar-customization.spec.ts tests/layout/mobile-navigation-hierarchy.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/app-sidebar components/navigation/mobile-sidebar-layout-navigation.tsx components/settings/sidebar-layout-editor-preview.tsx lib/sidebar --max-warnings 0)
(cd apps/web && pnpm run i18n:check)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Use the managed browser runner sequentially. Browser evidence must assert no
handle markup in rest/hover/drag states, actual grabbing cursor/cleanup, no
accidental navigation or disclosure during drag, no writes on cancellation,
menu final-item order, immediate reload persistence, all-hidden restoration,
Inbox feature gating, conflict/failed-save feedback, and clean versus dirty
Settings synchronization. Phone checks include explicit reorder, viewport
containment, safe areas, 44px targets, and focus return. Include dark/light and
Portuguese labels. Record results and run work-order documentation preflight.

## Files likely touched

- `apps/backend/internal/user/models/sidebar_layouts.go` and tests.
- `apps/backend/internal/user/service/sidebar_layouts.go` and layout patch tests.
- `apps/web/lib/sidebar/layout-types.ts`, `layout-projection.ts`,
  `layout-operations.ts`, `shortcut-catalog.ts`, mobile projection, and tests.
- `apps/web/hooks/domains/sidebar/use-sidebar-layout-navigation.ts` and catalog hooks.
- `apps/web/components/app-sidebar/sidebar-layout-navigation.tsx`, primary/default
  navigation composition, and regular/Office navigation wrappers.
- New `sidebar-customize-menu.tsx` and `sidebar-draggable-navigation.tsx` components,
  the controller under `hooks/domains/sidebar/`, and focused tests.
- `apps/web/components/navigation/mobile-sidebar-layout-navigation.tsx`,
  phone navigation composition, and tests.
- `apps/web/components/settings/sidebar-layout-editor-preview.tsx`, draft/save
  synchronization, and component tests. Extract shared normalization when needed.
- Browser specs listed above; shared API-client fixture types if needed.
- Existing locale namespaces in every shipped language, plus generated resources.
- `docs/public/use-kandev.md`, specifications, and both work-order result sections.

## Dependencies

Task 01 must be done before this work starts: its stored preferences and controls
are reused by the menu. No persistent Kandev subtask or extra agent is authorized.

## Risks

Fixed Inbox duplication, invisible drag listeners swallowing normal clicks,
pointer cancellation leaving the cursor changed, stale full-layout replacements,
phone gestures stealing scrolling, and direct writes erasing an existing editor draft.

## Parallelism

`sequential`

## Inputs

- [Sidebar customization requirements](../../specs/ui/requirements/sidebar-customization.md), especially 006.
- [Sidebar customization design](../../specs/ui/system-design/sidebar-customization.md), Direct sidebar editing and Inbox layout entries.
- [Navigation preferences](../../specs/ui/system-design/navigation-hierarchy.md) and Task 01.
- [Inbox visible naming](../../specs/ui/system-design/needs-you-inbox-03.md).
- Existing ContextMenu primitives, dnd-kit editor operations, revision-aware
  layout saves, phone drawer primitives, and UI-05 through UI-07.

## Results

Complete on 2026-10-06. Implemented and verified in the primary session.
See the [shared implementation and validation record](plan.md#implementation-results-2026-10-06)
for backend, SQLite/PostgreSQL, frontend, desktop/phone browser, build,
localization, lint/type, and documentation evidence. No unresolved blocker.
The user subsequently authorized delivery to existing PR #4239.
