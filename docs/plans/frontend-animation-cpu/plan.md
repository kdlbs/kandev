---
created: 2026-08-28
status: completed
requirements:
  - REQ-UI-PERSISTENT-STATUS-MOTION-002
  - REQ-UI-PERSISTENT-STATUS-MOTION-003
system_design:
  - ../../specs/ui/system-design/persistent-status-motion.md
legacy_specs: []
---

# Implementation Plan: Frontend Animation CPU

## Overview

Keep Kandev's persistent activity animations while removing the recurring
main-thread style and layer work found in the supplied Chromium trace. First
move the shared nine-cell grid spinner to compositor-backed Web Animations.
Then move the task composer glow and other persistent opacity pulses to an HTML
opacity primitive, using the first change as the performance baseline.

## Scope

### In scope

- Preserve the existing animated nine-cell grid indicator and its stagger.
- Preserve the animated running and starting composer glows.
- Preserve long-lived task, session, agent, and run opacity pulses found by the
  implementation audit.
- Keep CSS animation fallbacks and current reduced-motion behavior.
- Prove desktop and mobile animation behavior and repeat the node-attributed
  Chromium performance capture.

### Out of scope

- Removing, freezing, slowing, or visually replacing the animations.
- Changing task, session, agent, run, or composer state rules.
- Migrating bounded request spinners or one-shot attention and transition cues.
- Kandy plugin celebration changes. They belong to the plugin's dedicated
  repository and were a temporary, secondary cost in this capture.
- Adding a user setting, runtime feature flag, or backend contract.

## Technical approach

### Grid spinner

Update `apps/web/components/grid-spinner.tsx` so each existing cube uses an
infinite Web Animations API transform effect with the same keyframes, duration,
easing, and stagger as `spinner-grid` in `apps/web/app/globals.css`. Keep the
CSS classes as the unsupported-browser fallback and preserve the wrapper's
status semantics. Add focused tests for all nine effects, fallback, and cleanup.

### Opacity motion

Add `CompositorPulse` beside `CompositorSpin` in `apps/packages/ui/src/`. Move
the chat-input glow from its pseudo-element to a pointer-inert HTML target that
retains the existing box shadows and cadence. Audit long-lived `animate-pulse`
call sites and migrate only domain-status motion, beginning with the live agent
tab indicator captured on the active task surface. Keep bounded and one-shot
motion unchanged.

### Desktop and mobile behavior

The change does not alter composition. The existing Quick Chat tab and task
chat composer are the desktop and mobile exemplars. Their current actions,
touch targets, scroll ownership, focus behavior, safe-area behavior, and labels
remain unchanged. Shared motion targets are decorative or status-only and do
not intercept pointer events. Mobile Playwright coverage verifies the same
running-to-settled outcome and no document horizontal overflow.

## Tests

- `apps/web/components/grid-spinner.test.tsx` covers Web Animations setup,
  nine-cell timing, fallback, cancellation, and status semantics for
  `AC-UI-PERSISTENT-STATUS-MOTION-002.1` through `.5`.
- `apps/web/lib/ui/compositor-pulse.test.tsx` covers opacity timing, fallback,
  reduced-motion suppression, and cancellation for
  `AC-UI-PERSISTENT-STATUS-MOTION-003.3`, `.4`, and `.6`.
- `apps/web/components/task/chat/chat-input-body.test.tsx` covers running,
  starting, settled, and unmounted glow targets for
  `AC-UI-PERSISTENT-STATUS-MOTION-003.1` through `.4`.
- Existing status component tests cover each migrated persistent pulse's state
  precedence and selectors.

## E2E tests

- Extend `apps/web/e2e/tests/chat/quick-chat-idle-dot.spec.ts` to prove the
  desktop grid animation runs through nine Web Animations effects and stops
  when work settles.
- Extend `apps/web/e2e/tests/chat/mobile-quick-chat-idle-dot.spec.ts` with the
  same user outcome and mobile overflow assertion.
- Add focused desktop and `mobile-` task-chat motion specs under
  `apps/web/e2e/tests/chat/` to prove the busy composer glow remains animated,
  stops when the turn settles, and preserves mobile containment.

## Work orders

- [x] [Task 01: Move grid activity motion to the compositor](task-01-compositor-grid-motion.md)
- [x] [Task 02: Move persistent opacity motion to the compositor](task-02-compositor-opacity-motion.md)

## Verification results

- Added compositor-backed transform effects for all nine grid cells and a
  shared compositor-backed opacity pulse. Both paths retain their CSS fallback,
  cancellation, and reduced-motion contracts.
- Migrated the busy/starting composer glow, live task agent indicator, Office
  working-agent indicator, and live execution indicator. Bounded skeletons,
  one-shot attention cues, integration check status, and connection status
  remain on their existing paths because they are not persistent domain motion.
- Focused component coverage passes: 63 tests for the grid integration set and
  32 targeted opacity/status and grid tests after review remediation. Focused
  ESLint and the direct web TypeScript check pass.
- Desktop Chromium and mobile Chrome production E2E pass for both Quick Chat
  grid motion and composer running-to-settled motion. Mobile checks also confirm
  that the animation targets do not introduce horizontal document overflow.
- The gated 8.34-second production Chromium comparison recorded zero
  `UpdateLayoutTree`, `Layerize`, `Layout`, `Paint`, or target-invalidation
  events for the compositor path. The forced CSS fallback recorded 47
  `UpdateLayoutTree`, 45 `Layerize`, zero `Layout`, zero `Paint`, and 893 target
  invalidations in the initial capture; the remediation rerun recorded 45
  `UpdateLayoutTree`, 45 `Layerize`, zero `Layout`, zero `Paint`, and 855 target
  invalidations.

## Risks

- Nine Web Animations effects must preserve the grid's phase offsets without
  creating nine unnecessary permanent `will-change` layers.
- Inline CSS fallback suppression can leave an element static after a class
  change unless cleanup restores the declaration before recomputing timing.
- Moving a glow from a pseudo-element to real markup can change stacking,
  clipping, or pointer targeting if its isolated wrapper contract is lost.
- Total page trace counts include unrelated extensions and plugin animations;
  acceptance depends on node attribution, not a page-wide zero count.
