---
id: "02-desktop-warning-surfaces"
title: "Desktop warning surfaces"
status: done
wave: 2
depends_on: ["01-connection-issue-timing"]
plan: "plan.md"
spec: "../../specs/ui/requirements/ws-connectivity-warning.md"
---

# Task 02: Desktop warning surfaces

## Acceptance

- With App status bar enabled, its connection item has no healthy/grace content, shows the exact
  yellow/red accessible details during an issue, and keeps its stable saved item identity.
- With App status bar disabled, one focusable indicator appears immediately after the sidebar theme
  control only during an issue, in both expanded and collapsed sidebar layouts.
- The two desktop/tablet surfaces are mutually exclusive, expose details on hover and keyboard
  focus, and clear immediately on recovery.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- components/app-status-bar/connection-status-item.test.tsx components/app-status-bar/app-status-bar.test.tsx components/app-sidebar/app-sidebar-footer.test.tsx
cd apps/web && pnpm run typecheck
```

## Files likely touched

- `apps/web/components/app-status-bar/connection-status-item.tsx`
- `apps/web/components/app-status-bar/connection-status-item.test.tsx`
- `apps/web/components/app-status-bar/app-status-items.tsx`
- `apps/web/components/app-status-bar/app-status-bar.test.tsx`
- `apps/web/components/app-sidebar/app-sidebar-footer.tsx`
- `apps/web/components/app-sidebar/app-sidebar-footer.test.tsx`

## Dependencies

Task 01.

## Parallelism

Sequential. Shared connection detail/presentation contracts are also consumed by Task 03.

## Inputs

- Spec: desktop/tablet bullets, accessibility requirements, and desktop scenarios.
- Plan: `Frontend > Desktop and tablet presentation`.
- Existing patterns: `ConnectionStatusItem`, `FooterIconButton`, and the app-status item
  `empty:hidden` wrapper.

## Risks

- Removing the healthy visual must not remove `builtin:connection` from saved ordering.
- A non-action warning must not be implemented as a misleading button; use semantic status/focus
  behavior with the existing tooltip pattern.

## Output contract

Report surface behavior, files changed, exact commands and results, blockers/risks, then mark this
task `done` and update its checkbox in `plan.md`.
