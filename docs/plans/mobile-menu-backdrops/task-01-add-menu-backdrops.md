---
id: "01-add-menu-backdrops"
title: "Add shared mobile menu backdrops"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-MOBILE-TASK-NAVIGATION-001
acceptance_criteria:
  - AC-UI-MOBILE-TASK-NAVIGATION-001.3
  - AC-UI-MOBILE-TASK-NAVIGATION-001.9
  - AC-UI-MOBILE-TASK-NAVIGATION-001.10
  - AC-UI-MOBILE-TASK-NAVIGATION-001.11
  - AC-UI-MOBILE-TASK-NAVIGATION-001.12
system_design:
  - ../../specs/ui/system-design/mobile-menu-backdrops.md
---

# Task 01: Add Shared Mobile Menu Backdrops

## Summary

Extend shared dropdown/context bottom-sheet styling with a backdrop matching
the existing Drawer treatment. Prove the rendered result, nested interactions,
cleanup, and the existing desktop boundary through focused browser tests.

## In scope

- Add the `mobile-menu-root` marker to both shared root content components.
- Add a phone-only decorative backdrop to the immediate positioning wrapper
  while the marked root content is open, following the system design.
- Implement the five named scenarios in the plan, with shared assertions in
  `e2e/helpers/menu-backdrop.ts` only where used by both files.
- Update scoped frontend guidance for the shared backdrop rule.

## Out of scope

- Consumer-specific backdrop patches, new menu state, extra portals, or new
  event handlers and listeners.
- Changes to Drawer, Sheet, Dialog, Select, Popover, menu breakpoints, or action
  handlers; public documentation, translations, or dependencies.

## Acceptance

1. The current Kanban trigger produces the expected failing browser regression
   before the fix. After the fix, both root primitive families show a viewport
   backdrop with supported blur, sharp foreground content, internal scrolling,
   safe-area spacing, and reachable 44px rows.
2. Nested context choices create no additional menu backdrop. Selecting a
   priority or move action succeeds; dismissing a child menu leaves the parent
   Drawer usable. Outside tap, Escape, menu unmount, and non-modal workspace
   selection remove only the appropriate layer without leaking a click into
   task navigation or drag behavior.
3. At 639px the open menu has a backdrop; at 640px and normal desktop widths it
   has none and remains anchored. Resizing while open follows this boundary.
   Focus returns as before and closing restores normal page interaction.

## Verification

Bootstrap this fresh worktree from the repository root:

```bash
cd apps && pnpm install --frozen-lockfile
```

Write browser regressions first and run the mobile file before changing
production code. Record the missing-backdrop failure, not a fixture or build
failure. Use real `.tap()` input for mobile actions. For geometry, await only
finite animations and tolerate cancellation rather than sleeping.

From `apps/web`, run these commands sequentially. The managed runner builds
the current application and owns its isolated backend and cleanup:

```bash
pnpm e2e:run --project mobile-chrome tests/layout/mobile-menu-backdrops.spec.ts -- --retries=0
pnpm e2e:run --project chromium tests/layout/menu-backdrops.spec.ts -- --retries=0
pnpm e2e:run --project mobile-chrome tests/kanban/mobile-kanban.spec.ts -- --grep 'renders kanban card dropdown as a mobile bottom sheet|switches workspaces from the mobile menu' --retries=0
pnpm e2e:run --project mobile-chrome tests/task/mobile-sidebar-task-actions.spec.ts -- --grep 'opens a viewport-bound action sheet without covering diff stats|moves a task to another step from the mobile task drawer' --retries=0
pnpm run typecheck
pnpm exec eslint --max-warnings 0 e2e/helpers/menu-backdrop.ts e2e/tests/layout/mobile-menu-backdrops.spec.ts e2e/tests/layout/menu-backdrops.spec.ts
```

Do not run these suites concurrently. Confirm test discovery in each named
project. Do not create a permanent per-test phone-device override; use the
configured Pixel 5 project. The desktop boundary scenario can resize its
viewport to 639px and 640px because the existing menu rule is width-based.

Check computed pseudo-element content, background opacity, supported blur,
fixed viewport coverage, pointer transparency, and relative stacking. Compare
the effective dimming and blur against an open Drawer. Verify a real menu row
is returned by `elementFromPoint` at its center before completing its action.
Assert no document horizontal overflow and one generated backdrop per menu
hierarchy. Include a close/unmount case and light/dark theme coverage.

Capture a rendered phone screenshot for the Kanban menu and a nested context
menu. Inspect that background content is blurred while text remains sharp.
Use the isolated application or test fixture, never the developer's live
instance. Record the configured browser and any limits of visual evidence.

From the repository root, verify the edited documents and whitespace:

```bash
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/packages/ui/src/dropdown-menu.tsx`
- `apps/packages/ui/src/context-menu.tsx`
- `apps/web/app/globals.css`
- `apps/web/AGENTS.md`
- `apps/web/e2e/helpers/menu-backdrop.ts` (new)
- `apps/web/e2e/tests/layout/mobile-menu-backdrops.spec.ts` (new)
- `apps/web/e2e/tests/layout/menu-backdrops.spec.ts` (new)

## Dependencies

None. Install workspace dependencies before package commands. Follow `/tdd`,
`/e2e`, and `/mobile-parity` during implementation.

## Risks

- Pseudo-element stacking depends on the actual root positioning wrapper;
  source-text assertions cannot establish visual correctness.
- Parent Drawer overlays and non-modal workspace menus must keep their current
  lifetime and event handling.
- Browser builds lacking filter support must retain dimming. Chromium alone
  does not prove WebKit rendering.

## Parallelism

`sequential`

## Inputs

- [Mobile Task Navigation requirements](../../specs/ui/requirements/mobile-task-navigation.md)
- [Mobile Menu Backdrops design](../../specs/ui/system-design/mobile-menu-backdrops.md)
- Existing backdrop: `apps/packages/ui/src/drawer.tsx`.
- Existing bottom-sheet CSS: `apps/web/app/globals.css`.
- Existing browser patterns: `e2e/tests/kanban/mobile-kanban.spec.ts`,
  `e2e/tests/task/mobile-sidebar-task-actions.spec.ts`, and
  `e2e/tests/task/mobile-external-link-menu.spec.ts`, relative to `apps/web/`.

## Results

Completed on 2026-09-10 after the explicit implementation request.

### Implementation and regression evidence

- Added `mobile-menu-root` only to root dropdown/context content. Added one
  pointer-transparent, viewport-sized backdrop on its open positioning wrapper,
  below 640px, using the existing Drawer dimming and supported blur utilities.
- Reset Radix's inline `will-change: transform` hint in the existing phone
  positioner rule. Browser inspection confirmed the hint otherwise constrains
  the fixed backdrop to menu bounds despite `transform: none`.
- The first Kanban regression failed before production changes: expected
  generated pseudo-element content, received `none`. The final mobile run
  passed all three scenarios, including light/dark comparison against Drawer,
  outside tap, Escape, unmount cleanup, nested priority selection, and workspace
  selection inside a non-modal parent Drawer.
- Browser assertions accept the existing animation's `blur(0px)` as sharp
  content. Submenu tests preserve Radix behavior: ArrowLeft closes the child;
  Escape closes the menu hierarchy and leaves the enclosing Drawer open.
  Workspace reopening waits for the previous root's exit presence to detach.
- One intermediate Kanban run reported absent generated content. The immediate
  diagnostic run, five sequential traced repetitions, and final full mobile
  run passed without another production change. The intermittent result was
  not reproduced in those checks.

### Completed checks

| Check | Result |
| --- | --- |
| New mobile backdrop file, configured Pixel 5 / `mobile-chrome` | 3 passed |
| New boundary/desktop file, configured Desktop Chrome / `chromium` | 2 passed |
| Selected existing mobile Kanban compatibility scenarios | 2 passed |
| Selected existing mobile sidebar compatibility scenarios | 2 passed |
| Additional traced Kanban repetitions, one worker, no retries | 5 passed |
| Web typecheck | Passed |
| Targeted ESLint and Prettier | Passed |

Installed dependencies with `pnpm install --frozen-lockfile` from `apps/`.
Built the backend, mock helpers, and E2E plugin package through the repository
targets, then built current frontend assets with
`pnpm --filter @kandev/web build:e2e`. The focused managed runs used `--host
--no-build` to reuse those verified current artifacts, one worker, and
`--retries=0`. The final mobile command added `CAPTURE_PR_ASSETS=1` and
`--trace=on`; the diagnostic command filtered `Kanban task options` and added
`--repeat-each=5 --trace=on`.

The normal desktop invocation found no tests because its existing
`/mobile-.*\.spec\.ts/` ignore expression also matches the absolute worktree
parent `some-mobile-menu-bot_6he5618a`. A temporary config imported the normal
config, retained the `chromium` project's browser settings, selected only
`**/menu-backdrops.spec.ts`, and cleared that project's ignore list. The same
managed command with `--config=e2e/menu-backdrops-verification.config.ts`
passed both cases. The temporary config was removed; the shared Playwright
configuration was not changed.

### Visual evidence and cleanup

Before publication, refreshed the captures in one disposable mobile capture
scenario, using the same isolated Pixel 5 fixture. The capture passed and its
temporary spec was removed. Inspected these assets in `apps/web/.pr-assets/`:

- `mobile-menu-pr-capture--kanban-light.png`
- `mobile-menu-pr-capture--kanban-dark.png`
- `mobile-menu-pr-capture--nested-task-menu.png`

The fresh captures wait for transient update toasts to dismiss normally.
No desktop screenshot is required because the new backdrop is structurally
absent at 640px and above; the desktop and boundary tests cover that behavior.
This evidence is Chromium-only, not a WebKit or physical-device validation.

Closed the named verification browser and stopped the owned isolated runtime.
The managed browser fixtures handled their own backend and test-data cleanup.
No live user instance or data was used. Commit and publication follow the
user's separate PR request.
