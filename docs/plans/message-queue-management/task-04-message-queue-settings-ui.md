---
id: "04-message-queue-settings-ui"
title: "Add the Message Queue General settings page"
status: completed
wave: 3
depends_on: ["02-live-queue-capacity-settings"]
plan: "plan.md"
spec: "../../specs/ui/message-queue-management.md"
---

# Task 04: Add the Message Queue General Settings Page

## Acceptance

- `/settings/general/message-queue` is reachable through desktop and mobile
  General navigation and has a translated breadcrumb/title. The former System
  URL redirects to the canonical General route.
- The page shows configured/effective values, source, unlimited explanation,
  loading/error state, environment lock, and member read-only state.
- Admin edits accept integers `>= 0`, register with the shared settings save
  coordinator, persist through PATCH, and reconcile the returned effective
  value. No local Save/Cancel button exists.
- The page is single-column, uses the shared page scroll owner, has no
  horizontal overflow, and keeps the numeric input at least 44px high.
- API types, helpers, route registry, sidebar tests, and i18n catalogs cover the
  new surface.

## TDD Sequence

1. Add API helper tests and component tests for default, unlimited, dirty save,
   validation, environment lock, member read-only, failure, and discard. Run
   RED.
2. Add route/sidebar/breadcrumb tests for the new leaf and mobile Settings tree.
   Run RED.
3. Implement typed API helpers, settings component/page, route registration,
   translated nav label, and save-contributor wiring.
4. Apply mobile layout and input sizing from the shared System page language.
5. Run focused tests, typecheck, lint, and i18n gates GREEN.

## Verification

```bash
cd apps
pnpm --filter @kandev/web exec vitest run \
  components/settings/system/message-queue-settings.test.tsx \
  components/app-sidebar/sections/settings/settings-tree-render.test.tsx \
  components/settings/settings-layout-client.test.tsx \
  lib/api/domains/settings-api.test.ts
pnpm --filter @kandev/web run typecheck
pnpm --filter @kandev/web run lint
pnpm --filter @kandev/web run i18n:check
pnpm --filter @kandev/web run i18n:ratchet
```

## Files Likely Touched

- `apps/web/components/settings/system/message-queue-settings.tsx`
- `apps/web/components/settings/system/message-queue-settings.test.tsx`
- `apps/web/app/settings/general/message-queue/page.tsx`
- `apps/web/app/settings/system/message-queue/page.tsx` (legacy redirect)
- `apps/web/src/settings-routes.tsx`
- `apps/web/components/app-sidebar/sections/settings/system-group.tsx`
- `apps/web/components/app-sidebar/sections/settings/settings-tree-render.test.tsx`
- `apps/web/components/settings/settings-layout-client.tsx`
- `apps/web/components/settings/settings-layout-client.test.tsx`
- `apps/web/lib/api/domains/settings-api.ts`
- `apps/web/lib/api/domains/settings-api.test.ts`
- `apps/web/lib/types/system.ts`
- `apps/web/src/locales/en/system.json`
- generated pseudo-locale catalog

## Dependencies

Task 02 supplies the typed HTTP contract and live behavior.

## Parallelism

Parallel-safe with Task 03 after backend dependencies. No subagent is
authorized by this task file.

## Output Contract

Report RED/GREEN unit evidence, save-coordinator behavior, desktop/mobile route
coverage, typecheck/lint/i18n status, changed files, and residual UX risks.
Update this task and `plan.md` status in the same implementation conversation.

## Results

- RED: the five focused files reported one unresolved component suite and five
  failures for missing API helpers, route, translated breadcrumb, and System
  navigation leaf.
- GREEN: 54 focused tests pass across the settings component, API, route,
  sidebar tree, and layout.
- The shared save contributor stages valid admin changes, PATCHes only on the
  global Save changes action, reconciles the response, and restores the
  authoritative configured value on discard. Invalid drafts block shared save.
- Environment overrides show their effective value and lock explanation;
  members receive the same read surface with a disabled field. Load and save
  failures stay visible and retryable/actionable.
- The same translated General leaf feeds desktop and mobile navigation. The
  page uses the shared safe-area-aware scroll container, adds no nested page
  scroller, and gives the numeric input a 44px height.
- Follow-up RED/GREEN: navigation and route tests proved the leaf was still
  under System; it now lives under General and the old URL redirects.
- GREEN: web typecheck, lint, `i18n:check`, and `i18n:ratchet`.
