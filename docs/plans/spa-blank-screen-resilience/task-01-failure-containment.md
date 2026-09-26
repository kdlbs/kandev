---
id: "01-failure-containment"
title: "Contain SPA failures and show route loading"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/mobile-task-navigation.md"
decision: "../../decisions/2026-07-27-spa-failure-containment-and-deployment-recovery.md"
---

# Task 01: Contain SPA Failures and Show Route Loading

## Acceptance

- RED tests prove Settings/Office suspension currently renders no status and an
  uncaught route/root render failure has no first-party recovery surface.
- A root boundary outside `StateProvider` renders a full-viewport `role="alert"`
  and native Reload action for shell/provider failures.
- A route boundary inside `AppShell` preserves shell navigation, reports a
  route alert, and resets only after an errored route's pathname/search changes.
- Settings and Office suspension renders one accessible visible status.
- Recovery actions are mobile-safe, at least 44px high, and raw error text is
  not shown.
- Plugin-specific boundaries remain the narrower containment layer.

## Files likely touched

- `apps/web/src/main.tsx`
- `apps/web/src/spa-routes.tsx`
- `apps/web/src/app-error-boundary.tsx` (new)
- `apps/web/src/app-error-boundary.test.tsx` (new)
- `apps/web/src/spa-routes.loading.test.tsx` (new)

## Verification

```bash
cd apps/web
pnpm test -- src/app-error-boundary.test.tsx src/spa-routes.loading.test.tsx
pnpm run typecheck
```

## Output contract

Report RED and GREEN evidence, boundary ownership/reset behavior, files
changed, test results, and risks. The primary session updates this task and
`plan.md` after accepting the result.
