---
id: "02-resolve-threads-home-navigation"
title: "Resolve Threads Home navigation"
status: done
wave: 2
depends_on:
  - "01-persist-threads-startup-choice"
plan: "plan.md"
requirements:
  - REQ-UI-TASK-LISTING-DISPLAY-PREFERENCES-001
  - REQ-UI-TASK-LISTING-DISPLAY-PREFERENCES-002
  - REQ-UI-TASK-LISTING-DISPLAY-PREFERENCES-003
acceptance_criteria:
  - AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-001.3
  - AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-001.4
  - AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-001.5
  - AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-001.6
  - AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-001.12
  - AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-002.2
  - AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-002.3
  - AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-002.4
  - AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-003.4
  - AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-003.5
  - AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-003.6
  - AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-003.7
system_design:
  - ../../specs/ui/system-design/task-listing-display-preferences.md
---

# Task 02: Resolve Threads Home Navigation

## Summary

Make generic Home and bare startup honor the portable Threads choice in the
resolved workspace. Keep explicit navigation and remembered listing behavior
independent, including when settings bootstrap is delayed.

## In scope

- Use `/tdd`; add the design's entry-resolution and Home href matrices first.
- Share the Home resolver between `workspaceHomeHref`, `homeDestinationHref`,
  and their actual sidebar/topbar/mobile/settings/picker/command consumers.
  Carry `startupPage` in `NavContext` and keep old callers' default behavior.
- Override the palette's legacy overview href for the Threads choice only.
- Resolve one startup redirect after authoritative settings/workspace
  bootstrap. Expose completion from `useKanbanRouteBootstrap`, including
  failed-fetch and empty-workspace completion. Keep cancellation tied to route
  identity. Preserve the local view hook's real `loaded` value rather than
  setting it to true during a rendering-mode update.
- Preserve `isExplicitHomeDestination`, local view storage/fallbacks,
  explicit `linkToTaskOverview`, and path-preserving `listingHistoryHref`.

## Out of scope

- Settings radio/copy, saved Threads filter/session defaults, task Back
  destination changes, mobile topbar geometry, swipe timing, and new stores.

## Acceptance

- Every generic Home consumer honors Threads in the selected non-Office
  workspace; Office remains Office, and existing choices preserve current
  behavior, including the palette's legacy target.
- Bare Home chooses Threads despite a competing local view or unavailable
  storage, with no earlier wrong-route redirect during settings/workspace
  hydration. Empty/error/no-workspace paths settle without loops.
- Explicit task/session/workflow/overview/listing links and history remain
  usable; view toggles never change the portable default, last-task resume
  stays workspace-local/startup-only, and phone Pipeline fallback is not saved.

## Verification

Task 01 has already installed dependencies. Run each line from the repo root:

```bash
(cd apps && pnpm --filter @kandev/web test lib/startup-page.test.ts lib/task-listing/view-preference.test.ts lib/task-listing/view-navigation.test.ts hooks/use-task-listing-view.test.tsx app/page-client.test.tsx)
(cd apps && pnpm --filter @kandev/web test lib/navigation/workspace-home.test.ts lib/navigation/resolve-destinations.test.ts hooks/use-app-destinations.test.tsx hooks/use-home-affordance.test.ts components/app-sidebar/app-sidebar-workspace-navigation.test.ts src/kanban-route.test.ts src/kanban-route-startup.test.tsx)
(cd apps/web && pnpm run typecheck)
git diff --check
```

`workspace-home.test.ts` and `kanban-route-startup.test.tsx` are new files.
The latter covers deferred settings, stale workspace fetches, Office priority,
and empty/error readiness using real route wiring with mocked transport.
Browser entry-point proof is owned by Task 03; pure href tests alone do not
complete the feature.

## Files likely touched

- `apps/web/lib/startup-page.ts` and `startup-page.test.ts`
- `apps/web/app/page-client.tsx` and `page-client.test.tsx`
- `apps/web/src/kanban-route.tsx`, existing `kanban-route.test.ts`, and new
  `kanban-route-startup.test.tsx`
- `apps/web/lib/navigation/workspace-home.ts` and new `workspace-home.test.ts`
- `apps/web/lib/navigation/core-destinations.ts`, `types.ts`, and
  `resolve-destinations.test.ts`
- `apps/web/hooks/use-app-destinations.ts`, `use-app-destinations.test.tsx`,
  and `use-home-affordance.test.ts`
- `apps/web/hooks/use-task-listing-view.ts` and `use-task-listing-view.test.tsx`
- `apps/web/lib/task-listing/view-preference.test.ts` and `view-navigation.test.ts`
- `apps/web/components/app-sidebar/app-sidebar-header.tsx`,
  `app-sidebar-primary-nav.tsx`, `app-sidebar-footer.tsx`,
  `app-sidebar-workspace-picker.tsx`, and `app-sidebar-workspace-navigation.test.ts`
- `apps/web/components/kanban/kanban-header-mobile.tsx` (Home destination only)

`view-preference.ts`, `view-navigation.ts`, and `use-home-affordance.ts` already
own the correct lower-level contracts; change their production logic only if
needed for the shared resolver, not to move startup policy into view memory.

## Dependencies

[Task 01](task-01-persist-threads-startup-choice.md).

## Risks

- Root startup cannot distinguish generic Home from explicit overview by
  pathname alone; use entry intent rather than an unconditional redirect.
- Gating on a workflow/snapshot deadlocks empty workspaces. Gating only on the
  local view hook's old `loaded: true` write races settings hydration.
- A workspace picker must pass its selected workspace, not a stale active one.
- The phone brand is Home on listing surfaces; task Task overview is a
  separate explicit link and must not be swept into this change.

## Parallelism

`sequential`

## Inputs

- Design sections **Entry resolution**, **Home consumers**, **Remembered
  listing and list details**, and **Failure and recovery**.
- Existing view preference/navigation tests and page-client startup tests.
- Existing `useHomeAffordance` and sidebar navigation tests.
- `apps/web/AGENTS.md`, `/mobile-parity`, ADR 0023, and Office-mode ADR.

## Results

Shared Home policy now covers the manifest, palette, sidebar, settings exit,
workspace picker, and phone brand. Bare startup waits for authoritative route
bootstrap; explicit destinations and Office retain priority. The device view
hook preserves settings readiness.

RED: 11 destination/page/hook assertions and four bootstrap assertions failed
for the expected missing behavior. GREEN: the listed suites plus existing SPA
workspace and sidebar/header neighbors passed (16 files, 148 tests). Typecheck,
focused ESLint, and `git diff --check` passed. The SPA workspace test now waits
for bootstrap before asserting the selected workspace. Browser proof follows
in Task 03.

### PR review remediation

UI remains the owner of Home presentation; Office availability retains its
existing feature-gate ownership. AC-003.6 and the Home-consumer design were
clarified before repairing the phone brand's disabled-Office destination. Reuse the
existing brand as direct navigation into the native task listing. Layout,
touch targets, safe areas, scroll ownership, and the native menu/deck exemplars
remain unchanged; no parent topbar or swipe work is included.

RED: the two disabled-Office header cases and phone href assertion resolved
to `/office` instead of the expected task listing. The stale workspace-picker
mock also reproduced 23 crashes before repair. GREEN: gate the phone's Office
record with the existing feature flag, preserving its workspace ID fallback;
restore complete fixture inputs and cover Threads selection and Office priority.
Direct no-workspace and explicit-overview cases document existing resolver
behavior, with a short production invariant comment.

Verification from `apps/web`:

```bash
pnpm exec vitest run components/kanban/kanban-header-mobile.test.tsx components/app-sidebar/app-sidebar-workspace-picker.test.tsx lib/startup-page.test.ts e2e/helpers/api-client.test.ts lib/navigation/workspace-home.test.ts hooks/use-home-affordance.test.ts src/kanban-route-startup.test.tsx app/page-client.test.tsx
pnpm run typecheck
pnpm exec eslint --max-warnings 0 components/kanban/kanban-header-mobile.tsx components/kanban/kanban-header-mobile.test.tsx components/app-sidebar/app-sidebar-workspace-picker.test.tsx lib/startup-page.ts lib/startup-page.test.ts e2e/helpers/api-client.ts e2e/helpers/api-client.test.ts e2e/tests/office/mobile-office-navigation.spec.ts
```

All 88 focused tests passed. After completing the typed header fixture and
splitting its describe blocks to meet the lint limit, all 13 header tests
passed again. Typecheck and focused ESLint passed. Task 03 records the rebuilt
browser proof. Specification lint and its 30 validator tests passed.

### CI navigation-fixture remediation

The full frontend CI suite exposed three additional stale store fixtures in
the primary sidebar, phone navigation sheet, and integrations menu tests.
All 32 failures reproduced locally because the shared Home context now reads
`userSettings.startupPage`. Restore the complete settings slice from the
existing default state and reset mutable settings between cases. Extend the
existing desktop and phone Home assertions to cover the Threads choice.

This is test-only compatibility repair. Requirements, system design, public
copy, production routing, and native surface geometry remain unchanged.

Verification from the repo root:

```bash
(cd apps/web && pnpm exec vitest run components/app-sidebar/app-sidebar-primary-nav.test.tsx components/navigation/app-nav-sheet.test.tsx components/integrations/integrations-menu.test.ts)
(cd apps/web && pnpm exec eslint --max-warnings 0 components/app-sidebar/app-sidebar-primary-nav.test.tsx components/navigation/app-nav-sheet.test.tsx components/integrations/integrations-menu.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps && NODE_ENV=production pnpm --filter @kandev/web test)
```

The pre-merge focused run passed all 34 tests, focused ESLint, and typecheck.
Main integration then required the UI index to retain both this design and the
new control-sizing design. The merged routing/settings suite passed 122 tests
in 11 files, followed by full web lint, typecheck, i18n checks, tagged/race Go
startup and boot-mapping tests, and the rebuilt browser checks in Task 03.
Harness validation passed 19 tests and all 196 files; specification validation
passed 36 tests and all specs. The PR-only diff is whitespace-clean; staged
whitespace warnings were verified as unchanged incoming main content.

The first merged full-suite attempt could not open a tsx IPC socket inside the
agent sandbox (`EPERM`) and was stopped without a verdict. Full-suite execution
requires local IPC permission in this environment. Final broad-suite and
current-head CI/review results belong to the PR delivery evidence; the scoped
remediation checks above are complete.
