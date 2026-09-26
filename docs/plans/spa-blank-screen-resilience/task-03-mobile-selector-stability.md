---
id: "03-mobile-selector-stability"
title: "Stabilize mobile repository selectors"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/mobile-task-navigation.md"
decision: "../../decisions/2026-07-27-spa-failure-containment-and-deployment-recovery.md"
---

# Task 03: Stabilize Mobile Repository Selectors

## Acceptance

- A RED component test mounts the real store provider with a two-repository task
  while the keyed repository and task-session maps are missing.
- Repository and task-session selectors use typed module-level empty sentinels
  for both missing IDs and missing keyed collections.
- The picker renders its rows without a cached-snapshot warning, maximum update
  depth loop, or mocked Zustand selector.
- Existing hydrated repository/session behavior is unchanged.

## Files likely touched

- `apps/web/components/task/mobile/mobile-repos-section.tsx`
- `apps/web/components/task/mobile/mobile-repos-section.test.tsx` (new)

## Verification

```bash
cd apps/web
pnpm test -- components/task/mobile/mobile-repos-section.test.tsx
pnpm run typecheck
```

## Output contract

Report the exact unstable snapshots, RED/GREEN evidence with the real
`StateProvider`, files changed, and remaining selector risks. The primary
session updates this task and `plan.md` after accepting the result.
