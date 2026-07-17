---
id: task-12
title: Plugins settings page (list/register/enable/disable/uninstall)
status: pending
wave: 3
depends_on: [task-10, task-11]
plan: docs/plans/plugins/plan.md
---

# Plugins settings page

## Title
A Plugins settings page listing registered plugins with status badges and
enable/disable/uninstall actions plus a register-via-manifest-paste dialog, gated
on the `plugins` feature flag.

## Inputs
- Slice + API from task-11; feature flag from task-10 (`useFeature("plugins")`
  via `apps/web/hooks/domains/features/use-feature.ts`).
- Page + route convention: `apps/web/app/settings/integrations/jira/page.tsx` and
  registration in `apps/web/src/settings-routes.tsx` (add
  `import PluginsSettingsPage`, a `SETTINGS_ROUTES` entry, and the switch/nav
  wiring parallel to `IntegrationsIndexPage`). Add a nav entry in the settings nav
  (grep where `integrations` nav item is defined).
- Shadcn components via `@kandev/ui`. Status badge colors: active=green,
  error=red, disabled=gray, registered=amber, uninstalled=hidden.
- `/mobile-parity`: page must be responsive; include mobile layout.

## Acceptance
1. `apps/web/app/settings/plugins/page.tsx`: lists plugins (name, id, status badge,
   categories); actions enable/disable/uninstall (confirm dialog on uninstall);
   "Register plugin" opens a dialog with a manifest YAML textarea → `registerPlugin`,
   showing the returned api_key + webhook_secret once with a copy button and a
   "shown only once" warning.
2. Route registered in `src/settings-routes.tsx`; nav item added; whole page gated
   on `useFeature("plugins")` (renders nothing / notFound when off).
3. Each plugin row links to the detail route `/settings/plugins/:id` (task-13).
4. Loading/empty/error states.

## Files
- `apps/web/app/settings/plugins/page.tsx` (+ small child components as needed)
- edits: `apps/web/src/settings-routes.tsx`, settings nav file
- `apps/web/app/settings/plugins/page.test.tsx` (render + action wiring with mocked slice)

## Verification
- `cd apps/web && pnpm run typecheck && pnpm lint && pnpm test -- plugins`

## Output contract
Report: route key added, nav wiring, feature-gate approach, mobile handling.
Component markup itself need not be unit-tested beyond action wiring.

## Dependencies
task-10, task-11.
