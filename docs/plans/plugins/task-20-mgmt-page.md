---
id: task-20
title: Plugins management page + API client + slice
status: done
wave: B
depends_on: [task-18, task-17]
plan: docs/plans/plugins/plan.md
---

# Plugins management page + API client + slice

## Title
Operator UI to register/list/enable/disable/uninstall plugins, gated on the
`plugins` feature flag.

## Inputs
- Backend API (task-17). Feature flag (task-10) via
  `apps/web/hooks/domains/features/use-feature.ts`.
- Conventions: API client `apps/web/lib/api/domains/jira-api.ts`; slice
  `apps/web/lib/state/slices/jira/` + registration in `lib/state/slices/index.ts`
  and `lib/state/store.ts`; settings page `app/settings/integrations/jira/page.tsx`
  registered in `src/settings-routes.tsx` + settings nav.

## Acceptance
1. `apps/web/lib/api/domains/plugins-api.ts` (+ test): list/get/register/updateConfig/
   enable/disable/uninstall/listTools.
2. `apps/web/lib/state/slices/plugins/` (+ test): `plugins[]`, loading/error;
   loadPlugins/upsert/remove; wired into store.
3. `apps/web/app/settings/plugins/page.tsx`: table with name/id/status badge/
   categories; enable/disable/uninstall (confirm on uninstall); "Register" dialog
   (manifest YAML textarea → registerPlugin, show api_key+webhook_secret once with
   copy + "shown once" warning). Route + nav registered; gated on
   `useFeature("plugins")`. Enable/disable triggers a plugin bundle reload/unload
   (call task-18 `loadPlugins`/`unloadPlugin` for the affected plugin, or reload).
4. Loading/empty/error states; `/mobile-parity` responsive.

## Files
- `apps/web/lib/api/domains/plugins-api.ts` + `plugins-api.test.ts`
- `apps/web/lib/state/slices/plugins/{plugins-slice.ts,types.ts,plugins-slice.test.ts}`
- edits: `lib/state/slices/index.ts`, `lib/state/store.ts`, `src/settings-routes.tsx`, settings nav
- `apps/web/app/settings/plugins/page.tsx` (+ `page.test.tsx` for action wiring)

## Verification
- `cd apps/web && pnpm run typecheck && pnpm lint && pnpm test -- plugins`

## Output contract
Report client methods, slice shape, route/nav wiring, and how enable/disable
reloads the plugin bundle at runtime.

## Dependencies
task-18, task-17.
