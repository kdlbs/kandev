---
id: "02-build-responsive-explorer"
title: "Build the responsive explorer"
status: done
wave: 2
depends_on: ["01-capture-tool-catalog"]
plan: "plan.md"
spec: "../../specs/platform/requirements/mcp-session-observability.md"
---

# Task 02: Build the Responsive Explorer

## Acceptance

- A click on the desktop MCP trigger opens a wide, accessible server dialog.
  Selecting Kandev shows the current tool names and descriptions.
- A phone tap opens a full-height drawer. Server selection opens one focused
  detail view with a visible Back control and one scroll owner.
- Third-party rows show safe status metadata and explain the catalog limit.
  All user copy uses the `task` locale namespace.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps && pnpm --filter @kandev/web exec vitest run components/task/chat/mcp-explorer components/task/chat/chat-input-toolbar.test.tsx
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run i18n:check
cd apps/web && pnpm run i18n:ratchet
```

Write failing view-model and component tests before the implementation. Use
keyboard events for focus, Escape, selection, Back, and focus return.

## Files likely touched

- `apps/web/lib/types/session-runtime-payloads.ts`
- `apps/web/lib/state/slices/session-runtime/types.ts`
- `apps/web/components/task/chat/chat-input-toolbar-primitives.tsx`
- `apps/web/components/task/chat/chat-input-toolbar.test.tsx`
- `apps/web/components/task/chat/mcp-explorer/mcp-indicator.tsx`
- `apps/web/components/task/chat/mcp-explorer/mcp-server-explorer.tsx`
- `apps/web/components/task/chat/mcp-explorer/mcp-server-list.tsx`
- `apps/web/components/task/chat/mcp-explorer/mcp-server-detail.tsx`
- `apps/web/components/task/chat/mcp-explorer/mcp-explorer-view-model.ts`
- `apps/web/components/task/chat/mcp-explorer/mcp-explorer-view-model.test.ts`
- `apps/web/components/task/chat/mcp-explorer/mcp-server-explorer.test.tsx`
- `apps/web/src/locales/en/task.json`
- `apps/web/src/locales/pseudo/task.json`
- `apps/web/src/locales/pt-pt/task.json`
- `apps/web/src/locales/zh-cn/task.json`
- `apps/web/src/locales/zh-hk/task.json`
- `apps/web/src/locales/zh-tw/task.json`

## Dependencies

Task 01 provides the catalog fields and bounds.

## Parallelism

Sequential. This task owns the shared explorer components and locale files.

## Inputs

- Spec section `User experience` and related scenarios.
- Mobile UI language rules for master-detail and dense content.
- Existing GitHub connection dialog and full-height Azure detail drawer.

## Output contract

Report the desktop and touch compositions, shared state, accessibility behavior,
localized copy, files changed, tests run, blockers, and risks. Update this task
and the plan status in the same session.

## Results

Implemented a shared MCP explorer view model and responsive server surfaces.
Desktop uses a wide two-pane dialog. Touch devices use a full-height drawer
with a list view, focused detail view, visible Back control, safe-area padding,
and one active scroll owner. Selection defaults to `kandev` and falls back when
live session updates remove the selected server.

The detail view renders the bounded Kandev catalog as plain text and explains
why third-party catalogs are unavailable. The trigger keeps a short localized
tooltip on desktop and a 44px touch target. New copy is present in all six
task catalogs and the old compact indicator was removed from toolbar
primitives.

Verification:

```text
pnpm install --frozen-lockfile: passed
Vitest: 5 tests passed in 3 files
typecheck: passed
i18n:check: passed, with the repository's existing advisory real-locale parity warnings
i18n:ratchet: passed, 0 added + 5 modified files clean
```
