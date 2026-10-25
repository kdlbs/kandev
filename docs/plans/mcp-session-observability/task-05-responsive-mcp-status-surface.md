---
id: "05-responsive-mcp-status-surface"
title: "Build the responsive MCP status surface"
status: pending
wave: 5
depends_on: ["03-persist-attachment-reports", "04-release-diagnostic-operations"]
plan: "plan.md"
spec: "../../specs/platform/requirements/mcp-session-observability.md"
---

# Task 05: Build the responsive MCP status surface

## Acceptance

- The toolbar reads the active Kandev session report and never synthesizes
  Active status from profile configuration or a fetch fallback.
- The plug trigger remains neutral in all states.
- Desktop hover and keyboard focus open a compact status popover; click pins
  it; second click, outside interaction, and Escape close it accessibly.
- Touch/coarse-pointer input opens an inset bottom Drawer from a minimum 44px
  trigger and exposes every desktop status and diagnostic action.
- Rows render the exact green/amber/red/gray semantics from the spec and show
  explicit uncertainty for third-party servers.
- Test endpoint, recent output, copy diagnostics, settings, and eligible reset
  actions behave as specified. Recent output requires explicit activation and
  is absent from copied diagnostics.
- Long server names and summaries wrap without document overflow; the
  popover/drawer owns bounded internal scrolling.

## Verification

- `cd apps && pnpm --filter @kandev/web exec vitest run hooks/domains/session/use-session-mcp.test.ts components/task/chat/mcp-status components/task/chat/chat-input-toolbar.test.tsx`
- `cd apps/web && pnpm run typecheck`

Write RED component tests for store truth, neutral icon, focus opening, pinning,
status colors/labels, diagnostic-copy exclusion, touch Drawer selection, and
44px geometry before implementation. Use keyboard focus in jsdom; reserve real
pointer hover and touch for Task 07.

## Files likely touched

- `apps/web/hooks/domains/session/use-session-mcp.ts`
- `apps/web/hooks/domains/session/use-session-mcp.test.ts`
- `apps/web/components/task/chat/chat-input-toolbar-primitives.tsx`
- `apps/web/components/task/chat/chat-input-toolbar-desktop.tsx`
- `apps/web/components/task/chat/chat-input-toolbar-mobile.tsx`
- `apps/web/components/task/chat/chat-input-toolbar.test.tsx`
- `apps/web/components/task/chat/mcp-status/mcp-status-trigger.tsx`
- `apps/web/components/task/chat/mcp-status/mcp-status-content.tsx`
- `apps/web/components/task/chat/mcp-status/mcp-status-row.tsx`
- `apps/web/components/task/chat/mcp-status/mcp-status-view-model.ts`
- `apps/web/components/task/chat/mcp-status/mcp-status-view-model.test.ts`
- `apps/web/lib/api/domains/session-api.ts`

## Dependencies

- Task 03 provides store hydration and live reports.
- Task 04 provides test and recent-output operations.

## Parallelism

Sequential. Task 07 validates this surface in a production build.

## Inputs

- Mobile-parity Drawer/Popover contract
- `useTouchDrawer`
- Existing chat desktop/mobile toolbar composition
- Existing reset-context confirmation and MCP settings route

## Output contract

Report the shared view model, desktop state machine, mobile Drawer geometry,
accessibility behavior, diagnostic privacy behavior, RED/GREEN results, files
changed, blockers, and risks. Mark this task `done` and update its plan
checkbox.
