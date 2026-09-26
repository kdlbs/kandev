---
id: "02-compositor-opacity-motion"
title: "Move persistent opacity motion to the compositor"
status: completed
wave: 2
depends_on:
  - "01-compositor-grid-motion"
plan: "plan.md"
requirements:
  - REQ-UI-PERSISTENT-STATUS-MOTION-003
acceptance_criteria:
  - AC-UI-PERSISTENT-STATUS-MOTION-003.1
  - AC-UI-PERSISTENT-STATUS-MOTION-003.2
  - AC-UI-PERSISTENT-STATUS-MOTION-003.3
  - AC-UI-PERSISTENT-STATUS-MOTION-003.4
  - AC-UI-PERSISTENT-STATUS-MOTION-003.5
  - AC-UI-PERSISTENT-STATUS-MOTION-003.6
system_design:
  - ../../specs/ui/system-design/persistent-status-motion.md
---

# Task 02: Move Persistent Opacity Motion to the Compositor

## Summary

Keep the busy composer glow and long-lived status pulses visibly animated, but
run opacity through a shared HTML Web Animations primitive. Use the optimized
grid from Task 01 as the trace baseline so remaining target costs can be
attributed independently.

## In scope

- Add the narrow `CompositorPulse` UI primitive with CSS fallback and cleanup.
- Replace the chat-input glow pseudo-element with a pointer-inert HTML target
  that preserves the current shadows, opacity range, and cadence.
- Audit and migrate persistent `animate-pulse` task, session, agent, and run
  status targets, beginning with the live agent tab indicator.
- Preserve active-state ownership, layout, semantics, selectors, responsive
  composition, and reduced-motion behavior.
- Add component, desktop E2E, mobile E2E, and node-attributed trace evidence.

## Out of scope

- One-shot selection, request-changes, search, or confirmation pulses.
- Status-state or composer interaction changes.
- Plugin-owned motion.

## Acceptance

- Busy/starting composer glows and audited persistent status dots retain their
  existing visible pulse and stop when their owning state ends.
- The HTML targets use running infinite Web Animations effects when supported,
  preserve CSS fallback and current reduced-motion suppression, and cancel on
  cleanup.
- After setup, an 8.34-second production Chromium trace attributes no recurring
  `UpdateLayoutTree` or `Layerize` work to migrated opacity targets.

## Verification

```bash
cd apps
pnpm install --frozen-lockfile
pnpm --filter @kandev/web exec vitest run lib/ui/compositor-pulse.test.tsx components/task/chat/chat-input-body.test.tsx components/task/simple/chat-activity-tabs.test.tsx app/office/agents/components/agent-status-dot.test.tsx app/office/components/execution-indicator.test.tsx --reporter=dot
pnpm --filter @kandev/web exec eslint ../packages/ui/src/compositor-pulse.tsx lib/ui/compositor-pulse.test.tsx components/task/chat/chat-input-body.tsx components/task/chat/chat-input-body.test.tsx components/task/simple/chat-activity-tabs.tsx components/task/simple/chat-activity-tabs.test.tsx app/office/agents/components/agent-status-dot.tsx app/office/agents/components/agent-status-dot.test.tsx app/office/components/execution-indicator.tsx app/office/components/execution-indicator.test.tsx
pnpm --filter @kandev/web typecheck
cd web
pnpm e2e:run --project chromium tests/chat/persistent-animation-motion.spec.ts
pnpm e2e:run --project mobile-chrome tests/chat/mobile-persistent-animation-motion.spec.ts
```

Capture one production-build Chromium trace with a running task, its composer
glow, and the migrated persistent status targets visible. After a one-second
settle, inspect an 8.34-second window and record target types plus node-attributed
`UpdateLayoutTree` and `Layerize` counts. Compare Web Animations and forced CSS
fallback controls with React DevTools disabled.

## Files likely touched

- `apps/packages/ui/src/compositor-pulse.tsx`
- `apps/packages/ui/src/animation-utils.tsx`
- `apps/web/lib/ui/compositor-pulse.test.tsx`
- `apps/web/components/task/chat/chat-input-body.tsx`
- `apps/web/components/task/chat/chat-input-body.test.tsx`
- `apps/web/components/task/simple/chat-activity-tabs.tsx`
- `apps/web/components/task/simple/chat-activity-tabs.test.tsx`
- `apps/web/app/globals.css`
- Additional persistent pulse call sites and their focused tests found by the
  audit.
- `apps/web/e2e/tests/chat/persistent-animation-motion.spec.ts`
- `apps/web/e2e/tests/chat/mobile-persistent-animation-motion.spec.ts`
- `apps/web/e2e/helpers/animation-assertions.ts`

## Dependencies

- Task 01 establishes the optimized grid baseline used by the final trace.

## Risks

- Replacing a pseudo-element can alter z-index or clipping around the composer.
- A broad `animate-pulse` migration would accidentally include bounded cues;
  the audit must classify each call site by state lifetime.
- Reduced-motion CSS must be read before the Web Animations effect starts, or
  the new path could override the user's current preference.

## Parallelism

`sequential`

## Inputs

- `REQ-UI-PERSISTENT-STATUS-MOTION-003` and its acceptance criteria.
- Opacity motion, lifecycle, and responsive sections of the system design.
- Task 01 trace evidence and the existing `CompositorSpin` lifecycle pattern.
- Current chat-input and live-agent status component tests.

## Results

- Added `CompositorPulse`, which derives duration, delay, and easing from the
  existing CSS declaration, replaces it only after Web Animations setup
  succeeds, respects reduced motion, and cancels and restores CSS on cleanup.
- Replaced the composer pseudo-element with an isolated, pointer-inert HTML
  glow while retaining the running and starting shadows, cadence, state
  precedence, and responsive layout.
- Migrated the live task agent indicator, working Office agent indicator, and
  live execution indicator. The audit left bounded skeletons, one-shot cues,
  integration check status, and connection status unchanged because their
  lifetime or ownership is outside persistent task, session, agent, and run
  motion.
- The six focused component suites pass 32 tests, including exact timing,
  endpoint phase, unsupported fallback, reduced-motion suppression and
  preference changes, cleanup, state precedence, and static-state behavior.
  Focused ESLint and the direct web TypeScript check pass.
- Desktop Chromium and mobile Chrome production E2E pass the composer
  running-to-settled flow; the mobile flow also confirms no horizontal
  overflow.
- The gated 8.34-second production trace recorded zero `UpdateLayoutTree`,
  `Layerize`, `Layout`, `Paint`, or target-invalidation events for the
  compositor path. Its initial forced CSS fallback control recorded 47
  `UpdateLayoutTree`, 45 `Layerize`, zero `Layout`, zero `Paint`, and 893 target
  invalidations; the review-remediation rerun recorded 45 `UpdateLayoutTree`,
  45 `Layerize`, zero `Layout`, zero `Paint`, and 855 target invalidations.
