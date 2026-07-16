---
id: task-19
title: Frontend dynamic integration — routes/nav/slots/ws + PluginSlot
status: done
wave: B
depends_on: [task-18]
plan: docs/plans/plugins/plan.md
---

# Frontend dynamic integration

## Title
Wire the plugin registry into the live app: dynamic SPA routes, nav items, slot
host component, WS handler bridge, and boot the plugin host on app start.

## Inputs
- `PLUGIN-API.md` "Integration points the app must add".
- task-18 exports: `PluginRegistry` singleton + `usePluginRegistry()`,
  `installPluginGlobal`, `loadPlugins`, `buildHostApi`.
- Seams: `apps/web/src/spa-routes.tsx` (static route resolver — add registry
  fallthrough before not-found), `apps/web/src/settings-routes.tsx`
  (`/settings/plugins/{id}/*` → registry), app sidebar nav component (grep the
  file that renders nav items, e.g. `components/app-sidebar/*`), `lib/ws/router.ts`
  + `lib/ws/client.ts` (forward decoded messages to registry WS handlers),
  app root that reads boot payload and renders providers (grep for `readBootPayload`
  / `StateProvider` mount — likely `apps/web/app/layout.tsx` or a client bootstrap).

## Acceptance
1. `apps/web/components/plugins/plugin-slot.tsx`: `<PluginSlot name props/>` renders
   all `registry.getSlotComponents(name)` (each wrapped in an error boundary).
   Mount `<PluginSlot name="task-sidebar"/>` in the task detail sidebar and
   `<PluginSlot name="settings-nav"/>` in the settings nav (minimal, additive).
2. `apps/web/components/plugins/plugin-nav-items.tsx`: `<PluginNavItems/>` renders
   `registry.getNavItems()` (section main) in the sidebar; clicking navigates to
   `item.path`.
3. SPA route resolver: unknown path → check `registry.getRoutes()`; render the
   plugin component inside the normal app shell. Settings resolver: `/settings/
   plugins/{id}/*` → registry settings routes.
4. WS bridge: incoming messages also dispatch to `registry.getWsHandlers(action)`.
5. Boot: on app start, after store + boot payload are ready,
   `installPluginGlobal(...)` then `loadPlugins(bootPayload.plugins, buildHostApi)`
   — gated on `useFeature("plugins")` / `bootPayload.plugins` present. Idempotent
   (don't double-load on re-render).
6. Tests: PluginSlot renders registered components + isolates a throwing one;
   nav items render + navigate; route resolver returns a registered plugin route;
   WS bridge forwards to a registered handler.

## Files
- `apps/web/components/plugins/plugin-slot.tsx` + test
- `apps/web/components/plugins/plugin-nav-items.tsx` + test
- `apps/web/lib/plugins/boot.ts` (`bootPlugins(bootPayload, storeApi, feature)`) + test
- edits: `src/spa-routes.tsx`, `src/settings-routes.tsx`, sidebar nav file,
  `lib/ws/router.ts`, app bootstrap file

## Verification
- `cd apps/web && pnpm run typecheck && pnpm lint && pnpm test -- plugin`
- `/mobile-parity`: nav items + slot render responsively.

## Output contract
Report each seam edited + how idempotent boot is guaranteed + slot names live.
Keep edits additive (don't regress existing routes/nav). Follow apps/web/AGENTS.md.

## Dependencies
task-18.
