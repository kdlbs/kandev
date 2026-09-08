---
id: "02-virtualize-file-tree"
title: "Virtualize the large file tree"
status: done
wave: 2
depends_on:
  - "01-stabilize-file-tree-rows"
plan: "plan.md"
spec: "../../specs/ui/requirements/file-tree-chat-context.md"
system_design: "../../specs/ui/system-design/task-surface-render-isolation.md"
---

# Task 02: Virtualize the large file tree

## Acceptance

- A tree with at least 600 visible entries mounts fewer than 80 row components at one time.
- The current file-tree viewport remains the only vertical scroll owner.
- Scrolling can reach and open the first and last generated entries.
- Active-file reveal mounts and shows an offscreen target before DOM-based focus work.
- Expansion, filtering, selection, drag and drop, rename, and context actions keep their behavior.
- Mobile rows keep a visible 44 CSS pixel action and a viewport-contained responsive menu.

This task preserves `AC-UI-FILE-TREE-CHAT-CONTEXT-001.1`, `.2`, and `.7`. It also preserves
`AC-UI-MOBILE-TASK-NAVIGATION-001.1`, `.3`, and `.7`.

## TDD sequence

1. Add an E2E fixture that creates at least 600 deterministic file entries.
2. Add a failing desktop test for the mounted-row bound and last-row access.
3. Add a failing mobile test for the row bound, touch action, and file opening.
4. Add `useVirtualizer` to `FileTreeView` with measurement and overscan.
5. Update reveal and restoration flows for offscreen rows.
6. Run the existing mobile context-action test with the new E2E cases.

## Verification

```bash
rtk make build-web
cd apps/web
rtk pnpm e2e:run --no-build --project chromium tests/task/large-file-tree-virtualization.spec.ts
rtk pnpm e2e:run --no-build --project mobile-chrome tests/task/mobile-large-file-tree-virtualization.spec.ts tests/task/mobile-file-tree-chat-context.spec.ts
rtk pnpm run lint:e2e-sleeps -- e2e/tests/task/large-file-tree-virtualization.spec.ts e2e/tests/task/mobile-large-file-tree-virtualization.spec.ts
```

## Files likely touched

- `apps/web/components/task/file-browser-parts.tsx`
- File-browser reveal or scroll helpers, if required
- `apps/web/e2e/tests/task/large-file-tree-virtualization-helpers.ts`
- `apps/web/e2e/tests/task/large-file-tree-virtualization.spec.ts`
- `apps/web/e2e/tests/task/mobile-large-file-tree-virtualization.spec.ts`

## Dependencies

- Task 01 supplies stable row callbacks and a memoized row boundary.

## Inputs

- `REQ-UI-FILE-TREE-CHAT-CONTEXT-001`
- `REQ-UI-MOBILE-TASK-NAVIGATION-001`
- Existing Kanban virtual-list implementation and large-column E2E fixture
- Existing mobile file-tree context-action E2E coverage

## Output contract

Report the scroll-element choice, estimate, measurement strategy, overscan, mounted-row count,
changed files, E2E results, and remaining edge cases.
