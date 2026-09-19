---
created: 2026-09-03
status: complete
requirements:
  - REQ-UI-FILE-TREE-CHAT-CONTEXT-001
system_design:
  - ../../specs/ui/system-design/file-tree-chat-context.md
legacy_specs: []
---

# Implementation Plan: File Tree Responsive Spacing

## Overview

Restore compact file-tree row spacing on coarse-pointer devices, including
zoomed-out phones that select the desktop workbench. The existing visible touch
action remains available for every coarse pointer; all compositions keep
single-line rows.

The prior `FileBrowser` implementation enabled a 44px overflow trigger and row
wrapping for every non-fine pointer, including wide desktop workbenches. The
trigger remains required; its wrapping behavior caused the spacing in the
report.

## Scope

### In scope

- Keep the visible 44px action trigger on every coarse-pointer file tree, including desktop workbench.
- Keep file-tree rows compact and single-line in every composition.
- Add regression coverage for coarse-pointer desktop and mobile geometry.

### Out of scope

- Changing file-tree data loading, selection, drag/drop, or file operations.
- Changing the context-menu action or chat-context state and message contracts.
- Redesigning the mobile Files surface or changing panel persistence.

## Technical approach

Update `FileBrowser` in `apps/web/components/task/file-browser.tsx` to retain
the existing coarse-pointer touch-action gate. Keep `TreeNodeItem` and
`SearchResultsList` in `apps/web/components/task/file-browser-parts.tsx`
non-wrapping. Responsive rows reserve a 44px vertical interaction slot for
their absolutely positioned trigger, while labels reserve the horizontal action
space and truncate.

Add focused regression assertions for `FileBrowser` responsive action wiring
and multi-result search overlays. Extend
`apps/web/e2e/tests/task/mobile-file-tree-chat-context.spec.ts` with computed
non-wrapping/truncation geometry, two simultaneous mobile search results, and a
1280px coarse-pointer context that verifies non-overlapping 44px action slots.

## Tests

- `AC-UI-FILE-TREE-CHAT-CONTEXT-001.7`: existing touch-action component and
  mobile E2E coverage continue to assert a visible trigger and 44px menu/action
  targets.
- `AC-UI-FILE-TREE-CHAT-CONTEXT-001.9`: focused component coverage verifies
  coarse-pointer desktop action wiring and multi-result search row anchoring;
  E2E verifies non-overlapping 44px action slots on wide desktop and no-wrap,
  truncation, and per-row search actions on mobile.

## E2E tests

- `apps/web/e2e/tests/task/mobile-file-tree-chat-context.spec.ts` using the
  `mobile-chrome` project: verify long-name no-wrap/truncation and two
  row-anchored mobile search results with non-overlapping action slots, then
  create a 1280px coarse-pointer context that proves a boundary touch reaches
  the intended 44px action and can add it to chat context.
- `apps/web/e2e/tests/task/file-tree-chat-context.spec.ts`: existing desktop
  right-click flow remains unchanged and covers the desktop action path.

## Work orders

- [x] [Task 01: Restore responsive file-tree row spacing](task-01-restore-responsive-file-tree-spacing.md)

## Verification results

- Focused component coverage passed, including FileBrowser-level coarse,
  mobile, and fine-pointer rendering plus multi-result search-overlay
  anchoring.
- Mobile Chromium E2E: `pnpm e2e:run --project mobile-chrome
  tests/task/mobile-file-tree-chat-context.spec.ts` passed both Pixel 5
  no-wrap/truncation with two independently anchored search actions and 1280px
  coarse-pointer compact-row/add-to-chat scenarios.
- `make fmt` and `make lint` passed. `make typecheck` passed before the
  aggregate `make test` stage exposed unrelated ambient backend-suite failures.

## Risks

- A wide coarse-pointer device must retain the visible action rather than
  relying on desktop right-click interaction.
- The responsive hook distinguishes viewport composition from pointer precision;
  tests must cover both so a zoomed-out phone preserves touch access without
  row wrapping.
