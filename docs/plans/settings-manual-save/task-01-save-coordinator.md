---
id: "01-save-coordinator"
title: "Save coordinator and navigation guard"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/settings-manual-save.md"
---

# Task 01: Save Coordinator and Navigation Guard

## Acceptance

- Settings descendants can register stable dirty contributors and one fixed action saves all valid dirty contributors in stable order with revision-safe partial-failure handling.
- The action is hidden when clean, accessible on desktop/mobile, uses safe-area offsets, and does not cover the final page controls.
- Dirty in-app/history navigation offers Save and leave, Discard and leave, and Continue editing; reload/external exit installs a native warning.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run components/settings/settings-save-provider.test.tsx components/settings/settings-layout-client.test.tsx components/routing/app-link.test.tsx lib/routing/client-router.test.ts
cd apps/web && pnpm run typecheck
```

## Files Likely Touched

- `apps/web/components/settings/settings-save-provider.tsx`
- `apps/web/components/settings/settings-save-provider.test.tsx`
- `apps/web/components/settings/settings-floating-save.tsx`
- `apps/web/components/settings/settings-layout-client.tsx`
- `apps/web/components/settings/settings-layout-client.test.tsx`
- `apps/web/lib/routing/navigation-guard.ts`
- `apps/web/lib/routing/client-router.ts`
- `apps/web/lib/routing/client-router.test.ts`
- `apps/web/components/routing/app-link.tsx`
- `apps/web/components/routing/app-link.test.tsx`

## Dependencies

None.

## Inputs

- Spec: What, State Machine, Failure Modes, navigation and mobile Scenarios.
- ADR 0042.

## Output Contract

Report registry API, save ordering/error semantics, navigation behavior, rendered mobile checks, tests run, files touched, blockers, and update this task plus `plan.md`.

## Result

- Added a keyed contributor registry with stable ordering, validation state, revision snapshots, sequential partial-failure saves, retries, and discard callbacks.
- Added the safe-area-aware fixed action and dirty-navigation dialog to the settings shell.
- Guarded Link/router/history navigation and installed native unload protection while the route is dirty.
- Targeted component/routing tests, TypeScript typecheck, and targeted ESLint pass.
