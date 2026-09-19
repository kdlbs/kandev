---
id: "03-cover-responsive-flow"
title: "Cover responsive context flow"
status: done
wave: 3
depends_on: ["01-model-directory-context", "02-add-file-tree-action"]
plan: "plan.md"
spec: "../../specs/ui/requirements/file-tree-chat-context.md"
---

# Task 03: Cover Responsive Context Flow

## Intent

Prove the complete file-tree-to-chat behavior on fine-pointer desktop and Pixel 5 touch layouts, including session ownership, directory presentation, deduplication, send clearing, and mobile menu geometry.

## Acceptance

- Desktop Playwright covers right-clicking a file and directory, duplicate suppression, visible composer chips, sent-message metadata, and successful ephemeral clearing without opening the node.
- Mobile Playwright uses `.tap()` on the visible row action, verifies the 44px trigger/menu target and viewport-safe bottom menu, then observes the same directory-context send outcome in Chat.
- Page-object helpers use stable node/menu/chip selectors, and both focused managed-runner commands discover and pass the intended tests from a fresh production build.

## TDD sequence

1. Add desktop and mobile Playwright scenarios against the missing selectors/actions and confirm both fail for the intended reason.
2. Add only the stable page-object helpers or selector adjustments required by the implemented UI.
3. Run each managed E2E command with a fresh build, inspect failure/render artifacts as needed, and record final counts plus cleanup evidence.

## Files likely touched

- `apps/web/e2e/pages/session-page.ts`
- `apps/web/e2e/tests/task/file-tree-chat-context.spec.ts`
- `apps/web/e2e/tests/task/mobile-file-tree-chat-context.spec.ts`
- `apps/web/components/task/chat/context-items/context-chip.tsx` only if Task 01 did not expose the planned stable path selector
- `apps/web/components/task/file-context-menu.tsx` only if Task 02 did not expose the planned stable menu/trigger selectors

## Dependencies

Tasks 01 and 02.

## Parallelism

`sequential` — both scenarios depend on the integrated frontend behavior and share `SessionPage` helpers.

## Verification

- `cd apps && pnpm install --frozen-lockfile`
- `cd apps/web && pnpm e2e:run tests/task/file-tree-chat-context.spec.ts`
- `cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-file-tree-chat-context.spec.ts`
- `cd apps/web && pnpm exec eslint e2e/pages/session-page.ts e2e/tests/task/file-tree-chat-context.spec.ts e2e/tests/task/mobile-file-tree-chat-context.spec.ts`

## Inputs

- Every spec scenario, especially the desktop, directory, duplicate, success/failure, mobile, and bulk boundaries.
- Plan `E2E Tests` and `Mobile design contract`.
- Existing setup patterns in `apps/web/e2e/tests/file-tree-multi-select.spec.ts` and `apps/web/e2e/tests/task/mobile-file-viewer.spec.ts`.
- `/e2e` managed-runner, selector, touch-input, and production-build requirements.

## Output contract

Report RED/GREEN evidence, actual files changed, exact discovered/passed test counts, viewport/hitbox/overflow results, artifact paths and managed-runner cleanup, blockers/risks, and synchronized task/plan status in this conversation.

## Results

- RED: the first desktop managed run exposed duplicate Radix portal matches in the global
  context-menu selector. The first mobile run exposed that the desktop `files-panel` scope did
  not contain the visible mobile tree; after that was corrected, the geometry assertion caught
  the 42.3px mid-animation menu-item sample. These failures were test synchronization/scope
  issues, not missing product behavior.
- GREEN: `cd apps/web && pnpm e2e:run tests/task/file-tree-chat-context.spec.ts` — 1 test
  passed (fresh production build and managed cleanup).
- GREEN: `cd apps/web && pnpm e2e:run --project mobile-chrome
  tests/task/mobile-file-tree-chat-context.spec.ts` — 1 test passed (fresh production build and
  managed cleanup). The Pixel 5 flow used `.tap()`, verified the 44px trigger and settled menu
  item, viewport bounds, no horizontal overflow, directory chip identity, sent metadata, and
  ephemeral clearing.
- `cd apps/web && pnpm exec eslint e2e/pages/session-page.ts
  e2e/tests/task/file-tree-chat-context.spec.ts
  e2e/tests/task/mobile-file-tree-chat-context.spec.ts` passed.
- Files added/updated: the focused `FileTreePage` composed by `SessionPage`, plus the desktop and
  mobile task specs. The final managed runs removed temporary E2E results, blob reports, PR
  assets, and shard logs before execution.
- PR review remediation: the page objects now expose search input/result selectors. The desktop
  flow right-clicks a search result and verifies the existing context action remains idempotent;
  the Pixel 5 flow taps the search result's 44px overflow action in addition to the tree-row flow.
- Final PR-remediation run: the first mobile attempt timed out on the desktop-only panel scope;
  the next attempt established that workspace search returns files, not directories. After
  widening the visible page-object scope and adding a real file fixture, the final Pixel 5 run
  passed 1 test, including the 44px trigger/menu, viewport bounds, no horizontal overflow,
  directory tree-row action, file search-result action, sent metadata, and ephemeral clearing.
