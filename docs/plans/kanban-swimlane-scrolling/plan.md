---
created: 2026-09-16
status: completed
requirements:
  - REQ-UI-ADAPTIVE-KANBAN-001
  - REQ-UI-ADAPTIVE-KANBAN-002
  - REQ-UI-ADAPTIVE-KANBAN-003
system_design:
  - ../../specs/ui/system-design/adaptive-kanban.md
legacy_specs: []
---

# Implementation Plan: Six-card Kanban swimlane scrolling

## Overview

Keep all workflows visible in their existing swimlanes. Increase compact lanes to six initial task rows and add directional overflow cues.
Two sequential work orders implement sizing first, then scrollbar presentation and accessible scrolling.
The user requested this design package after confirming these choices. Implementation is authorized and is being executed through the two sequential work orders below.

## Scope

### In scope

- Six-row compact sizing for multiple desktop/tablet workflows, with content-sized sparse lanes.
- Column top/bottom fades and lane left/right fades that reflect actual hidden content.
- Scrollbars visible during hover, focus, or scrolling without geometry changes.
- Native vertical scroll chaining to the outer workflow container.
- Touch, keyboard, reduced-motion, and forced-color behavior.
- Preservation of virtualization, WIP dividers, drag geometry, existing filters, and phone navigation.

### Out of scope

- Workflow tabs, a replacement overview, show-more controls, and column pagination.
- Automatic empty/Done-column collapse beyond existing user preferences.
- Column-width redesign, synchronized scrolling, sticky workflow-header stacks, or custom scrollbar widgets.
- Backend changes, dependencies, new settings, migrations, or new runtime flags.
- Publication and delegation.

## Assumption check and ownership

Confirmed: all workflow swimlanes stay visible, six cards replace the earlier four-card suggestion, and hover-based scrollbar reveal is desired.
Confirmed by the accepted proposal: keyboard focus and active scrolling also reveal scrollbars, and column boundaries permit outer scrolling.
Verified: compact lanes currently use `max(12.5rem, naturalHeight)` with shared column height and drag-time freeze.
Verified: each column already virtualizes rows. `AdaptiveDesktopKanban` owns horizontal pan, snap, and the drag reserve.
Routine design choice: six means the first six logical task rows, including their metadata, gaps, and intervening WIP divider.
Routine design choice: retain the 12.5rem sparse floor and full-height single-workflow behavior. The six-row rule does not truncate cards.
UI owns the existing reusable adaptive-board presentation contract. Task lifecycle and workflow data stay with their existing systems.
No material questions remain. No ADR is required because the paired design preserves this local presentation rationale.

## Technical approach

### Sizing

Extend `VirtualizedColumnTaskList`'s compact height callback to report the initial six-row segment rather than every task.
Keep the full virtualizer total for scroll range. Reuse measurement caches and estimates without mounting every card.
`useColumnNaturalHeight` adds actual chrome. `useCompactSwimlaneHeight` retains the floor, cleanup, and drag freeze but removes the fixed maximum.
All new measurement props participate in existing memo comparisons. Scroll position alone must not resize a lane.

### Overflow cues and input

Add a Kanban-scoped overflow hook and stationary decorative overlays around existing native scroll viewports.
Integrate with `AdaptiveDesktopKanban`, `VirtualizedColumnTaskList`, and the tablet branch of `SwimlaneKanbanContent`.
Scope scrollbar CSS in `app/globals.css`. Reuse theme tokens and the nearby sidebar hover-reveal pattern as styling precedent.
Reserve native geometry permanently. Preserve drag-only horizontal suppression and ignore drag-reserve space in content cues.
Keep native scrolling. Audit ancestor overflow rules and give scroll regions translated accessible names and keyboard focus stops.

### Phone and tablet

Home is the entry point. The shipped exemplars are `MobileColumnTabs`, `mobile-menu-sheet.tsx`, and `kanban-with-preview.tsx`.
Phone retains workflow/step context, its temporary picker drawer, a full-height task list, and direct task navigation.
This primary task list owns vertical scrolling. Fades are decorative and touch scrolling never depends on hover.
Tablet retains two-column snap navigation and receives the new multi-workflow sizing.
Existing state derivation, filtering, selection, and actions are shared. Presentation state remains local and is not persisted.

## ASCII UI preview

UI-01: Desktop Home, All Workflows, mixed dense and sparse lanes.

```text
[Home / Search / Existing display controls]
+-- Feature 19 -------------------------------------------+
| Backlog 0   Analysis 8      Implement 2    Review 1       |
|             [card 1]        [card 1]       [card 1]       |
|             [card 2]        [card 2]                      |
|             [card 3]                                    |
|             [card 4]                                    |
|             [card 5]                                    |
|             [card 6]                                    |
|             ..bottom fade..                             |
| < left fade only after scrolling   right fade if needed>|
| [horizontal scrollbar appears during interaction]       |
+-- Contributor PR Review 3 -------------------------------+
| Triage 1        Review 2          Fix 0         Done 0    |
| [card 1]        [card 1]                                 |
|                 [card 2]                                 |
+---------------------------------------------------------+
Outer board: vertical scroll between workflow separators.
Analysis: independent vertical scroll through all 8 cards.
```

UI-02: Overflow column, fine pointer, interaction and boundary states.

```text
Start / idle         Middle / hover or focus      End / idle
[fixed header]       [fixed header]               [fixed header]
[card 1]             ..top fade..      | thumb    ..top fade..
...                  [middle cards]   |          ...
[card 6]             ..bottom fade..   |          [last card]
..bottom fade..                                  no bottom fade
```

UI-03: Phone Home, existing focused context.

```text
[Workflow / Step v] [Search / Actions]
[card 1]
[card 2]
...
[visible cards]       one full-height task scroll region
..bottom fade..       only with more content below
                     [Create task]
                     safe-area clearance
```

UI-04: Empty, short, collapsed, and touch states.

```text
[Workflow >]                  collapsed: header only
[Empty workflow v]
[Existing empty guidance]     no false fade or scrollbar
[Short workflow v]
[card]                        existing minimum floor
[Tablet workflow v]
[column A] [column B] ...      two-column snap navigation
```

Workflow separators, six-row sizing, independent scroll regions, and the focused phone surface are structural requirements.
Labels, spacing, and fade depth are illustrative. Existing column visibility preferences remain authoritative.
Headers stay outside their task scroll regions. Workflow headers move with the outer board as they do today.
No global sticky-header stack is added. Phone keeps dynamic viewport sizing, safe areas, and existing touch targets.
UI-01/04 map to 002.1-002.7. UI-02/03 map to 003.1-003.9.

## Tests

The following unit and component evidence is implemented and recorded below.

| Criteria                  | Unit/component evidence                                                                                                                                                                                       |
| ------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 002.1-002.2, 002.5, 002.7 | `use-compact-swimlane-height.test.ts`: six-row height, sparse floor, stale steps, drag freeze.                                                                                                                |
| 002.1, 002.7              | `use-column-natural-height.test.ts`: chrome included once. Virtualized-list tests: prefix estimate, measured prefix, WIP divider, scroll-stable reporting, bounded mounts, and offscreen prefix invalidation. |
| 003.1-003.4, 003.8-003.9  | New `use-kanban-overflow.test.ts`: each edge, client-dimension boundary tolerance, real-content extent, content resize, idle timer, unchanged-state isolation, cleanup.                                       |
| 001.9-001.10, 003.9       | `adaptive-desktop-kanban.test.tsx`: drag reserve and width remain stable.                                                                                                                                     |

All unit paths are under `apps/web/hooks/domains/kanban` or `apps/web/components/kanban` as named in the work orders.

## E2E tests

| Criteria                   | File, project, and scenario                                                                                                                                                                                                   |
| -------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 002.1-002.2                | `swimlane-height.spec.ts`, chromium: six standard and mixed-height cards fully fit at the initial scroll position. A seventh card remains reachable. Sparse lanes shrink independently.                                       |
| 002.3-002.5, 002.7         | Same file: collapse, filter to one workflow, restore multiple workflows, preview resize, empty recovery, metadata update, and unchanged height while scrolling.                                                               |
| 001.5-001.7, 002.7         | Existing large-column suites: 440 tasks mount fewer than 100 bodies. Existing spacing suite checks actual row gaps.                                                                                                           |
| 003.1-003.4                | New `swimlane-scroll-affordances.spec.ts`, chromium: vertical and horizontal start/middle/end, no-overflow state, hover/focus/scroll reveal, idle reset, and unchanged card/header bounds.                                    |
| 003.5-003.6                | Same file: keyboard region scrolling, final-card action, and real wheel gestures at both boundaries move the outer container. Sibling positions stay fixed.                                                                   |
| 003.7-003.8                | Same file: coarse-pointer tablet, reduced-motion, forced colors, empty/filter/resize transitions. Capture both light and dark themes.                                                                                         |
| 003.9, 001.4, 001.9-001.10 | Existing board, reorder, auto-hide, WIP, and multi-select suites. New affordance spec adds drag near faded edges, fitting-board reserve exclusion, last-real-column overflow, drop, and cancellation without geometry shifts. |
| 002.6, 003.7               | `mobile-kanban.spec.ts` and `mobile-kanban-navigation.spec.ts`, mobile-chrome: scroll to final task, open and return, picker navigation, focus surface containment, and no document horizontal overflow.                      |
| 003.7, 003.9               | Existing mobile reorder, auto-hide, and large-column suites retain touch actions and bounded mounts.                                                                                                                          |

All E2E files are under `apps/web/e2e/tests/kanban/`.
Phone uses the configured Pixel 5, plus 767px and 768px checks with the same pointer mode.
Tablet coverage uses `tabletTestPage`. Verify computed styles as well as geometry across the boundary.
Capture desktop, tablet, and phone screenshots and compare them with UI-01 through UI-04.
Native scrollbar appearance also needs visual inspection on Chromium and an available Firefox/WebKit browser.
If another engine is unavailable, record that limitation rather than claim cross-engine verification.

## Work orders

- [x] [Task 01: Size swimlanes for six task rows](task-01-six-card-sizing.md) - complete
- [x] [Task 02: Add accessible overflow cues](task-02-overflow-cues.md) - complete

Task 02 depends on Task 01. Execute sequentially in the primary session.

## Companion packages

The adaptive-kanban, kanban-large-column-virtualization, kanban-swimlane-height, and kanban-drag-column-width-stability packages describe previous deliveries.
Their recorded results remain historical. This package replaces the old 400px height assertion while retaining width, drag, and virtualization guarantees.
Do not copy old passing counts into this package or reopen completed tasks as new implementation.
The current height helper and E2E assertions must change together in Task 01.

## Documentation and lifecycle

This turn changes internal specifications, plans, and the Kanban implementation. Public documentation must describe the behavior only where it has a user-facing guide.
The docs-maintainer impact check against `docs/public`, `README.md`, and `docs/screenshots.md` found no public scrolling contract or screenshot that needs an update.
Update an existing affected Kanban guide during implementation if it documents scrolling behavior. No new public page is required for decorative cues alone.
New accessible labels use translations in all five languages. Traditional Chinese comes from `pnpm run i18n:zh-hant`.
Both work orders passed. The design is current and the pending-revision note was removed from the requirements.
Use the canonical lifecycle guide. Requirement status remains active. Record delivery completion in this plan and its work orders.

## Verification commands

Each work order contains a complete root-relative command block. `pnpm e2e:run` supplies a fresh managed production build.
Run its commands sequentially without overlapping suites or overriding worker limits.
New test and hook files named in Task 02 are created by that work order before its final checks.
Run new behavioral tests first for RED, then implement and rerun for GREEN. A missing selector is not a behavioral RED result.

## Verification results

Implemented and verified on 2026-09-16. The six-row compact sizing and local overflow cues are current:

- `make -C apps/backend build` passed.
- `make -C apps/backend e2e-plugin-package` passed.
- `pnpm run build:e2e` passed.
- Focused Kanban unit/component suites passed: 57 tests across eight sizing, render-stability, overflow, and drag-scroll files.
- The overflow hook uses client dimensions for native scroll boundaries and accepts the real lane-grid extent so synthetic drag reserve does not create a false cue.
- Offscreen first-six prefix changes invalidate bounded measurements, use estimates until rows return, and remeasure returned rows without widening mounts.
- Targeted Chromium height and overflow-affordance runs passed: 6 and 5 tests, including six-card mixed-height geometry, tablet full-strip cues, forced colors, and drag-reserve exclusion.
- Neighboring desktop Kanban regression suites passed: 23 tests.
- Mobile Kanban regression matrix passed: 29 tests across the retained view suite and the extracted navigation/scrolling suite.
- `pnpm run typecheck`, `pnpm run i18n:check`, and `pnpm run i18n:ratchet` passed.
- Full ESLint and targeted Prettier checks passed.
- Catalog validation, specification lint, reference/path validation, and `git diff --check` passed.
- E2E screenshots were captured for the desktop and mobile overflow scenarios during the managed runs.
- Chromium and mobile-Chromium were available and verified. Firefox and WebKit were not run in this environment.

The implementation also fixed integration details exposed by the regression matrix: sparse TanStack measurement caches are handled without assuming every index is present, offscreen prefix measurements are invalidated locally, the inner keyboard-focusable scroll regions are excluded from desktop drag-pan initiation, and tablet scroll owners retain native scrollbars in forced colors. Horizontal scrollbar gutter reservation is disabled for the lane owner so drag-time scrollbar hiding does not change its width.

The original design-package checks passed on 2026-09-16:

- Catalog validation: 272 decisions and 938 specifications.
- Specification-linter tests: 36 passed.
- Full specification lint passed.
- Plan links, requirement IDs, design paths, and existing likely files passed.
- Diff whitespace checks passed before implementation; the completed work orders are recorded above.

No public documentation changes were required because the affected contract is internal presentation behavior.

## Risks

- Six mixed-height cards produce taller lanes and more outer scrolling. That is the accepted density tradeoff.
- Prefix measurements can become stale after width or metadata changes. Tests cover invalidation and scroll stability.
- Transparent native scrollbars vary by browser and OS. Preserve native fallback behavior and report tested engines.
- An intermediate overflow wrapper can absorb wheel input. Real boundary gestures must prove outer scroll chaining.
- Fades can obscure content or capture drag input unless overlays remain shallow and pointer-transparent.
