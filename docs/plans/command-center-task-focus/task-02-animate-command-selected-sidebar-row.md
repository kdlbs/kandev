---
id: "02-animate-command-selected-sidebar-row"
title: "Animate and mark the command-selected sidebar row"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/command-panel-sidebar-task-reveal.md"
---

# Task 02: Animate and Mark the Command-selected Sidebar Row

## Outcome

Command-panel task navigation smoothly reveals an off-screen desktop sidebar
row and gives the selected row a short visual cue. Reduced-motion users get an
immediate scroll and a non-animated cue.

## In scope

- Add motion-preference-aware nearest scrolling to the existing reveal helper.
- Add a restartable, bounded row cue and CSS reduced-motion behavior.
- Extend unit and desktop Playwright coverage while retaining the phone
  navigation regression.

## Exclusions

- Sidebar filtering, expansion, ordering, or persistence changes.
- Command-panel search and task-route loading changes.
- Mobile task-switcher redesign.

## Requirements and design

- `REQ-UI-COMMAND-PANEL-SIDEBAR-TASK-REVEAL-001`
- `AC-UI-COMMAND-PANEL-SIDEBAR-TASK-REVEAL-001.1` through
  `AC-UI-COMMAND-PANEL-SIDEBAR-TASK-REVEAL-001.10`
- `docs/specs/ui/system-design/command-panel-sidebar-task-reveal.md`

## Implementation acceptance

1. Off-screen rows use smooth nearest scrolling in normal motion and immediate
   nearest scrolling under reduced motion. Already visible rows do not move.
2. Every successful reveal restarts one transient cue on the current row. An
   older request or cleanup timer cannot mark or clear the newer target.
3. Desktop E2E proves final nested-viewport containment, active state, cue
   presence, and unchanged document scroll. Phone navigation remains direct.

## TDD and verification

1. RED: extend the DOM unit tests with exact smooth-scroll, reduced-motion, and
   cue lifecycle assertions before changing production code.
2. Unit GREEN: `cd apps/web && pnpm exec vitest run lib/sidebar/task-navigation.test.ts`
3. Typecheck: `cd apps/web && pnpm run typecheck`
4. Desktop E2E GREEN: `cd apps/web && pnpm e2e:run --host --no-build tests/task/sidebar-scroll-preservation.spec.ts --grep "reveals a command-selected task$"`
5. Mobile E2E GREEN: `cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome tests/task/mobile-command-panel-task-navigation.spec.ts`

Make sure that Playwright discovers each focused scenario. Then accept each
browser result as evidence.

## Files likely touched

- `apps/web/lib/sidebar/task-navigation.ts`
- `apps/web/lib/sidebar/task-navigation.test.ts`
- `apps/web/app/globals.css`
- `apps/web/e2e/tests/task/sidebar-scroll-preservation.spec.ts`
- `apps/web/e2e/tests/task/mobile-command-panel-task-navigation.spec.ts`

## Dependencies and parallelism

No code dependency. Execute in the primary session. Do not delegate unless the
user explicitly authorizes implementation agents.

## Results

Completed on 2026-08-26.

### RED evidence

- The extended DOM suite failed 4 expected assertions for smooth and reduced
  motion options, visible-row cueing, and cue restart behavior.

### GREEN evidence

- `pnpm exec vitest run lib/sidebar/task-navigation.test.ts` passed 1 file and 10 tests.
- `pnpm run typecheck` passed.
- `pnpm e2e:run --host --no-build tests/task/sidebar-scroll-preservation.spec.ts --grep "reveals a command-selected task$"` passed 1 Chromium test.
- `pnpm e2e:run --host --no-build --project mobile-chrome tests/task/mobile-command-panel-task-navigation.spec.ts` passed 1 mobile test.
- The web lint, Vite production build, and `git diff --check` passed.

The helper keeps one latest-row cue, guards cleanup by generation, and leaves
hidden or missing rows as no-ops. The existing mobile test remains unchanged
and confirms that phone navigation does not target the desktop sidebar.
