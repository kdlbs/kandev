---
id: "01-compositor-status-motion"
title: "Move persistent status motion to the compositor"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/persistent-status-motion.md"
---

# Task 01: Move Persistent Status Motion to the Compositor

## Outcome

Persistent task, session, agent, and run indicators keep rotating with the same
meaning and appearance. Their SVG children no longer own the animation.

## Scope

- Add the shared compositor-prepared motion primitive in `apps/packages/ui`.
- Audit long-lived domain-status spinners on task lists, task board cards, the
  focused task page, shared state icons, and Office status rows.
- Migrate the in-scope spinners to an animated HTML wrapper and a static SVG.
- Preserve state precedence, dimensions, color, duration, selectors, tooltip
  behavior, and accessible meaning.
- Add focused component tests and desktop and mobile Playwright assertions.
- Record a repeat Chromium trace in the same steady focused-task state.

## Exclusions

- Short request, save, refresh, upload, and download spinners.
- Task, session, agent, or run state changes.
- New copy or localization keys.
- Changes to reduced-motion behavior.

## Requirements and design

- `REQ-UI-PERSISTENT-STATUS-MOTION-001`
- `AC-UI-PERSISTENT-STATUS-MOTION-001.1` through
  `AC-UI-PERSISTENT-STATUS-MOTION-001.5`
- `docs/specs/ui/system-design/persistent-status-motion.md`

## Acceptance conditions

1. Every in-scope persistent indicator places `animate-spin` and the transform
   promotion hint on an HTML wrapper. Its nested SVG has no animation class.
2. Existing component tests still prove icon precedence and settling. Desktop
   and mobile Playwright tests prove visible rotation, state exit, touch
   navigation, and no horizontal overflow.
3. A repeat steady-state Chromium trace shows that the migrated indicators do
   not account for recurring main-thread `UpdateLayoutTree` or `Layerize` work
   after their layers are established.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
pnpm --filter @kandev/web exec vitest run lib/ui/state-icons.test.tsx components/task/task-item.test.tsx components/kanban-card-content.test.tsx components/task/simple/components/topbar-working-indicator.test.tsx components/task/simple/components/session-timeline-entry.test.tsx components/task/simple/components/user-comment-run-badge.test.tsx app/office/components/agent-card.test.tsx --reporter=dot
cd web && pnpm run typecheck
pnpm e2e:run --project chromium tests/task/sidebar-settled-spinner.spec.ts
pnpm e2e:run --project mobile-chrome tests/task/mobile-task-status-summary.spec.ts
```

Capture and inspect one Chromium performance trace after the automated checks.
Use a focused task with the same persistent running indicators as the supplied
trace. Attribute recurring events to individual animation targets, include a
settled control with unrelated persistent animations disabled, and record the
animation target type and steady-state event counts in this work order's
results.

## Files likely touched

- `apps/packages/ui/src/compositor-spin.tsx`
- `apps/web/lib/ui/state-icons.tsx`
- `apps/web/lib/ui/state-icons.test.tsx`
- `apps/web/components/task/task-item.tsx`
- `apps/web/components/task/task-item.test.tsx`
- `apps/web/components/kanban-card-content.tsx`
- `apps/web/components/kanban-card-content.test.tsx`
- `apps/web/components/task/simple/components/`
- `apps/web/app/tasks/`
- `apps/web/app/office/`
- `apps/web/e2e/tests/task/sidebar-settled-spinner.spec.ts`
- `apps/web/e2e/tests/task/mobile-task-status-summary.spec.ts`

## Dependencies

None.

## Parallelism

Sequential. The shared primitive and its attribute contract must land before
call-site migration and browser evidence.

## Inputs

- The supplied Chromium trace and investigation findings in `plan.md`.
- The persistent status motion requirement and design.
- Existing state-icon precedence tests and task-row test IDs.
- Existing desktop and mobile task-status E2E flows.

## Output contract

Report the audited call sites, migrated files, animation target, preserved
semantics, test results, trace event comparison, risks, and blockers. Update
this task and `plan.md` with exact results.

## Risks

- A wrapper can add an unwanted inline box if size and margin classes move to
  the wrong element.
- An SVG test selector can silently change meaning if it is not moved to the
  status wrapper.
- Other page activity can affect total trace counts. Attribute the comparison
  to the persistent animation targets and record the capture state.

## Results

- Added `apps/packages/ui/src/compositor-spin.tsx` and exported it from the UI
  package. The primitive renders an `inline-flex` HTML `span` with
  `animate-spin will-change-transform`; its nested SVG is static. When
  `Element.animate` is available, it reads the existing CSS duration, disables
  the CSS animation, and starts an infinite linear Web Animations API
  transform animation on the HTML wrapper. The CSS class remains as the
  compatibility fallback, and the Web Animations API instance is cancelled on
  cleanup.
- Migrated the shared state icons, task-list rows, Kanban cards, focused-task
  topbar/session/agent/run indicators, and Office task/agent/run rows. Existing
  dimensions, colors, selectors, state precedence, labels, and mobile layout
  remain on the same surfaces.
- Component coverage includes the static-SVG wrapper assertion and a Web
  Animations API assertion for the live compositor path. Desktop and mobile
  Playwright status flows each passed one test, including settling behavior,
  mobile touch navigation, and overflow checks.
- A node-attributed 8.34-second Chromium trace found the migrated target as
  `span.inline-flex.animate-spin.will-change-transform` with a static SVG child.
  With the page's nine `spinner-grid-cube` animations enabled, the capture
  recorded 150 `UpdateLayoutTree` and 150 `Layerize` events. The trace
  attributed 1,350 invalidations to those grid-cube elements and 150
  co-occurring invalidations to the target wrapper.
- The decisive production control kept the Web Animations API-backed
  `CompositorSpin` running, disabled the grid-cube animation and all other
  persistent status wrappers, and waited one second for the page to settle.
  The following 8.34-second window recorded zero `UpdateLayoutTree`,
  `Layerize`, `Layout`, or `Paint` events and no target invalidations. The
  target had zero class or style mutations during the window. Its four
  `animate` calls were one-time lifecycle setup before the settled window. A
  CSS-animation control for the same target
  recorded 41 `UpdateLayoutTree` and 41 `Layerize` events with target
  invalidations, while the Web Animations API control had no recurring events.
  This confirms that the remaining enabled-page frame work is unrelated grid
  animation, and closes the performance acceptance condition.
- Typecheck, lint, i18n checks, spec lint, and `git diff --check` passed.
- Review remediation also marks the `STARTING` session icon as animated, keeps
  the accessible label on the agent status SVG, and strengthens the wrapper
  assertions so a missing child SVG cannot pass silently.
- The full frontend suite surfaced one stale Kanban status-icon shape helper;
  it now unwraps `CompositorSpin`, and that focused file passes all 17 tests.
