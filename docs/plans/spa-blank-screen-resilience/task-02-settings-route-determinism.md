---
id: "02-settings-route-determinism"
title: "Make Settings route props synchronous"
status: done
wave: 1
depends_on: []
plan: "plan.md"
decision: "../../decisions/2026-07-27-spa-failure-containment-and-deployment-recovery.md"
---

# Task 02: Make Settings Route Props Synchronous

## Acceptance

- Table-driven RED tests cover all nine SPA-mounted dynamic Settings routes and
  prove their current props are thenables or their components suspend.
- `settings-routes.tsx` passes decoded `pluginId`, executor/profile IDs,
  executor type, workspace ID, and automation ID as synchronous props.
- Six `use(params)` consumers and three async workspace/automation wrappers
  become synchronous client-side route views.
- `/settings/executor/new` resolves to `ExecutorCreatePage`, not the dynamic
  executor-id route.
- No migrated route creates a promise during render or relies on a generic
  promise cache.
- Settings bootstrap and unrelated page-level null states remain unchanged.

## Files likely touched

- `apps/web/src/settings-routes.tsx`
- `apps/web/src/settings-routes.test.ts`
- `apps/web/app/settings/plugins/[pluginId]/page.tsx`
- `apps/web/app/settings/executor/[id]/page.tsx`
- `apps/web/app/settings/executor/[id]/profile/[profileId]/page.tsx`
- `apps/web/app/settings/executors/[profileId]/page.tsx`
- `apps/web/app/settings/executors/new/[type]/page.tsx`
- `apps/web/app/settings/executors/ssh/[executorId]/page.tsx`
- `apps/web/app/settings/workspace/[id]/page.tsx`
- `apps/web/app/settings/workspace/[id]/automations/new/page.tsx`
- `apps/web/app/settings/workspace/[id]/automations/[automationId]/page.tsx`

## Verification

```bash
cd apps/web
pnpm test -- src/settings-routes.test.ts
pnpm run typecheck
```

## Output contract

Report the synchronous prop contract, all migrated routes, reserved-path
behavior, RED/GREEN results, files changed, and deferred null/bootstrap paths.
The primary session updates this task and `plan.md` after accepting the result.
