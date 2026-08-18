---
id: "06-refine-explorer-ux"
title: "Refine explorer navigation and layout"
status: pending
wave: 2
depends_on: ["05-capture-tool-definitions"]
plan: "plan.md"
spec: "../../specs/mcp-session-observability/spec.md"
---

# Task 06: Refine Explorer Navigation and Layout

## Acceptance

- Hover and keyboard focus show the rich server-status tooltip on precise
  pointers. An Active Kandev server has a green status dot.
- The desktop dialog contains one accessible close control.
- The server view opens a scrollable tools page. A tool row opens a focused
  tool page with its description and arguments.
- Back restores the tool-list scroll position and focus.
- Connection metadata uses a compact disclosure. The tools page gets most of
  the available height.
- Desktop, tablet, and phone surfaces have one active scroll owner and no
  document overflow.
- Token values use `~N tokens` and explain the estimator once.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile && pnpm --filter @kandev/web exec vitest run components/task/chat/mcp-explorer components/task/chat/chat-input-toolbar.test.tsx && cd web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet
```

Write failing view-model and component tests first. Cover the tooltip, close
control, page transitions, schema states, live catalog changes, scroll return,
and focus return.

## Files likely touched

- `apps/web/lib/types/session-runtime-payloads.ts`
- `apps/web/lib/state/slices/session-runtime/types.ts`
- `apps/web/components/task/chat/mcp-explorer/mcp-indicator.tsx`
- `apps/web/components/task/chat/mcp-explorer/mcp-server-explorer.tsx`
- `apps/web/components/task/chat/mcp-explorer/mcp-server-list.tsx`
- `apps/web/components/task/chat/mcp-explorer/mcp-server-detail.tsx`
- `apps/web/components/task/chat/mcp-explorer/mcp-tool-list.tsx`
- `apps/web/components/task/chat/mcp-explorer/mcp-tool-detail.tsx`
- `apps/web/components/task/chat/mcp-explorer/mcp-explorer-view-model.ts`
- `apps/web/components/task/chat/mcp-explorer/mcp-explorer-view-model.test.ts`
- `apps/web/components/task/chat/mcp-explorer/mcp-server-explorer.test.tsx`
- `apps/web/src/locales/*/task.json`

## Dependencies

Task 05 supplies schemas, estimates, truncation state, and estimator metadata.

## Parallelism

Sequential. This task owns shared explorer components and locale files.

## Inputs

- Spec section `User experience` and its scenarios.
- Mobile UI language rules for focused page navigation and scroll ownership.
- The previous rich tooltip in commit `85492e6f8^`.
- The shared dialog `showCloseButton` contract.

## Output contract

Report the desktop and touch page models, scroll ownership, accessibility
behavior, localized copy, files, tests, blockers, and risks. Update this task
and the plan status in the same session.

## Results

Pending.
