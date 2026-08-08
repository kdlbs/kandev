---
id: "03-task-surfaces"
title: "Desktop and mobile surfaces"
status: completed
wave: 3
depends_on: ["02-frontend-preference-state"]
plan: "plan.md"
spec: "../../specs/ui/port-forwarding-discovery.md"
---

# Task 03: Desktop and mobile surfaces

Expose the shared controller through the desktop Dockview launcher, watermark launcher, task
top-bar button, and the phone/tablet task-switcher and top-bar compositions. Preserve the existing
Port Forwarding dialog's runtime behavior while making its presentation phone-safe.

## Acceptance

- Desktop `+` and empty-group menus show a checkable Port forwarding row; enabling it persists then
  opens the existing dialog, and disabling it hides the top-bar button without stopping tunnels.
- Local and remote ready sessions have identical eligibility; the old remote-only gate is gone.
- Phone Drawer and tablet Sheet provide a labeled active-task action with a touch-sized row, and the
  enabled mobile top bar can open the same dialog without horizontal overflow or a second scroll owner.
- All new user-facing copy is translated in the task namespace and new controls have accessible
  labels/test IDs.

## Verification

- If dependencies are absent: `cd apps && pnpm install --frozen-lockfile`
- `cd apps && pnpm --filter @kandev/web test -- components/task/dockview-add-panel-items.test.tsx components/task/task-top-bar.test.tsx components/task/mobile/session-mobile-layout.test.tsx`
- `cd apps/web && pnpm run typecheck`
- `cd apps && pnpm --filter @kandev/web lint`

## Files likely touched

- `apps/web/components/task/dockview-add-panel-items.tsx`
- `apps/web/components/task/dockview-add-panel-items.test.tsx`
- `apps/web/components/task/dockview-header-actions.tsx`
- `apps/web/components/task/dockview-watermark.tsx`
- `apps/web/components/task/port-forward-dialog.tsx`
- `apps/web/components/task/task-top-bar.tsx`
- `apps/web/components/task/task-top-bar.test.tsx`
- `apps/web/components/task/task-page-inner.tsx`
- `apps/web/components/task/task-layout.tsx`
- `apps/web/components/task/mobile/session-mobile-layout.tsx`
- `apps/web/components/task/mobile/session-mobile-layout.test.tsx`
- `apps/web/components/task/mobile/session-mobile-top-bar.tsx`
- `apps/web/components/task/mobile/session-task-switcher-sheet.tsx`
- `apps/web/src/locales/en/task.json`
- `apps/web/src/locales/pt-pt/task.json`
- `apps/web/src/locales/pseudo/task.json`

## Dependencies

Task 02.

## Parallelism

Sequential. Desktop and mobile composition share the controller and dialog lifecycle.

## Inputs

- Spec sections: What, State machine, Failure modes, Scenarios.
- Mobile parity guide: use the existing Drawer/Sheet distinction, 44px rows, safe areas, and one
  internal scroll owner.
- Existing `PortForwardButton` and `PortForwardDialogContent` behavior.

## Output contract

Report the summary, actual files changed, focused component/typecheck/lint results, mobile geometry
evidence, and synchronized task/plan status. Do not add E2E coverage in this task.

## Results

- Added a checkable Port Forwarding item to Dockview's plus and empty-group launchers, using the
  shared preference controller for readiness, persistence, and toggle-and-open behavior.
- Removed the remote-only top-bar gate so local and remote ready sessions share the same control;
  disabling visibility closes only the control/dialog and leaves runtime tunnels untouched.
- Added the enabled control to the phone top bar and a 44px active-task action in the phone Drawer
  and tablet Sheet; the existing dialog now uses dynamic viewport bounds and one scroll owner.
- Hardened the Dockview launchers for routes without the task provider, ignored stale tunnel-list
  responses after session changes, and reset the dialog when agentctl readiness is lost.
- Verification: `rtk pnpm exec vitest run components/task/dockview-add-panel-items.test.tsx
  components/task/task-top-bar.test.tsx components/task/mobile/session-mobile-layout.test.tsx
  components/task/port-forwarding-visibility-provider.test.tsx components/task/port-forward-dialog.test.tsx`
  — passed (43 tests across 5 files).
- Verification: `rtk pnpm run typecheck` — passed.
- Verification: `rtk pnpm --filter @kandev/web lint` — passed.
