---
id: "01-isolate-sidebar-activation"
title: "Isolate sidebar activation from dialog gestures"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-CUSTOMIZATION-006
acceptance_criteria:
  - AC-UI-SIDEBAR-CUSTOMIZATION-006.1
  - AC-UI-SIDEBAR-CUSTOMIZATION-006.7
  - AC-UI-SIDEBAR-CUSTOMIZATION-006.9
system_design:
  - ../../specs/ui/system-design/sidebar-customization.md
---

# Task 01: Isolate Sidebar Activation

## Summary

Prevent prompt gestures from activating the ancestor sidebar context menu.
Retain normal editing and both desktop and phone customization entry points.

## In scope

A local ref-forwarding trigger in `sidebar-customize-menu.tsx`, component tests
using a real portal and Radix menu, and actual-dialog browser regressions.
Implement sequentially with red, green, and focused verification.

## Out of scope

Shared primitive changes, dialog lifecycle moves, settings writes, task creation
logic, visual redesign, commits, pushes, and PR publication.

## Acceptance

1. Portal context and held-touch events do not open Sidebar settings or cancel
   browser defaults. Short taps, selection, typing, and draft retention still work.
2. Real sidebar context gestures and the explicit phone customization action
   still open their intended surfaces. Pointer release and cancellation clear timers.
3. The named component and browser suites pass. Record the initial regression
   failure and final results. State any real-device verification limit.

## UI behavior preview

See the [combined preview](plan.md#ui-behavior-preview).
Phone: Menu, New Task, editable prompt. Prompt gestures keep the form active.
Desktop: New Task, centered dialog, editable prompt. Navigation right-click
continues to open Sidebar settings. No layout or copy changes are required.
This preview maps to AC-UI-SIDEBAR-CUSTOMIZATION-006.9.

## Verification

Run from the repository root. Install dependencies only when absent.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/app-sidebar/sidebar-customize-menu.test.tsx)
(cd apps/web && pnpm exec eslint components/app-sidebar/sidebar-customize-menu.tsx components/app-sidebar/sidebar-customize-menu.test.tsx e2e/tests/settings/sidebar-direct-customization.spec.ts e2e/tests/settings/mobile-sidebar-direct-customization.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-sidebar-direct-customization.spec.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/sidebar-direct-customization.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Managed E2E builds current assets. Run the two projects sequentially. Use existing
fixtures and preserve settings restoration. Include ordinary tap, held touch,
and context-event paths. Native keyboard behavior on physical phones remains a
manual check unless the environment provides a real device.

## Files likely touched

- `apps/web/components/app-sidebar/sidebar-customize-menu.tsx`
- `apps/web/components/app-sidebar/sidebar-customize-menu.test.tsx` (new)
- `apps/web/e2e/tests/settings/mobile-sidebar-direct-customization.spec.ts`
- `apps/web/e2e/tests/settings/sidebar-direct-customization.spec.ts`
- This plan and work order for implementation status and results.

## Dependencies

None.

## Risks

The filtering element must receive the injected Radix handlers, forward its ref,
and preserve other props. Do not prevent default on portal events. Do not
mistake a synthetic context event for native mobile text-selection coverage.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/sidebar-customization.md), section 006.
- [Design](../../specs/ui/system-design/sidebar-customization.md#dialog-event-isolation).
- `AppSidebarNewTaskItem`, `SidebarLayoutNavigation`, and `TaskCreateDialog`.
- Existing desktop and mobile direct-customization E2E fixtures and cleanup.
- Diagnostic evidence in [the plan](plan.md#evidence-and-root-cause).

## Results

Completed on 2026-10-21. Production changes are limited to
`sidebar-customize-menu.tsx`. The local ref-forwarding trigger admits activation
only from its DOM subtree. All other injected props and cleanup handlers remain
forwarded. No default action is cancelled for rejected portal events.

Verification commands and results:

- Workspace installation completed during planning and was reused.
- The named Vitest command failed on three portal cases before the correction,
  then passed all 10 cases after the final test edit.
- Both named E2E specs reproduced the defect with original built assets. The
  mobile case failed after a native CDP touch hold. Desktop failed after a real
  right-click in the prompt.
- `pnpm e2e:run --project mobile-chrome tests/settings/mobile-sidebar-direct-customization.spec.ts`:
  2 passed in 27.7 seconds, with a fresh managed build.
- `pnpm e2e:run --project chromium tests/settings/sidebar-direct-customization.spec.ts -- --config e2e/playwright.prompt-check.config.ts`:
  7 passed in 34.4 seconds, with a fresh managed build. The temporary config
  narrowed the mobile filename exclusion because this worktree's directory
  contains `mobile-`. All other settings remained intact.
- Both successful browser commands used `KANDEV_SERVER_HOST=127.0.0.1` and
  `LD_LIBRARY_PATH` resolved from installed Nix libraries. Initial attempts
  exposed a host bind mismatch and missing browser library search paths.
- The exact scoped ESLint command above passed with no warnings. Prettier
  checks on all four source/test files passed.
- `NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck` passed after the
  default 4 GiB heap exhausted memory.
- Both specification commands and `git diff --check` passed.
- `.github/scripts/pr-docs.cjs` `validateCoverage` passed for all changed
  source/test files and the complete linked documentation package.

See [plan results](plan.md#verification-results) for retained command logs.
The temporary browser config and library-path file were removed. Managed E2E
cleaned up its isolated instances. No physical phone or iOS check was performed.
