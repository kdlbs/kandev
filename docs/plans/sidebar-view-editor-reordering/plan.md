---
created: 2026-10-07
status: completed
requirements:
  - REQ-UI-SIDEBAR-VIEW-REORDER-001
system_design:
  - ../../specs/ui/system-design/sidebar-view-editor-reordering.md
legacy_specs: []
---

# Implementation plan: Sidebar view editor reordering

## Overview

Replace Sort's paired arrows with grips and remove redundant arrows from Automatic colors.
Use one compact More menu for explicit adjacent moves across all reorderable editor lists.
One sequential work order delivers the interaction, regression coverage, and public guidance.

UI owns the reusable editor interaction. Sort evaluation, view persistence, and personal color matching remain with their existing contracts.
The [requirements](../../specs/ui/requirements/sidebar-view-editor-reordering.md) define the observable result.

## Scope

### In scope

- Sort handles with stable identity and insertion across several positions.
- Color-rule grip cleanup and compact More menus for sort/color/detail lists.
- Pointer, touch, keyboard, cancellation, input isolation, and focus behavior.
- Targeted desktop/phone regression tests and a short public how-to update.

### Out of scope

Filter order, multi-level grouping, section ordering, task navigation ordering, Threads direction controls, ranking changes, and schema changes.

## Technical approach

Use the existing dnd libraries and personal settings routes.
`SortChainEditor` uses its existing identified entries as sortable IDs and commits only changed orders.
Add an ID-based `arrayMove` helper beside the existing adjacent-move helper.
`SortChainRuleCard` owns sortable card geometry and a leading grip instead of the desktop ordinal slot.

Add `SidebarReorderMenu` using existing menu primitives. Callers provide adjacent move callbacks and localized item names.
Replace arrows in `AutomaticColorRuleHeader` and add the menu beside the existing detail switch in `SortableDetailRow`.
Retain removal, rule editing, limits, announcements, and existing rollback paths.

Use `SidebarFilterPopover`'s resolved workspace/view scope to invalidate gestures after context changes.
Retain the anchored desktop popover and mobile drawer. Limit touch gesture suppression to the handle.
The [design inventory](../../specs/ui/system-design/sidebar-view-editor-reordering.md#section-inventory) records all audited sections and related view surfaces.

The previous [sort-chain package](../sidebar-running-first-activity-sort/plan.md) is completed history.
Update its current system design's Editor section during implementation, while preserving its completed work-order results.

## ASCII UI preview

### UI-01: Desktop Sort and color headings

Entry: Tasks filter gear, Sort expanded. Current source-backed Sort controls:

```text
1 [Color v] [Red v] [Matching first v] [Up] [Down] [X]
```

Proposed fine-pointer rows and color heading:

```text
SORT
[::] [Running v]      [Running first v]    [...] [X]
[::] [Color v] [Red v] [Matching first v]   [...] [X]
[::] [Last activity v][Newest first v]     [...] [X]
[+ Add sort]

AUTOMATIC COLORS
[::] Rule 1 Origin                        [...] [X]

More menu: [Move up] [Move down]
```

`[::]` is the grip. Grip, More, and Remove replace the ordinal and three old action slots.
The More menu retains explicit moves, with the first item's Move up disabled and the last item's Move down disabled.
At one sort rule, grip/reorder actions and Remove are unavailable. Custom remains one standalone card.
Sort fields retain their order, accessible names, and swatch labels.

### UI-02: Phone expanded Sort section

Entry: Tasks picker, Sidebar filters, Sort expanded.

```text
View editor                         [Close]
SORT
+-----------------------------------------+
| [::] Rule 1                    [...] [X] |
| [Running                              v]|
| [Running first                        v]|
+-----------------------------------------+
+-----------------------------------------+
| [::] Rule 2                    [...] [X] |
| [Color                                v]|
| [Red                                  v]|
| [Matching first                       v]|
+-----------------------------------------+
[+ Add sort]

More menu: [Move up] [Move down]
```

The drawer header stays fixed. The existing editor body owns vertical scrolling and bottom safe-area clearance.
All touch actions have at least 44px targets. A tap on More provides ordering without a gesture.
The grip starts dragging only that card. Scrolling elsewhere continues normally.

### UI-03: Task-row details on desktop and phone

Entry: view editor, Task row expanded, details enabled.

```text
[::] Relative time                  [...] [On]
[::] Pull request number            [...] [On]
```

Keep existing handles and switches. Add the same explicit move menu.
Desktop actions are compact. Phone/coarse-pointer actions retain 44px targets.

Required structure: leading grips, compact explicit alternative, retained removal/visibility actions, and stacked phone fields.
Spacing and example labels are illustrative. UI-01 through UI-03 map to AC .1, .3, .4, .6, and .8.

## Tests

All criterion suffixes refer to `AC-UI-SIDEBAR-VIEW-REORDER-001`.

| Criteria | Test evidence |
| --- | --- |
| .2, .5 | `sort-chain-editor-model.test.ts`: three-or-more-entry insertion, unchanged/missing IDs, immutable input, values/IDs preserved |
| .1, .3, .4, .8 | `sort-chain-editor.test.tsx` and new `sidebar-reorder-menu.test.tsx`: menu boundaries, one-rule/Custom behavior, no opening-side effects, stable focus |
| .1, .2, .3, .5, .7 | `automatic-color-settings.test.tsx`: reorder complete rules through menu, preserve condition/output/enabled state, rejected unchanged orders |
| .2, .3, .4, .7 | `task-row-settings.test.tsx`: explicit and keyboard moves retain visibility and detail identities |
| .5, .7 | `sidebar-filter-popover.test.tsx`: view/workspace changes cancel gestures and preserve drafts |

## E2E tests

- `sidebar-running-first-activity-sort.spec.ts`, Chromium: real handle drag across two positions, keyboard movement/cancellation, More movement, save/reload, and preserved task selection.
- `mobile-sidebar-running-first-activity-sort.spec.ts`, mobile-chrome: replace old arrow interactions with menu taps, preserve every existing sort/limit/group-indent assertion.
  Add touch handle dragging, normal body scrolling, 44px action geometry, card containment, and zero document overflow.
- `sidebar-automatic-colors.spec.ts` and `mobile-sidebar-automatic-colors.spec.ts`: overlapping matching rules expose changed first-match color after grip/menu moves and reload.
  Preserve existing repository selection and input alignment coverage.
- `sidebar-filter.spec.ts` and `mobile-sidebar-views.spec.ts`: retain existing detail drag tests and prove explicit menu moves preserve visibility and saved detail order.
- Include desktop 28px geometry, phone width below 768px with a fine pointer, and wider coarse-pointer containment in these focused specs.
  Browser evidence maps to .1 through .8. Use observable settings requests and existing causal wait helpers.

## Work orders

- [x] [Task 01: Implement compact editor reordering](task-01-editor-reordering.md) (`completed`)

Order: Task 01 only. Execution is sequential. No delegation is authorized.
The work order contains the exact validation commands.

## Verification results

Design-package validation passed on 2026-10-07:

- `python3 scripts/list-docs.py validate`: 359 decisions and 1425 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Local `validateCoverage` preflight: covered, with the planned sort editor source included to exercise delivery traceability.
  The preflight used only package references, within the repository's document-input size limit.
- All four package files have resolving document links and valid whitespace.
- Requirement, acceptance, plan, and design references resolve in the work order.
- `git diff --check -- docs/specs docs/plans/sidebar-view-editor-reordering`: passed.
- At the design-package handoff, the four new package files were unstaged and no production or permanent test files had changed.

Implementation verification is complete; see [Task 01 results](task-01-editor-reordering.md#results) for the focused runtime, browser, translation, geometry, and documentation evidence. Changes remain unstaged and uncommitted.

## Risks

- A swap instead of insertion silently reverses intermediate priority.
- Index/field-based IDs move the wrong card after editing or duplicate color criteria.
- Touch dragging can compete with drawer dismissal and body scrolling.
- Menu focus can jump to another item after reordering or a stale drop can target another view.
