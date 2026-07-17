---
id: task-13
title: Plugin detail view with sandboxed iframe UI pages
status: pending
wave: 3
depends_on: [task-12]
plan: docs/plans/plugins/plan.md
---

# Plugin detail view with sandboxed iframe UI pages

## Title
A plugin detail route rendering the plugin's declared `ui.pages` as sandboxed
iframes served via the UI proxy, with a minimal postMessage theme/context bridge.

## Inputs
- Detail route `/settings/plugins/:id` (linked from task-12); register in
  `src/settings-routes.tsx` like the `[id]`-style routes there.
- UI proxy endpoint (task-08/09): iframe `src={/api/plugins/${id}/ui${page.path}}`.
- `ui.pages[]` come from the plugin record (task-11 types): `{key,title,path,surface}`.
- Theme source: existing theme store/hook (grep `useTheme`/`theme` in apps/web).

## Acceptance
1. `apps/web/app/settings/plugins/[id]/page.tsx`: header (name, status, config
   summary), a tab/section per `ui.pages` entry with `surface: settings`, each
   rendering `<iframe sandbox="allow-scripts allow-forms allow-same-origin"
   src="/api/plugins/{id}/ui{page.path}">`.
2. A small bridge: on iframe load and on theme change, `postMessage`
   `{type:"kandev:context", theme, pluginId}` to the iframe; listen for
   `{type:"kandev:resize", height}` to size the iframe. Ignore messages whose
   `event.source` isn't the iframe's contentWindow.
3. Gated on `useFeature("plugins")`.
4. Detail shows enable/disable/uninstall + config edit (JSON/textarea against
   `config_schema` — a simple textarea is acceptable for v1).

## Files
- `apps/web/app/settings/plugins/[id]/page.tsx` (+ `plugin-ui-frame.tsx` child)
- edits: `apps/web/src/settings-routes.tsx`
- `apps/web/app/settings/plugins/[id]/plugin-ui-frame.test.tsx` (postMessage bridge:
  origin/source guarding + resize handling with a mocked iframe ref)

## Verification
- `cd apps/web && pnpm run typecheck && pnpm lint && pnpm test -- plugin`

## Output contract
Report: route registration, iframe sandbox attributes, postMessage bridge contract
(message types + source guarding), mobile behavior. Follow /mobile-parity.

## Dependencies
task-12.
