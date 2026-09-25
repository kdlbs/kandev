---
created: 2026-09-16
status: implemented
requirements:
  - REQ-UI-FILE-TREE-CHAT-CONTEXT-001
system_design:
  - ../../specs/ui/system-design/task-surface-render-isolation.md
legacy_specs: []
---

# Implementation Plan: File Tree Hidden Measurements

## Overview

Keep visible file-tree rows contiguous after the Files panel hides and reopens.
One work order adds a measurement guard and regression coverage.
The implementation is complete.

## Ownership and assumptions

UI owns row geometry and virtualization. Filesystem data and task lifecycle retain their existing owners.
The user supplied the intended complete tree and requested reproduction and repair.
The repair preserves file operations, row order, scroll ownership, and mobile navigation.
No material product choice remains unresolved.

## Confirmed cause and reproduction

`VirtualizedFileTreeView` in `apps/web/components/task/file-browser-parts.tsx`
uses the default TanStack row measurement through `ref={virtualizer.measureElement}`.
That measurement accepts zero-height `ResizeObserver` entries from hidden rows.
The entries replace valid cached heights and disrupt the virtual range on restoration.
Desktop Dockview uses `defaultRenderer="always"`, so inactive panels can retain mounted content.

A temporary browser fixture used the actual `FileTreeView`, shared `ScrollArea`,
row components, providers, and application CSS. Its viewport height was 560px.
The fixture supplied 18 directory rows and a button that toggled the ancestor's
`display` between `flex` and `none`. Two animation frames let each visibility
transition reach the browser observers. An initial owner update connected the
fixture's viewport ref, equivalent to the real tree's asynchronous data arrival.

| Scenario | First mounted index after restoration | Blank space at viewport top | Mounted rows |
| --- | --- | --- | --- |
| Original, compact rows | 12 | 234px | 6 |
| Temporary guard, compact rows, three cycles | 0 | 0px | 18 |
| Original, 44px touch rows at 393px browser width | 12 | 296px | 6 |
| Temporary guard, 44px touch rows at 393px browser width | 0 | 0px | 18 |

Desktop `scrollTop` remained zero. The broken state persisted after layout settled.
The touch reproduction changed `scrollTop` to 100px. The temporary guard retained zero.
The touch experiment exercised row sizing in the same fixture, not the complete mobile task layout.

The experiment changed only a temporary copy of the renderer. It kept a cached
row height, or its estimate, when default measurement returned zero.
This proves a matching failure path, while the permanent implementation and
integration coverage below verify the complete interaction.

## Scope

### In scope

- Guard unavailable row measurements in the file-tree virtualizer.
- Preserve positive dynamic measurements for normal rows and creation controls.
- Cover hidden-panel restoration and existing large-tree navigation on desktop and phone.

### Out of scope

- Dependency upgrades, changes to shared ScrollArea, or other virtualized lists.
- File loading, backend APIs, persisted layout, selection, and file operations.
- A new mobile composition or public documentation.

## Technical approach

Add `apps/web/components/task/file-tree-measurement.ts` for the typed measurement
callback. Delegate actual measurement to the installed TanStack function.
Accept positive measured heights. Otherwise retain the positive cached value
for `instance.options.getItemKey(index)`, or use `estimateSize(index)`.
Wire this callback into `VirtualizedFileTreeView` without changing its overscan,
row keys, viewport, reveal effects, or mounted-row bound.

Keep dynamic measurement. Fixed heights cannot cover every inline control or
responsive transition. Increasing overscan can conceal the defect without
protecting the measurement cache.

## ASCII UI preview

UI-01: Files panel after restoration, with the scroll position at the top.

```text
Observed desktop         Corrected desktop       Corrected phone Files
+-------------------+    +-------------------+   +----------------------+
| Files toolbar     |    | Files toolbar     |   | Files toolbar        |
|                   |    | > folder-00       |   | > folder-00     [...]|
|  blank gap        |    | > folder-01       |   | > folder-01     [...]|
|                   |    | > folder-02       |   | > folder-02     [...]|
| > folder-12       |    | ...               |   | ...                  |
| > folder-13       |    | > folder-17       |   +----------------------+
+-------------------+    +-------------------+   | Existing bottom nav  |
                                                +----------------------+
```

The toolbar and phone navigation retain their current positions.
The existing Files viewport is the only vertical scroll owner.
All rows intersecting that viewport must appear without gaps or overlaps
(`AC-UI-FILE-TREE-CHAT-CONTEXT-001.10`). Names and spacing in this sketch are illustrative.

The mobile exemplar is `components/task/mobile/session-mobile-layout.tsx`.
Users enter through the Files bottom-navigation action and tap a row to open it.
The existing full-height surface supports repeated file exploration.
It retains touch actions, safe-area clearance, and the current shared tree model.
Mobile panel switching currently unmounts Files, so tests cover that real path
in addition to hidden-measurement component coverage with touch-sized rows.

## Tests

`components/task/file-tree-measurement.test.ts` covers the measurement contract:

- A zero observer entry retains the previous positive height.
- A zero first measurement uses the node or creation-row estimate.
- A later positive measurement replaces the fallback after restoration or a touch-size change.

These cases support `.10` while preserving compact spacing `.9` and touch actions `.7`.
Use the real TanStack measurement function with controlled element geometry.
The zero-height regression must fail before the guard is implemented.

`components/task/file-tree-geometry.test.ts` covers the shared
viewport validator. It rejects a blank top edge, rejects a blank bottom edge when
the tree overflows the viewport, and permits legitimate trailing space for a short
tree or the file-tree container's end padding.

## E2E tests

Extend the existing desktop and mobile virtualization specs, preserving their current scenarios.

- Desktop: hide Files through an actual Dockview tab transition, then restore it.
  Cover a short tree at the top and a large tree after scrolling.
  Assert the first visible row reaches the viewport content top, the last visible
  row reaches the bottom when overflowing content continues below, and adjacent
  rows have no gaps or overlaps. The short-tree transition compares its ordered
  visible path window before and after restoration.
- Phone: enter Files, navigate away and return, then scroll and open a file.
  Reuse the same top, bottom, and adjacent-row validator, compare the ordered
  short-tree path window after Files to Chat to Files navigation, and assert
  reachable touch actions and no horizontal document overflow.
- Preserve the fewer-than-80 mounted-row bound for the 600-entry fixture,
  last-file access, and create-input reveal.

The implementation work order contains the exact build and test commands.

## Work orders

- [x] [Task 01: Preserve valid file-tree measurements](task-01-preserve-measurements.md)

This work order is sequential and has no dependencies.

## Verification results

Temporary browser reproduction and guard comparison passed as recorded above.
The production measurement callback now retains a positive row-keyed cache entry,
or the current row estimate, when TanStack receives a zero-height observer entry.
Positive measurements remain authoritative for compact rows, touch rows, and
inline creation controls.

- Focused Vitest: 4 files and 14 tests passed, including four shared viewport
  geometry cases for blank top, overflowing blank bottom, short-tree trailing
  space, and end padding.
- Frontend typecheck passed.
- Targeted ESLint passed for the measurement implementation, renderer, and E2E helpers/specs.
- Production-served desktop E2E: 3 tests passed, including collapsed, expanded-top,
  and scrolled Files restoration through Dockview. The restoration assertions
  compare ordered visible path windows at the same scroll offsets.
- Production-served mobile E2E: 3 tests passed, including the large-tree touch flow
  and the two existing file-tree chat-context scenarios. The large-tree flow uses
  the shared viewport validator and compares the collapsed path window after
  Files navigation.
- E2E sleep lint passed.
- `python3 scripts/list-docs.py validate`: passed (281 decisions, 973 specifications).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.

The temporary browser fixture, copied renderer, and browser configuration were
removed after the experiment. The named browser and isolated app were stopped.
Public documentation does not change.

## Risks

- Disabling measurement entirely would break variable-height controls.
- A fallback keyed by index can reuse another row's size after a tree change.
- Shared viewport checks must track intentional content insets and end padding if
  the Files layout changes.
- The isolated fixture remains diagnostic evidence; the production E2E scenarios
  cover the Dockview and mobile navigation lifecycles.

## Related implementation records

- [Original virtualization work order](../task-surface-render-isolation/task-02-virtualize-file-tree.md).
- [Responsive row spacing](../file-tree-responsive-spacing/plan.md).

Their historical results remain unchanged. This package owns the new visibility regression.
