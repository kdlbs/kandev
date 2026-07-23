---
id: app-status-bar-12
title: Reorder interaction and alignment
status: done
wave: 7
depends_on: [app-status-bar-10, app-status-bar-11]
plan: docs/plans/app-status-bar/plan.md
spec: docs/specs/ui/app-status-bar.md
---

# Reorder interaction and alignment

## Inputs

[Spec: What](../../specs/ui/app-status-bar.md#what), tasks 10-11, and the live
geometry diagnosis: `border-top` currently leaves a 23 px content box and puts
12 px plugin text at a half-pixel vertical position.

## Files likely touched

- `apps/web/components/app-status-bar/app-status-bar.tsx`, `.test.tsx`
- `apps/web/components/app-status-bar/app-status-drawer.tsx` and focused tests
- `apps/web/components/app-status-bar/app-status-bar-plugin-slots.tsx`
- New reorder hook/controller and pure movement helpers beside the status bar
- `apps/web/components/app-status-bar/connection-status-item.tsx`
- `apps/web/components/system-metrics/status-surface-metrics.tsx`
- `apps/web/lib/user-settings-sync.ts` or the existing queued-sync call pattern

## Acceptance

1. The separator is inset/overlaid so all content uses the full 24 px alignment
   box; representative dot, plugin text, and metric roots share the same center
   at 1x scale.
2. Cmd-drag on macOS and Ctrl-drag elsewhere reorders any built-in/plugin item
   horizontally across items and the spacer. Reorder begins only after a horizontal
   movement threshold, suppresses only the completed drag click, and never starts
   from plain mouse or touch input.
3. A successful drop optimistically changes order and persists through the queued
   user-settings PATCH. Exhausted failure restores the confirmed order and reports
   an error. Plugin interactive children remain directly usable without nested
   host buttons or added tab stops.
4. Phone renders the active saved left sequence followed by the active saved right
   sequence as vertical rows, creates no drag listeners, and preserves metrics
   subscription/focus/safe-area behavior.

## Verification

```sh
cd apps && pnpm --filter @kandev/web test -- components/app-status-bar components/system-metrics lib/user-settings-sync.test.ts
cd apps/web && pnpm run typecheck
```

## Dependencies

Tasks 10 and 11.

## Output contract

Report pointer-state rules, persistence/failure behavior, measured before/after
geometry, phone projection, files changed, tests, and blockers. Mark the task and
plan row done only after focused verification passes.
