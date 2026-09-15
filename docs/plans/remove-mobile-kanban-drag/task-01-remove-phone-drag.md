---
id: "01-remove-phone-drag"
title: "Remove phone card dragging"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-MOBILE-KANBAN-SCROLL-001
acceptance_criteria:
  - AC-TASKS-MOBILE-KANBAN-SCROLL-001.1
  - AC-TASKS-MOBILE-KANBAN-SCROLL-001.2
  - AC-TASKS-MOBILE-KANBAN-SCROLL-001.3
  - AC-TASKS-MOBILE-KANBAN-SCROLL-001.4
system_design:
  - ../../specs/tasks/system-design/mobile-kanban-scroll.md
---

# Task 01: Remove phone card dragging

## Summary

Restore phone card scrolling and remove all mobile task drag affordances.
Use TDD: replace the old mobile drag success test, observe the new regression
fail, implement the presentation guard and touch-action fix, then rerun.

## In scope

Mobile drag activation, touch-action, keyboard pickup wiring, drag targets,
focused tests, and a public documentation impact check through docs-maintainer.
Update any published mobile drag guidance with the implementation, not before.

## Out of scope

Other surfaces, tablet/desktop redesign, backend changes, replacement reorder UI.

## Acceptance

1. Phone swipes and delayed swipes scroll cards without task mutations or drag UI.
2. Taps, visible menu moves, and column navigation work on mobile.
3. Tablet/desktop drag and keyboard behavior survives; the 768px boundary works.

## ASCII UI preview

Full context: [plan](plan.md#ascii-ui-preview).

```text
UI-01 Phone: Home > Kanban
[Workflow / Step v] [<] [>]
[Task A              ...]  tap opens; menu moves
[Task B              ...]  swipe vertically scrolls
[Task C              ...]  no task pickup or drag targets

UI-02 Desktop/tablet: Home > Kanban
[Step A         ] [Step B         ]
[Task A      ...] [Task C      ...]
[Task B      ...]  existing drag and keyboard reorder
```

Navigator stays fixed; the phone column scrolls. Layout and control order stay
as shipped. Labels and spacing are illustrative. Maps to AC .1-.4.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/kanban-card-content.test.tsx components/kanban-card-regression.test.tsx components/kanban/swimlane-kanban-content.render-stability.test.tsx)
(cd apps/web && pnpm run typecheck)
GOCACHE=/tmp/kandev-mobile-go-build make -C apps/backend build-dev GOFLAGS=
GOCACHE=/tmp/kandev-mobile-go-build make -C apps/backend e2e-plugin-package GOFLAGS=
(cd apps/web && pnpm run build:e2e)
(cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome tests/kanban/mobile-kanban-reorder.spec.ts tests/kanban/mobile-kanban.spec.ts tests/kanban/mobile-auto-hide-empty-columns.spec.ts)
(cd apps/web && pnpm e2e:run --host --no-build --project chromium tests/kanban/kanban-reorder.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Add any newly changed test suites to this command block before marking done.
Build the native backend and fresh web assets above before managed E2E. Save a phone screenshot; inspect
scrolling and the final card's clearance against UI-01.

## Files likely touched

- apps/web/components/kanban-card.tsx
- apps/web/components/kanban-card-content.tsx
- apps/web/components/kanban/swimlane-kanban-content.tsx
- apps/web/components/kanban/mobile-drop-targets.tsx (only if unreferenced)
- apps/web/components/kanban-card-content.test.tsx
- apps/web/components/kanban-card-regression.test.tsx
- apps/web/components/kanban/swimlane-kanban-content.render-stability.test.tsx
- apps/web/e2e/tests/kanban/mobile-kanban-reorder.spec.ts

## Dependencies

None.

## Inputs

- [Requirements](../../specs/tasks/requirements/mobile-kanban-scroll.md)
- [Design](../../specs/tasks/system-design/mobile-kanban-scroll.md)
- Existing card, mobile reorder, and mobile navigation tests.

## Risks

Keep shared menu move callbacks. Suppressing touch drag alone leaves pointer
activation. Removing keyboard drag attributes must preserve card accessibility.

## Parallelism

sequential

## Results

Implemented phone drag disabling, drag-attribute removal, keyboard card
activation, native touch panning, and removal of mobile drag targets. The visible
Move to path and desktop/tablet sensors remain. Updated the auto-hide mobile
scenario and public workflow troubleshooting guidance.

Validation on 2026-09-15:

- RED: card regression failed because the phone card exposed `draggable`.
- GREEN: the three planned Vitest files passed (29 tests initially); final card
  regression rerun passed 11 tests, yielding 30 passing focused cases overall.
- `pnpm run typecheck`, targeted ESLint, prettier, deleted-path i18n guard,
  native `build-dev`, fixture packaging, and `pnpm run build:e2e` passed.
- Initial E2E could not bind a socket inside the sandbox. Outside the sandbox,
  28 existing mobile-kanban.spec.ts cases passed. The new scroll test initially
  assumed zero scroll after resizing; it now starts a fresh page per width and
  measures gesture movement from the actual baseline. One fixture also returned
  404/503 before reaching the UI; no product change was made for that transient.
- Final focused mobile command with `E2E_DEBUG=1 E2E_PORT_OFFSET=0` and
  `--host --no-build --project mobile-chrome`, selecting mobile-kanban-reorder.spec.ts
  and mobile-auto-hide-empty-columns.spec.ts, passed all 6 cases without retries.
  Captured diagnostics: `/tmp/kandev-mobile-drag-e2e.log`.
- Desktop kanban-reorder.spec.ts passed both pointer/persist and keyboard/persist
  cases with `E2E_PORT_OFFSET=0` and `--host --no-build --project chromium`.
- Specification catalog, full specification lint, and diff whitespace checks pass.

The default managed build cross-compiles unused remote helpers; verification
used the native build targets and a fresh production web bundle instead. The
writable Go build cache was `/tmp/kandev-mobile-go-build`. No backend code,
API, schema, locale copy, or desktop sensor changes were required.


Mobile auto-hide coverage also uses the card Move to menu in this work order,
replacing its removed drag-target interaction.

## Rendered phone verification

The final 393px touch-scroll capture passed and was visually inspected. The
focused step navigator remains fixed, lower cards and the final task are
reachable, the visible menus remain present, and no mobile drag surface appears.
Screenshot: `/tmp/kandev-mobile-scroll-visual/kanban-mobile-kanban-reord-367ae--393px-without-moving-tasks-mobile-chrome/phone-scrolled-cards.png`.
The capture used the focused 393px test with `--output=/tmp/kandev-mobile-scroll-visual`
after desktop verification, so the screenshot survives later default-output cleanup.

## PR review remediation

Reconciled the active adaptive Kanban requirements/design and the auto-hide phone
scenario with native scrolling and menu-based moves. Removed stale promises of
phone drop targets. Specification catalog and full specification lint pass.
No production behavior or rendered UI changed during this documentation repair.
