---
id: "01-synchronize-toast-theme"
title: "Synchronize toast theme at mount"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-TOAST-THEME-001
acceptance_criteria:
  - AC-UI-TOAST-THEME-001.1
  - AC-UI-TOAST-THEME-001.2
  - AC-UI-TOAST-THEME-001.3
system_design:
  - ../../specs/ui/system-design/toast-theme.md
---

# Task 01: Synchronize Toast Theme at Mount

## Summary

Correct the missed initial theme change in the shared Sonner wrapper. Add a
real mount regression and prove a plugin success notification uses the
resolved palette on desktop and mobile.

## In scope

- Component tests for initially unthemed documents, preference precedence,
  live preview/discard/system changes, cleanup, and explicit caller overrides.
- Initial document-theme synchronization in the shared observer effect.
- Focused desktop/mobile browser tests and mobile screenshot inspection.

## Out of scope

- Plugin backend, notification delivery, layout, localization, or new palettes.
- Replacing the document-theme boundary or the existing toast APIs.

## Acceptance

1. The saved-dark and system-dark cold-load regressions fail before the
   correction with root `dark` and `data-sonner-theme="light"`, then pass.
2. The shared wrapper preserves live theme updates, observer cleanup, caller
   overrides, notification identity/content, and provider layout ordering.
3. Production-build Chromium and Pixel 5 checks confirm actual success-toast
   colors from the plugin install UI; mobile screenshot and containment checks
   pass, and existing mobile theme-toggle behavior stays green.

## Verification

Fresh-worktree prerequisite, from the repository root:

```bash
cd apps && pnpm install --frozen-lockfile
```

Run RED component and browser regressions before the production edit; repeat
afterward for GREEN. From `apps/web`:

```bash
pnpm exec vitest run components/theme/sonner-theme.test.tsx components/theme/app-theme.test.tsx
pnpm e2e:run --project chromium tests/settings/toast-theme.spec.ts -- --retries=0
pnpm e2e:run --project mobile-chrome tests/settings/mobile-toast-theme.spec.ts tests/kanban/mobile-menu-theme-toggle.spec.ts -- --retries=0
pnpm run typecheck
pnpm exec eslint components/theme/sonner-theme.test.tsx e2e/helpers/toast-theme.ts e2e/tests/settings/toast-theme.spec.ts e2e/tests/settings/mobile-toast-theme.spec.ts
pnpm exec prettier --check ../packages/ui/src/sonner.tsx components/theme/sonner-theme.test.tsx e2e/helpers/toast-theme.ts e2e/tests/settings/toast-theme.spec.ts e2e/tests/settings/mobile-toast-theme.spec.ts
```

Use the managed runner's build and cleanup. Confirm test discovery in both
projects and inspect the captured mobile notification. Use installed Sonner's
`data-sonner-theme` attribute and real computed palette colors. Do not change
timeouts or preload the desired root class to satisfy the regression.

## Files likely touched

- `apps/packages/ui/src/sonner.tsx`
- `apps/web/components/theme/sonner-theme.test.tsx` (new)
- `apps/web/e2e/helpers/toast-theme.ts` (new)
- `apps/web/e2e/tests/settings/toast-theme.spec.ts` (new)
- `apps/web/e2e/tests/settings/mobile-toast-theme.spec.ts` (new)
- This work order and `plan.md` for status and evidence.

## Dependencies

None. Reuse `plugin-test-helpers.ts`, the packaged E2E plugin fixture, and the
existing mobile theme-toggle scenario.

## Risks

The root-class mutation occurs before the passive subscription. Tests that
start with a dark document or depend on development effect replay can pass
without repairing that race. Sonner's themed list is absent until a toast is
emitted, so a missing list is test setup failure, not the expected RED signal.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/toast-theme.md), all criteria.
- [System design](../../specs/ui/system-design/toast-theme.md), theme
  synchronization and responsive verification.
- `apps/web/components/theme/app-theme.test.tsx` for provider setup.
- `apps/web/e2e/tests/settings/mobile-plugin-updates.spec.ts` for the affected
  mobile plugin flow.
- `.agents/skills/tdd/SKILL.md`, `.agents/skills/e2e/SKILL.md`, and
  `.agents/skills/mobile-parity/SKILL.md` for implementation and verification.

## Results

Implemented after the explicit user request on 2026-09-09. The shared wrapper
now reads the document theme immediately after subscribing. Production scope
is one state synchronization call and its ordering comment.

### Red and green evidence

- Component RED: two dark-start failures, nine controls/provider tests passed.
  GREEN: all 11 passed in 4.21 seconds.
- Desktop RED: two dark-start failures and one passing light case. GREEN:
  all three passed in 10.2 seconds.
- Mobile RED: two dark-start failures and three passing controls. GREEN:
  all five passed in 16.5 seconds, including the existing theme-toggle tests.
- Both browser RED runs failed with expected `dark`, received `light` from
  the real notification's `data-sonner-theme` attribute. All GREEN browser
  tests also asserted rendered background, foreground, border, and viewport
  containment. Desktop and mobile dark screenshots were inspected.
- Final web typecheck, targeted ESLint, targeted Prettier, and diff checks
  passed. No new public copy or localization changes were needed.

### Commands used

The component, typecheck, ESLint, and Prettier commands in Verification were
run as written. Browser runs used these commands from `apps/web`:

```bash
pnpm e2e:run --project chromium tests/settings/toast-theme.spec.ts -- --config e2e/toast-theme-check.config.ts --retries=0
pnpm e2e:run --no-build --project mobile-chrome tests/settings/mobile-toast-theme.spec.ts tests/kanban/mobile-menu-theme-toggle.spec.ts -- --config e2e/toast-theme-check.config.ts --retries=0
```

The desktop run rebuilt the fixed production Vite assets. The mobile run
reused those unchanged artifacts. Backend build cache writes and test-server
ports required sandbox escalation, which was approved.

### Workspace-path discovery workaround

The default Chromium project's `/mobile-.*\.spec\.ts/` exclusion also matches
this checkout's parent directory, `on-mobile-notificati_ydji1lsa`, causing
`No tests found` before collection. A temporary configuration imported the
normal Playwright configuration and changed only the mobile filename filters
to `/[/\\]mobile-[^/\\]*\.spec\.ts$/`. It preserved the remaining project
settings, fixtures, and strict WebSocket checks, and discovered the intended
three desktop and five mobile tests. The temporary file was removed after
verification; the shared repository runner configuration was not changed.

The first browser setup attempt also corrected its expected fixture title to
the manifest's `Kandev E2E Fixture Plugin` before collecting valid RED evidence.

### Cleanup and artifacts

The managed runner tore down its test backends and browsers. The temporary
Playwright override was removed, and screenshots were retained in
`/tmp/kandev-toast-evidence.Eb9wFU/` for this session. No commit or PR was
requested or created.
