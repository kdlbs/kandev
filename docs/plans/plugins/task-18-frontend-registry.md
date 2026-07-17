---
id: task-18
title: Frontend Plugin API — globals + registry + host/loader
status: pending
wave: A
depends_on: []
plan: docs/plans/plugins/plan.md
---

# Frontend Plugin API — globals + registry + host/loader

## Title
Implement the native-JS-plugin runtime: the `window.registerKandevPlugin` global,
the reactive `PluginRegistry`, the `PluginHostApi`, and the loader that imports
plugin bundles from the boot payload.

## Inputs
- **`docs/plans/plugins/PLUGIN-API.md` is the frozen contract — implement it
  exactly** (types, method names, slot names, loading sequence, destroy/unregister).
- Store access: `createAppStore`/`useAppStoreApi` in
  `apps/web/components/state-provider.tsx` (there is an existing
  `window.__KANDEV_E2E_STORE__` global to mirror the pattern). The host needs the
  `StoreApi<AppState>` — expose it to plugins via `host.store`.
- Boot payload: `apps/web/src/boot-payload.ts` `BootPayload` type — add
  `plugins?: ActivePlugin[]` (`{id,name,bundleUrl,styleUrls?}`).
- React sharing: plugins use `host.React` (host instance). Provide `host.jsx` =
  `React.createElement`.
- `@kandev/ui` (workspace pkg) — expose a curated subset on `host.ui`
  (Button, Card, Badge, and a couple layout primitives; import what exists).

## Acceptance
1. `apps/web/lib/plugins/registry.ts`: reactive singleton `PluginRegistry`
   implementing the contract (register* + get* selectors), tracking owner pluginId,
   with `unregisterPlugin(id)` bulk-revoke. Backed by a tiny store/emitter so React
   can subscribe (`usePluginRegistry()` hook returning a snapshot).
2. `apps/web/lib/plugins/host.ts`: `installPluginGlobal(hostFactory)` defines
   `window.registerKandevPlugin`; `loadPlugins(bootPlugins, hostApi)` injects style
   links, dynamically `import(/* @vite-ignore */ bundleUrl)` each bundle, then calls
   the registered plugin's `initialize(registry, host)` inside try/catch (a failure
   is logged, never throws to boot). `unloadPlugin(id)` calls `destroy?()` +
   `registry.unregisterPlugin(id)`.
3. `apps/web/lib/plugins/host-api.ts`: builds `PluginHostApi` for a pluginId
   (React, jsx, store, api.fetch scoped to `/api/plugins/{id}`, ui subset, theme).
4. `apps/web/src/boot-payload.ts`: `ActivePlugin` type + `plugins` field parsed.
5. Unit tests (vitest): registry register/get/unregister + owner revoke;
   host `loadPlugins` with a fake bundle module that calls the global (mock
   `import` via injecting the module, or structure `loadPlugins` to accept an
   injectable importer so tests don't need real dynamic import); host-api fetch
   path scoping.

## Files
- `apps/web/lib/plugins/registry.ts` + `registry.test.ts`
- `apps/web/lib/plugins/host.ts` + `host.test.ts`
- `apps/web/lib/plugins/host-api.ts` + `host-api.test.ts`
- `apps/web/lib/plugins/types.ts` (mirror PLUGIN-API.md TS types)
- edit `apps/web/src/boot-payload.ts`

## Verification
- `cd apps/web && pnpm run typecheck && pnpm test -- plugins && pnpm lint`

## Output contract
Report the exact exported API (so task-19/20/21 align), how dynamic import is made
testable (injectable importer), and the boot-payload additions. Do NOT wire into
routes/nav yet (task-19). Follow apps/web/AGENTS.md lint limits.

## Dependencies
None (contract is fixed in PLUGIN-API.md).
