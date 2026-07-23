---
id: app-status-bar-01
title: Stable plugin status slots
status: done
wave: 1
depends_on: []
plan: docs/plans/app-status-bar/plan.md
---

# Stable plugin status slots

## Inputs

[Spec: plugin slots](../../specs/ui/app-status-bar.md#plugin-slots); [frozen API](../plugins/PLUGIN-API.md#app-status-bar-slots); existing chat-input typed wrapper.

## Files

- `apps/web/lib/plugins/types.ts`
- `apps/web/lib/plugins/registry.ts`
- `apps/web/components/plugins/plugin-slot.tsx`
- `apps/web/components/plugins/plugin-slot.test.tsx`
- `apps/web/components/app-status-bar/app-status-bar-plugin-slots.tsx`
- `apps/web/components/app-status-bar/app-status-bar-plugin-slots.test.tsx`

## Acceptance

1. Export exact `AppStatusBarSlotProps`; left/right wrappers forward current context, presentation, density, and placement to their matching slots.
2. Registry exposes stable owned registrations (`registrationId`, `pluginId`, `Component`) while old `getSlotComponents` callers remain compatible.
3. Error boundaries use stable registration identity and owner/slot context; removing a failed first plugin cannot poison a healthy following plugin.

## Verification

```sh
cd apps && pnpm --filter @kandev/web test -- components/plugins/plugin-slot.test.tsx components/app-status-bar/app-status-bar-plugin-slots.test.tsx
```

## Output contract

Report changed files, red/green commands, compatibility result, and blockers. Mark task frontmatter and plan row `done` only after tests pass.
