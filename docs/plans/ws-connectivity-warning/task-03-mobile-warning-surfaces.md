---
id: "03-mobile-warning-surfaces"
title: "Mobile warning surfaces"
status: done
wave: 3
depends_on: ["01-connection-issue-timing", "02-desktop-warning-surfaces"]
plan: "plan.md"
spec: "../../specs/ui/requirements/ws-connectivity-warning.md"
---

# Task 03: Mobile warning surfaces

## Acceptance

- Phone Status access remains ordinary when App status bar is enabled; when it is disabled, it
  appears only for an active issue and opens a connection-only drawer with no metrics or plugin
  contributions.
- Direct Page/Office Status controls, task bottom navigation, and Home/Settings persistent menu
  triggers plus nested Status rows communicate yellow/red severity without relying on color alone.
- Existing 44 px touch targets, inset drawer containment, one internal scroll owner, safe-area
  clearance, dismissal, focus return, and no-horizontal-overflow behavior remain intact.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- components/app-status-bar/app-status-surface-provider.test.tsx components/app-status-bar/app-status-drawer.test.tsx components/kanban/kanban-header-mobile.test.tsx components/settings/settings-layout-client.test.tsx components/task/mobile/session-mobile-bottom-nav.test.tsx
cd apps/web && pnpm run typecheck
```

## Files likely touched

- `apps/web/components/app-status-bar/app-status-surface-provider.tsx`
- `apps/web/components/app-status-bar/app-status-surface-provider.test.tsx`
- `apps/web/components/app-status-bar/app-status-drawer.tsx`
- `apps/web/components/app-status-bar/app-status-drawer.test.tsx`
- `apps/web/components/kanban/kanban-header-mobile.tsx`
- `apps/web/components/kanban/kanban-header-mobile.test.tsx`
- `apps/web/components/kanban/mobile-menu-sheet.tsx`
- `apps/web/components/settings/settings-layout-client.tsx`
- `apps/web/components/settings/settings-layout-client.test.tsx`
- `apps/web/components/task/mobile/session-mobile-layout.tsx`
- `apps/web/components/task/mobile/session-mobile-bottom-nav.tsx`
- `apps/web/components/task/mobile/session-mobile-bottom-nav.test.tsx`

## Dependencies

Tasks 01 and 02.

## Parallelism

Sequential. It extends the shared details from Task 02 and the provider contract used by all mobile
callers.

## Inputs

- Spec: `Responsive and mobile contract` and phone scenarios.
- Plan: `Frontend > Phone presentation` and `Mobile design contract`.
- Existing exemplar:
  `apps/web/components/app-status-bar/app-status-drawer.tsx`; existing responsive routing in
  `AppStatusSurfaceProvider`.

## Risks

- Filtering after item render could still mount disabled metrics/plugin effects; connection-only
  selection must happen before contributions render.
- Home and Settings require warning treatment on their persistent menu triggers, not only on nested
  Status rows.

## Output contract

Report mobile composition, files changed, exact commands and results, rendered verification
evidence or blocker, blockers/risks, then mark this task `done` and update its checkbox in
`plan.md`.
