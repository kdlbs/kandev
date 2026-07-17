---
id: task-22
title: Playwright e2e — native plugin loads into the SPA
status: done
wave: C
depends_on: [task-17, task-19, task-20, task-21]
plan: docs/plans/plugins/plan.md
---

# Playwright e2e — native plugin loads into the SPA

## Title
End-to-end: start the example plugin, register it, and assert its native nav item,
route/page, slot component, and WS-driven update appear inside kandev.

## Inputs
- `apps/web/e2e/README.md` + `fixtures/backend.ts` (backend boot, env scrubbing).
- Example plugin (task-21) server binary/script; its manifest.
- e2e profile sets `KANDEV_FEATURES_PLUGINS=true` (task-10).
- Bundle loads via `/api/plugins/{id}/bundle` proxy (task-17); registry integration
  (task-19). Secret reconciliation: register first, then push kandev's returned
  webhook_secret to the plugin (fixture/example `/_config` or example accepts it).

## Acceptance
1. `apps/web/e2e/plugins.spec.ts`:
   - boot example plugin server; `POST /api/plugins/register` with its manifest;
     configure returned secret.
   - reload SPA → the plugin bundle loads; assert a nav item "Hello" is visible.
   - navigate to `/plugins/hello` → the native plugin page renders (assert a
     plugin-owned element, e.g. `#hello-plugin-page`), styled with host `@kandev/ui`.
   - open a task detail → assert the plugin's `task-sidebar` slot component renders.
   - create a task (UI or API) → assert the plugin page's task-created counter
     increments (WS handler path works end-to-end).
   - disable the plugin in Plugins settings → nav item disappears (bundle unloaded).
2. Cleanup: uninstall plugin, stop server.

## Files
- `apps/web/e2e/plugins.spec.ts`
- `apps/web/e2e/helpers/example-plugin.ts` (spawn/stop + capture listen addr)

## Verification
- `cd apps/web && pnpm e2e -g "plugin"`

## Output contract
Report which project it runs in, secret reconciliation used, and flake mitigations.

## Dependencies
task-17, task-19, task-20, task-21.
