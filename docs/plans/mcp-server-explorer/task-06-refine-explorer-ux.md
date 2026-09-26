---
id: "06-refine-explorer-ux"
title: "Refine explorer navigation and layout"
status: done
wave: 2
depends_on: ["05-capture-tool-definitions"]
plan: "plan.md"
spec: "../../specs/platform/requirements/mcp-session-observability.md"
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

Implemented a shared three-level explorer model. Desktop keeps the server rail
visible and switches the main pane between the tools page and one tool page.
Phone and coarse-pointer tablet surfaces show one level at a time: servers,
tools, then one tool. Back returns from the tool to its tools and then to the
server list on touch surfaces.

The precise-pointer tooltip again lists every current server with its status
dot, name, localized status, and summary. Active Kandev servers use the green
status color. The desktop `DialogContent` now disables its built-in close
button because the fixed explorer header owns the single 44px close control.

The tools page has one constrained scroll owner. Tool rows show the tool name
and localized `~N tokens` estimate without the long description. Selecting a
row stores its scroll offset. Back restores the offset and keyboard focus.
Live evidence preserves valid selections, returns to the tools page when a
selected tool disappears, and falls back to Kandev when a selected server
disappears.

Connection transport, target, ID, and observation time now use a compact
disclosure. The tool page renders the full plain-text description, common
object properties as argument rows, and JSON for nested or composed schemas.
It distinguishes **No arguments** from **Schema too large to display**.

Added all copy to the six task catalogs. Pseudo and Traditional Chinese files
were generated with the repository scripts. No status, server name, tool name,
description, or schema data is translated.

The RED view-model run failed three expected assertions. The RED component run
failed four expected flows. The final focused run passed 31 tests in three
files. Verification passed:

```text
pnpm install --frozen-lockfile
pnpm --filter @kandev/web exec vitest run components/task/chat/mcp-explorer components/task/chat/chat-input-toolbar.test.tsx
pnpm run typecheck
pnpm run i18n:check
pnpm run i18n:ratchet
pnpm exec eslint components/task/chat/mcp-explorer
```

Browser geometry, safe-area, and production-build behavior remain assigned to
Task 07.
