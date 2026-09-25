---
created: 2026-09-15
status: implemented
requirements:
  - REQ-TASKS-MOBILE-KANBAN-SCROLL-001
system_design:
  - ../../specs/tasks/system-design/mobile-kanban-scroll.md
legacy_specs: []
---

# Remove mobile Kanban task dragging

## Overview

One sequential work order disables phone task dragging and restores native
scrolling with focused regression coverage. Implementation and targeted verification are complete.

## Scope

Phone Kanban card gestures and scroll behavior. Preserve visible task-menu
moves, navigation, desktop/tablet interactions, and stored ordering. Exclude
other drag surfaces, backend changes, and new ordering controls.

## Technical approach

Apply the [design](../../specs/tasks/system-design/mobile-kanban-scroll.md)
at the card presentation boundary and remove mobile drag-only affordances.
The existing shared pointer sensor also accepts touch: disabling TouchSensor
alone is insufficient. Remove the card's phone touch-action restriction too.

## ASCII UI preview

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

## Tests and E2E tests

AC .1/.4: extend kanban-card-regression.test.tsx and
kanban-card-content.test.tsx with mobile/desktop interaction checks; cover
responsive wiring in swimlane-kanban-content.render-stability.test.tsx.
AC .1/.2/.3/.4: replace the positive phone drag test in
mobile-kanban-reorder.spec.ts with native touch scrolling/no mutations, delayed
swipe, tap/menu move, and boundary cases. Retain mobile-kanban.spec.ts for
horizontal navigation and general phone flows. Run kanban-reorder.spec.ts on
chromium for desktop reorder compatibility. All ACs use the prefix
`AC-TASKS-MOBILE-KANBAN-SCROLL-001`.

## Work orders

- [x] [Task 01: Remove phone card dragging](task-01-remove-phone-drag.md)

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

## Verification results

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

## Risks

Shared drag code also supplies Move to; retain that action. CSS alone cannot
disable sensors. Gesture tests must use actual touch input. Existing mobile
reorder coverage deliberately changes expectation.

Mobile auto-hide coverage also uses the card Move to menu in this work order,
replacing its removed drag-target interaction.

## Rendered phone verification

The final 393px touch-scroll capture passed and was visually inspected. The
focused step navigator remains fixed, lower cards and the final task are
reachable, the visible menus remain present, and no mobile drag surface appears.
Screenshot: `/tmp/kandev-mobile-scroll-visual/kanban-mobile-kanban-reord-367ae--393px-without-moving-tasks-mobile-chrome/phone-scrolled-cards.png`.
The capture used the focused 393px test with `--output=/tmp/kandev-mobile-scroll-visual`
after desktop verification, so the screenshot survives later default-output cleanup.
