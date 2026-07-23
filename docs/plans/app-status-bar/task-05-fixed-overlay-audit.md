---
id: app-status-bar-05
title: Offset fixed desktop overlays
status: done
wave: 3
depends_on: [app-status-bar-03]
plan: docs/plans/app-status-bar/plan.md
---

# Offset fixed desktop overlays

## Inputs

[Spec: responsive/layout](../../specs/ui/app-status-bar.md#responsive-and-layout-contract); task 03 CSS variable; current overlay z-index layers.

## Files

- `apps/web/components/toast-provider.tsx`
- `apps/web/components/config-chat/config-chat-panel.tsx`
- `apps/web/components/kanban/task-multi-select-toolbar.tsx`
- `apps/web/components/kanban-with-preview.tsx`
- `apps/web/components/review/walkthrough-overlay.tsx`
- `apps/web/components/diff/walkthrough-floating-window.tsx`
- Focused component/layout tests where coverage exists.

## Acceptance

1. Each audited desktop fixed control clears the 24 px bar through `--app-status-bar-height` and retains its current phone `bottom: 0` behavior.
2. Preview and walkthrough surfaces retain usable z-index relative to bar, drawer, and modal layers.
3. No global fixed-element padding rule or unrelated phone offset is introduced.

## Verification

```sh
cd apps && pnpm --filter @kandev/web test -- components/kanban components/review components/diff
```

## Output contract

Report audited offsets/z-indexes, desktop and phone rendered checks, tests, blockers, and task status.
