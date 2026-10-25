---
id: "01-compositor-grid-motion"
title: "Move grid activity motion to the compositor"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-PERSISTENT-STATUS-MOTION-002
acceptance_criteria:
  - AC-UI-PERSISTENT-STATUS-MOTION-002.1
  - AC-UI-PERSISTENT-STATUS-MOTION-002.2
  - AC-UI-PERSISTENT-STATUS-MOTION-002.3
  - AC-UI-PERSISTENT-STATUS-MOTION-002.4
  - AC-UI-PERSISTENT-STATUS-MOTION-002.5
system_design:
  - ../../specs/ui/system-design/persistent-status-motion.md
---

# Task 01: Move Grid Activity Motion to the Compositor

## Summary

Keep the shared grid spinner's nine-cell staggered animation, but establish its
steady transform effects through Web Animations API targets. Preserve CSS as
the fallback and make the existing Quick Chat running flows the desktop and
mobile behavioral evidence.

## In scope

- Add Web Animations lifecycle management to `GridSpinner`.
- Preserve all nine cubes, transform keyframes, duration, easing, delays,
  geometry, color, classes, role, and accessible label.
- Restore one consistent CSS fallback if setup is unavailable or incomplete.
- Add component, desktop E2E, mobile E2E, and node-attributed trace evidence.

## Out of scope

- Replacing the grid with another spinner design.
- Adding `requestAnimationFrame` animation logic.
- Migrating unrelated pulse or rotation indicators.

## Acceptance

- One mounted grid exposes nine running infinite Web Animations effects with
  the existing stagger and remains visibly animated.
- Settling or unmounting cancels every effect; unavailable or partial Web
  Animations setup leaves all nine CSS animations active.
- After setup, an 8.34-second production Chromium trace attributes no recurring
  `UpdateLayoutTree` or `Layerize` work to the grid targets.

## Verification

```bash
cd apps
pnpm install --frozen-lockfile
pnpm --filter @kandev/web exec vitest run components/grid-spinner.test.tsx components/quick-chat/quick-chat-tab-item.test.tsx components/task/chat/message-list-shared.test.tsx --reporter=dot
pnpm --filter @kandev/web exec eslint components/grid-spinner.tsx components/grid-spinner.test.tsx
pnpm --filter @kandev/web typecheck
cd web
pnpm e2e:run --project chromium tests/chat/quick-chat-idle-dot.spec.ts -- --grep "shows tab and sidebar running state"
pnpm e2e:run --project mobile-chrome tests/chat/mobile-quick-chat-idle-dot.spec.ts -- --grep "shows the mobile header running"
```

Capture one production-build Chromium trace with one working Quick Chat tab.
After a one-second settle, inspect an 8.34-second window and record animation
target types plus node-attributed `UpdateLayoutTree` and `Layerize` counts. Run
the Web Animations path and a forced CSS-fallback control.

## Files likely touched

- `apps/web/components/grid-spinner.tsx`
- `apps/web/components/grid-spinner.test.tsx`
- `apps/packages/ui/src/animation-utils.tsx`
- `apps/web/app/globals.css`
- `apps/web/e2e/tests/chat/quick-chat-idle-dot.spec.ts`
- `apps/web/e2e/tests/chat/mobile-quick-chat-idle-dot.spec.ts`

## Dependencies

None.

## Risks

- CSS positive delays and Web Animations delay semantics must match during the
  first cycle as well as steady state.
- Per-cube promotion can increase compositor memory if implementation adds
  permanent promotion hints without trace evidence.

## Parallelism

`sequential`

## Inputs

- `REQ-UI-PERSISTENT-STATUS-MOTION-002` and its acceptance criteria.
- Grid and animation-lifecycle sections of the system design.
- The prior `frontend-idle-cpu` trace controls and `CompositorSpin` pattern.
- Existing Quick Chat desktop and mobile running-state E2E flows.

## Results

- Added nine Web Animations API transform effects with the original four-phase
  scale pattern, 1.3-second duration, easing, and per-cell delays. CSS remains
  active when Web Animations is unavailable or any effect fails to start.
- Added focused coverage for timing, semantics, unsupported-browser fallback,
  partial-setup recovery, and cleanup cancellation. The three focused suites
  pass 63 tests; focused lint and direct web typecheck pass.
- Desktop Chromium and mobile Chrome production E2E each pass the running and
  settled Quick Chat flow. Both assert nine live infinite non-CSS effects; the
  mobile flow also confirms no document overflow.
- The combined 8.34-second trace recorded zero recurring style, layerization,
  layout, paint, or target-invalidation events for the compositor targets. The
  initial forced CSS fallback recorded 47 `UpdateLayoutTree`, 45 `Layerize`,
  and 893 target invalidations; the review-remediation rerun recorded 45, 45,
  and 855 respectively.
