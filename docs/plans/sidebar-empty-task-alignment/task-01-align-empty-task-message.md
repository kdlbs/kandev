---
id: "01-align-empty-task-message"
title: "Align empty task message"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/sidebar-empty-task-alignment.md"
---

# Task 01: Align empty task message

## Intent

Align the desktop app sidebar's empty task message with its Tasks section title without changing the shared mobile task-switcher inset or the task list's full-width surface.

## Root cause

The desktop navigation starts with `px-2`; its Tasks title adds `px-2` and renders at x=16. The Tasks content wrapper cancels the navigation inset with `-mx-2`, while the shared empty state uses `px-3`, rendering its text at x=12. The four-pixel difference is the reported drift.

## Acceptance

- On the expanded desktop app sidebar, the **No tasks yet.** text anchor matches the **Tasks** title x-coordinate within subpixel tolerance.
- The empty task scroll surface stays transparent and spans the full navigation width; task rows, group headers, load errors, and other sidebar sections keep their current geometry.
- At the mobile Playwright viewport, the desktop app sidebar remains hidden and the existing mobile task composition remains visible and unchanged.

## Regression tests (write first)

- `apps/web/e2e/tests/task/sidebar-scroll-preservation.spec.ts`: extend the existing `keeps the empty list transparent and uses the full sidebar width` test. Locate the empty state through `data-slot="task-switcher-empty-state"`, derive its text x-coordinate as `getBoundingClientRect().x + parseFloat(getComputedStyle(element).paddingLeft)`, and compare it with the Tasks title span's bounding-box x-coordinate. Add the slot marker first, run the test before the padding override, and record the expected RED mismatch (currently x=12 versus x=16).
- `apps/web/e2e/tests/task/mobile-task-listing-display.spec.ts`: extend `uses Kanban on a phone without overwriting a saved Pipeline preference` to assert `app-sidebar-layout` is hidden while `mobile-kanban-layout` is visible. This guards the viewport boundary and is expected to remain green.

## Implementation

- `apps/web/components/task/task-switcher.tsx`: add `data-slot="task-switcher-empty-state"` to the existing empty-message element. Preserve its base `px-3` class.
- `apps/web/components/app-sidebar/sections/tasks-section.tsx`: add `[&_[data-slot=task-switcher-empty-state]]:px-4` to the wrapper that already owns `-mx-2`. Do not change shared task rows or mobile wrappers.

## Files likely touched

- `apps/web/components/task/task-switcher.tsx`
- `apps/web/components/app-sidebar/sections/tasks-section.tsx`
- `apps/web/e2e/tests/task/sidebar-scroll-preservation.spec.ts`
- `apps/web/e2e/tests/task/mobile-task-listing-display.spec.ts`
- `docs/plans/sidebar-empty-task-alignment/plan.md`
- `docs/plans/sidebar-empty-task-alignment/task-01-align-empty-task-message.md`

## Dependencies

None.

## Parallelism

Sequential. The RED geometry assertion, host-scoped class change, and desktop/mobile checks share one behavior and one production build.

## Inputs

- Repair behavior in `docs/specs/ui/requirements/sidebar-empty-task-alignment.md`.
- Root-cause geometry and host-scoping design in `docs/plans/sidebar-empty-task-alignment/plan.md`.
- Existing empty-list coverage in `apps/web/e2e/tests/task/sidebar-scroll-preservation.spec.ts`.
- Existing phone composition coverage in `apps/web/e2e/tests/task/mobile-task-listing-display.spec.ts`.

## Verification

Bootstrap a fresh worktree once if dependencies are absent:

```bash
(cd apps && pnpm install --frozen-lockfile)
```

Add the slot marker and geometry assertion, then run the desktop regression before the host override (RED):

```bash
(cd apps/web && pnpm e2e:run --project chromium tests/task/sidebar-scroll-preservation.spec.ts -- --grep "keeps the empty list transparent")
```

Apply the minimal host override, rerun the same command (GREEN), then run mobile preservation coverage against the fresh build:

```bash
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/task/mobile-task-listing-display.spec.ts -- --grep "uses Kanban on a phone")
```

Run targeted static checks:

```bash
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/task/task-switcher.tsx components/app-sidebar/sections/tasks-section.tsx e2e/tests/task/sidebar-scroll-preservation.spec.ts e2e/tests/task/mobile-task-listing-display.spec.ts)
(cd apps/web && pnpm exec prettier --check components/task/task-switcher.tsx components/app-sidebar/sections/tasks-section.tsx e2e/tests/task/sidebar-scroll-preservation.spec.ts e2e/tests/task/mobile-task-listing-display.spec.ts)
(cd apps/web && pnpm run i18n:ratchet)
git diff --check
```

The managed desktop E2E run rebuilds the production web assets. Confirm each focused command discovers one test before treating it as evidence.

## Output contract

Report the RED and GREEN x-coordinates, exact commands and test counts, changed files, mobile preservation result, generated artifacts, cleanup evidence, risks, and synchronized task/plan statuses in the same conversation.

## Results

- Added a stable empty-state slot while preserving the shared `px-3` default, then applied `px-4` only from the desktop Tasks host.
- Extended the existing desktop E2E with text-anchor geometry coverage. Its RED run found the expected 4px drift; its GREEN run passed one test after the host override.
- Extended the existing phone scenario to prove the desktop sidebar remains hidden; the focused mobile Chrome run passed one test.
- Confirmed the final desktop geometry in an isolated mock instance: Tasks title x=16, empty text x=16, delta 0.
- Passed web typecheck, targeted ESLint and Prettier, the i18n ratchet, and `git diff --check`. Captured and inspected a synthetic 1280x800 PR screenshot, then stopped the browser and isolated app.
