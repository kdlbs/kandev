---
created: 2026-10-28
status: implemented
requirements:
  - REQ-UI-QUICK-CHAT-ACTIVE-VISIBILITY-001
system_design:
  - ../../specs/ui/system-design/quick-chat-active-tab-visibility.md
legacy_specs: []
---

# Implementation Plan: Quick Chat active tab visibility

## Overview

One sequential work order first proves the gap on unchanged production, then implements and verifies a strip-local reveal. UI owns the independent presentation behavior; existing selection and terminal contracts remain authoritative. The design checkpoint does not authorize implementation or heavy work. ROOT must read the complete requirement, design, plan and work order and issue a later separate execution and exclusive global HEAVY release.

## Scope

### In scope

- Selection-change reveal for persisted conversations, temporary setup tabs and terminals, including oversized labels.
- Focused mounted geometry, desktop and phone browser evidence, and one concise instruction in `docs/public/developer-tools.md` near the Quick Chat tabs paragraph.

### Out of scope

- Selection, ordering, drag, focus, lifecycle, API, storage, process or backend ownership changes; global CSS, labels/catalogs, polling, resize observers and speculative hardening.
- Live developer data, broad suites, shared dependencies/builds, extra agents, unrelated process/cache/temporary/volume inspection or cleanup. Respect `/home/jcfs/a.txt` exclusions and preserve concurrent edits.

## Technical approach

Follow the [system design](../../specs/ui/system-design/quick-chat-active-tab-visibility.md): derive kind-qualified identity from the existing props in `QuickChatTabStrip`; bind the strip and target DOM without disturbing `SortableQuickChatTab`'s DnD refs. On initial mount and selection identity change only, use local geometry and the smallest feasible `scrollLeft` delta. For a tab wider than the viewport reveal its activation control; if even that is wider align its leading edge. Remaining actions are reachable by manual scroll. Preserve focus and vertical scrolling. No active identity means no reveal. No resize, reorder, content or unrelated-render retrigger.

The source currently renders setup sessions outside the sortable wrappers; both paths need identity association. A real browser RED is a gate, not an assumed result: if browser focus already exposes the selected target or no ordinary user path reproduces it, stop with evidence before production changes.

### Mobile contract

Desktop opens the floating Quick Chat dialog; phone uses the app menu to open its full-height dialog. Existing Quick Chat phone tabs and their visible action menu are the nearest shipped surface; the curated mobile task switcher illustrates current-context and touch-accessible navigation. The existing direct tab row suits frequent switching and preserves drag and actions without introducing a new navigator. The row owns horizontal overflow; selected content owns vertical scrolling; existing dynamic viewport, safe-area and touch sizing remain. Shared selection drives both surfaces. A phone test must use real touch, actual row overflow and the configured Pixel 5 `mobile-chrome` project.

## ASCII UI preview

UI-01: shared Quick Chat tab row, before/after selection of an offscreen tab. Desktop and phone preserve the same row structure; phone uses a full-height dialog, visible action menus and an explicit close button. Brackets mark the scroll viewport; arrow is illustrative, not a new control.

```text
Before: [ Chat A | Chat B | ...             ]  selected: Terminal 3 offscreen
After:  [ ... | Chat B | Terminal 3  actions ]  selected: Terminal 3
Phone:  [ ... | Terminal 3  ... ] [close]
        | selected terminal content          |
```

The tab row scrolls horizontally; content scrolls vertically. When the tab is wider than the row, reveal its label-bearing control as far as possible and leave the action reachable by manual scrolling. Geometry and the absence of focus/vertical movement are requirements, spacing is illustrative. Covers AC 001.1–001.5; see the work order for rendered checks.

## Tests

`apps/web/components/quick-chat/quick-chat-tab-strip.test.tsx` mounts the real strip with controlled geometry. Assert minimum deltas in both directions, visible no-op, oversized control behavior, persisted conversation, terminal and setup, unchanged-identity rerender/manual browse, focus and vertical scroll, and bounded ordering/DnD interaction. Establish meaningful RED before implementation. Existing tab item and tab order tests remain focused regression evidence.

## E2E tests

`apps/web/e2e/tests/chat/quick-chat-active-tab-visibility.spec.ts` (`chromium`) and `apps/web/e2e/tests/chat/mobile-quick-chat-active-tab-visibility.spec.ts` (`mobile-chrome`) exercise an isolated real overflowing row with reachable launch/setup/adjacent activation, keyboard and touch respectively. Assert selected identity and geometry, focus/vertical continuity, no horizontal document overflow, and bounded existing order/drag interaction. Capture synthetic desktop and phone views and preserve each run's owned assets, manifest and hashes externally before a subsequent managed run can clear them. They cover AC 001.1–001.5; a synthetic store mutation alone is not integration proof.

## Work orders

- [x] [Task 01: Reveal active tab](task-01-reveal-active-tab.md)

## Verification results

Mounted regression failed before the change and passed after it; the ordinary terminal launcher reproduced the hidden selection in an overflowing desktop row. Focused desktop and phone E2E passed, including the existing phone reorder scenario. Web typecheck, focused lint, i18n, public docs and specification checks passed. See the work order and task plan for command receipts and captures.
