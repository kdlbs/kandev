---
id: app-status-bar-11
title: Stable status item identities
status: done
wave: 6
depends_on: []
plan: docs/plans/app-status-bar/plan.md
spec: docs/specs/ui/app-status-bar.md
---

# Stable status item identities

## Inputs

[Spec: plugin slots](../../specs/ui/app-status-bar.md#plugin-slots),
[portable-order ADR](../../decisions/2026-07-21-portable-status-bar-order.md),
and existing owned slot registrations in `lib/plugins/registry.ts`.

## Files likely touched

- `apps/web/lib/plugins/registry.ts`, `registry.test.ts`
- `apps/web/lib/plugins/types.ts`
- `apps/web/components/plugins/plugin-slot.tsx`, `plugin-slot.test.tsx`
- `apps/web/components/app-status-bar/app-status-bar-plugin-slots.tsx`
- `apps/web/components/app-status-bar/app-status-bar-plugin-slots.test.tsx`
- New pure status-item projection/reconciliation module and unit test beside the
  app-status-bar components.

## Acceptance

1. Built-ins have reserved identities and every plugin contribution exposes a
   deterministic ordering identity derived from plugin ID, original slot, and
   slot-local registration ordinal; existing component/error-boundary behavior
   and `getSlotComponents` compatibility remain intact.
2. Reconciliation filters duplicate active identities, retains unknown saved
   identities, restores re-enabled registrations, and appends newly seen items
   to their default side.
3. The host treats one component registration as one opaque item and never
   introspects or reorders its rendered children.

## Verification

```sh
cd apps && pnpm --filter @kandev/web test -- lib/plugins/registry.test.ts components/plugins/plugin-slot.test.tsx components/app-status-bar/app-status-bar-plugin-slots.test.tsx components/app-status-bar
cd apps/web && pnpm run typecheck
```

## Dependencies

None; this can run in parallel with task 10.

## Output contract

Report identity/reconciliation rules, compatibility evidence, files changed,
red and green tests, and blockers. Mark the task and plan row done only after
focused verification passes.
