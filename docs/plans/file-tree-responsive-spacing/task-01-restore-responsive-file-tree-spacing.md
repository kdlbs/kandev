---
id: "01-restore-responsive-file-tree-spacing"
title: "Restore responsive file-tree row spacing"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-FILE-TREE-CHAT-CONTEXT-001
acceptance_criteria:
  - AC-UI-FILE-TREE-CHAT-CONTEXT-001.7
  - AC-UI-FILE-TREE-CHAT-CONTEXT-001.9
system_design:
  - ../../specs/ui/system-design/file-tree-chat-context.md
---

# Task 01: Restore responsive file-tree row spacing

## Summary

Ensure the Files panel keeps compact single-line rows on every responsive
composition. Preserve the 44px touch action and responsive menu on
coarse-pointer devices, including desktop-workbench and zoomed-out layouts.

## In scope

- Correct the responsive presentation gate in `FileBrowser`.
- Remove row wrapping that inflates file-tree spacing.
- Add focused component regression coverage and mobile Playwright geometry
  coverage without changing the existing context action flow.

## Out of scope

- File-tree data, filesystem operations, context-file storage, or chat sending.
- New responsive primitives, breakpoints, or desktop panel redesign.

## Acceptance

- Coarse-pointer file trees, including wide desktop workbench viewports, retain
  the visible touch action with its 44px minimum active hitbox.
- Tree and search-result rows remain compact and non-wrapping; long names
  truncate within reserved action space, and responsive rows provide an
  exclusive 44px vertical action slot.
- Existing desktop and mobile file-tree context actions continue to pass.

## Verification

```bash
(cd apps && pnpm --filter @kandev/web test -- --run components/task/file-browser-context-action.test.tsx components/task/file-browser-responsive.test.tsx components/task/file-browser-search-context-action.test.tsx)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-file-tree-chat-context.spec.ts)
```

## Files likely touched

- `apps/web/components/task/file-browser.tsx`
- `apps/web/components/task/file-browser-parts.tsx`
- `apps/web/components/task/file-browser-context-action.test.tsx`
- `apps/web/components/task/file-browser-responsive.test.tsx`
- `apps/web/components/task/file-browser-search-context-action.test.tsx`
- `apps/web/e2e/fixtures/test-base.ts`
- `apps/web/e2e/tests/task/mobile-file-tree-chat-context.spec.ts`

## Dependencies

None.

## Risks

- A coarse-pointer desktop must retain the visible action in a 44px row slot so
  adjacent triggers cannot overlap.
- Each search result must anchor its overlay independently, reserve its action
  space, and truncate its label.
- Geometry assertions cover non-overlapping action slots and a boundary touch
  that opens the intended row action.

## Parallelism

`sequential`

## Inputs

- `AC-UI-FILE-TREE-CHAT-CONTEXT-001.7` and `AC-UI-FILE-TREE-CHAT-CONTEXT-001.9`.
- `docs/specs/ui/system-design/file-tree-chat-context.md`.
- Existing file-tree component and Playwright tests.

## Results

- Focused component coverage passed, including FileBrowser-level wide
  coarse-pointer and mobile action visibility, fine-pointer hiding, no-wrap
  tree rows, and independently anchored multi-result search actions.
- Mobile Chromium E2E passed Pixel 5 long-name no-wrap/truncation with two
  clipped, row-anchored search actions and a 1280px coarse-pointer desktop
  context with compact short and long rows, a 44px trigger, and Add to chat
  context.
- `make fmt` and `make lint` passed. The aggregate `make test` stage remains
  blocked by unrelated ambient backend-suite failures.
