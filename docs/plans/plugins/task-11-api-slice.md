---
id: task-11
title: Plugins API client + Zustand slice
status: pending
wave: 3
depends_on: [task-09]
plan: docs/plans/plugins/plan.md
---

# Plugins API client + Zustand slice

## Title
Frontend REST client for `/api/plugins/*` and a Zustand `plugins` slice with unit
tests.

## Inputs
- Backend API from task-09 (management endpoints + `GET /api/plugins/tools`).
- Client convention: `apps/web/lib/api/domains/jira-api.ts` (+ `linear-api.ts` and
  its `linear-api.test.ts`). Match fetch/error handling + typing style.
- Slice convention: `apps/web/lib/state/slices/jira/` and `linear/`. Register the
  new slice in `apps/web/lib/state/slices/index.ts` and `apps/web/lib/state/store.ts`
  (add `createPluginsSlice`).
- Types shared with backend DTOs (plugin id, display_name, status, categories,
  capabilities, tools, ui.pages).

## Acceptance
1. `apps/web/lib/api/domains/plugins-api.ts`: `listPlugins()`, `getPlugin(id)`,
   `registerPlugin(manifestYaml)`, `updatePluginConfig(id, config)`,
   `enablePlugin(id)`, `disablePlugin(id)`, `uninstallPlugin(id)`,
   `listPluginTools()`. Typed responses.
2. `apps/web/lib/state/slices/plugins/`: `plugins-slice.ts` (state: `plugins[]`,
   `loading`, `error`; actions: `loadPlugins`, `setPlugins`, `upsertPlugin`,
   `removePlugin`), `types.ts`, wired into store.
3. Unit tests: `plugins-api.test.ts` (mock fetch, assert URLs/methods/error paths),
   `plugins-slice.test.ts` (state transitions).

## Files
- `apps/web/lib/api/domains/plugins-api.ts` + `plugins-api.test.ts`
- `apps/web/lib/state/slices/plugins/plugins-slice.ts` + `types.ts` + `plugins-slice.test.ts`
- edits: `apps/web/lib/state/slices/index.ts`, `apps/web/lib/state/store.ts`

## Verification
- `cd apps/web && pnpm run typecheck && pnpm test -- plugins`
- `cd apps/web && pnpm lint`

## Output contract
Report: client method list, slice shape, store wiring points. Follow apps/web/AGENTS.md
lint limits. Do not build the page (task-12).

## Dependencies
task-09.
