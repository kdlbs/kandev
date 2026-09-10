---
created: 2026-09-10
status: in_progress
requirements:
  - REQ-UI-PERSISTENT-STATUS-MOTION-001
  - REQ-UI-PERSISTENT-STATUS-MOTION-002
  - REQ-UI-PERSISTENT-STATUS-MOTION-003
  - REQ-UI-PERSISTENT-STATUS-MOTION-004
  - REQ-UI-PERSISTENT-STATUS-MOTION-005
system_design:
  - ../../specs/ui/system-design/persistent-status-motion.md
legacy_specs: []
---

# Implementation Plan: Reduce Remaining Task Rendering CPU

## Overview

Identify the remaining continuous rendering trigger, make a targeted repair,
then stop spending resources on hidden status motion. Keep visible feedback
and task behavior intact. Implementation is in progress.

UI owns the reusable motion and visibility contract. Task/session state,
transport, persistence, and plugin implementations retain their current owners.

## Evidence and limits

The supplied Trace-20260910T181155.json.gz contains 5.57 seconds for PID 2960.
It records 752 Layerize passes (1,079 ms), 213 ms PrePaint, 187 ms
UpdateLayoutTree, and 182 ms FunctionCall. Full steady seconds contain
139–144 Layerize passes. These event categories can nest and are not additive
CPU totals. Profiler startup contributes a separate one-time 249 ms.

The trace proves rendering cost, but lacks the invalidation attribution needed
to identify the continuous trigger. Persistent animation is a hypothesis.
The context-ring scrollbar-color and stroke-dashoffset transitions are confirmed
but last only about 300 ms. Roughly 21,100 nodes do not establish a leak or
justify virtualization by themselves.

The exact hashed asset in the capture contains the previous compositor fixes.
[Idle CPU](../frontend-idle-cpu/plan.md),
[animation CPU](../frontend-animation-cpu/plan.md), and
[runtime CPU](../frontend-runtime-cpu/plan.md) are completed packages.
Preserve their results; this package does not repeat those migrations.

## Scope

### In scope

- Reproducible normal-page and animation-isolation captures.
- Narrow the context-ring transition.
- Repair the remaining attributed host-owned rendering trigger.
- Pause host-owned spin, grid, and pulse motion when hidden.
- Verify visible feedback, lifecycle recovery, and measured rendering results.

### Out of scope

- Transcript virtualization or unmounting retained editors and panels.
- Global animation disabling, slower visible feedback, or an energy setting.
- Backend, WebSocket, logging, and state-library changes.
- Editing Kandy or another production plugin inside this repository.
- Fixed cross-machine CPU percentages or a wall-clock performance CI gate.

## Technical approach

Use the proposed extension in the
[existing system design](../../specs/ui/system-design/persistent-status-motion.md#proposed-rendering-cpu-follow-up)
and its paired
[requirements](../../specs/ui/requirements/persistent-status-motion.md#proposed-follow-up-requirements).

1. Extend the existing gated Chromium trace test. Its current script-disabled,
   CSS-suppressed control cannot substitute for a normal-page baseline.
2. Restrict ContextWindowRing to the intended arc transition.
3. Use pause/restore attribution to select one minimal repair. Task 03 has an
   evidence gate: populate exact target files and a failing regression from
   Task 01 before changing production code. If the cause lies outside the
   reviewed motion boundary, revise the design instead of guessing.
4. Share visibility lifecycle handling across existing motion primitives.
   Preserve each primitive's timing, fallback, cleanup, and reduced-motion rules.

Do not add an ADR: this proposal keeps current package, state, and persistence
boundaries. No public documentation change is expected because it adds no
setting, command, terminology, or user workflow.

## ASCII UI preview

UI-01: Existing task presentation, desktop and phone, active then settled.

```text
Desktop: [Task list] | [Task chat: transcript] | [Changes]
Phone:  [Task header]
        [Chat transcript]
        [Composer: activity glow; usage ring]
        [Existing bottom navigation]

Visible + active -> existing moving indicator
Hidden + active  -> motion paused; task updates continue
Visible + settled -> existing settled indicator; no replay
Usage update -> ring arc moves; threshold color updates directly
```

This is a motion-only change. Existing grouping and navigation are structural
requirements; spacing above is illustrative. Desktop split panes that remain
visible keep motion. Phone keeps the existing Chat scroll owner, safe areas,
and touch controls. No new control or text is introduced.
Maps to AC-UI-PERSISTENT-STATUS-MOTION-004.1 through .5 and -005.1 through .3.

## Tests

- Existing token-usage-display.test.tsx: value, threshold color, and disclosure
  regressions; rendered computed-transition checks prove -005.1 through .3.
- New lib/ui/persistent-motion-visibility.test.tsx: hidden mount, resume,
  settled/unmount while hidden, missing observer, repeated toggles, and cleanup.
- Existing lib/ui/compositor-pulse.test.tsx, lib/ui/state-icons.test.tsx, and
  components/grid-spinner.test.tsx: fallback, effect replacement, reduced motion,
  and owned-handle pause/resume. Maps to -004.1 through .5 and existing -001/002/003.
- Task 03 adds the target-specific failing regression identified by Task 01.
  Broad memoization or class-only assertions cannot prove the CPU repair.

## E2E tests

Extend existing chat/persistent-animation-motion.spec.ts and its mobile partner.
Cover visible busy motion, offscreen/panel pause, restoration, and settlement
while hidden. Include actual browser document-visibility evidence when the
runner supports it; a synthetic event alone cannot prove a hidden tab.
Use deterministic unit coverage for visibility event handling.

Extend chat/context-window-source.spec.ts and its mobile partner for ring
updates, intended transition properties, and disclosure behavior.
Use chat/animation-performance-trace.spec.ts for normal-page and isolated
controls. It must retain target presence assertions to prevent a vacuous pass.

## Work orders

- [x] [Task 01: Attribute continuous rendering](task-01-attribute-rendering.md)
- [x] [Task 02: Bound context-ring transitions](task-02-bound-ring-transitions.md)
- [ ] [Task 03: Repair the attributed rendering trigger](task-03-repair-rendering-trigger.md)
- [x] [Task 04: Pause hidden status motion](task-04-pause-hidden-motion.md)

Execute sequentially: 01 -> 02 -> 03 -> 04 when the evidence gate is met.
Task 03 requires evidence, not elapsed time. If no cause is reproduced, report
that result and keep it pending. Task 02 and the specified visibility work can
proceed without claiming the main CPU problem is solved. No subagent
authorization is implied.

## Verification protocol

Bootstrap a fresh worktree once with
`(cd apps && rtk pnpm install --frozen-lockfile)`.
Each work order lists its targeted commands, run from the repository root.
Managed E2E commands build production assets and use isolated data.
No production traces or real task transcripts are committed.

For each performance arm, settle for one second and collect 8.34 seconds.
Repeat three times with identical browser, fixture, viewport, and display
cadence. Keep script execution enabled for the normal-page arms. Capture a
separate disabled-script control only to isolate CSS/compositor work.
Report target-level invalidations and page-level layer/style work separately.

Success requires the reproduced target to stop causing recurring main-thread
work while visible motion remains correct. Hidden primitives must pause and
resume without extra effects or stale status. Record CPU medians and ranges;
a smaller token-ring transition alone does not satisfy the main repair.
If layerization persists, retain the open finding and document the next
attribution step. Investigate DOM-size controls before proposing virtualization.

## Verification results

Planning: specification linter tests passed (36 tests); all specification files
passed lint; documentation whitespace and local-link checks passed.
Task 01: three-repeat Chromium capture passed; see [evidence.md](evidence.md).
Task 02: bounded ring transition implemented and verified by unit plus desktop
and mobile browser tests.
Task 03: remains pending because the fixture isolation did not reproduce the
supplied trace's persistent Layerize cadence.
Task 04: shared visibility lifecycle implemented for spin, grid, and pulse;
focused unit tests, desktop/mobile persistent-motion tests, desktop/mobile
Quick Chat tests, and a post-integration three-repeat trace passed. See
[evidence.md](evidence.md).

## Risks

- Browser compositor behavior varies with browser build, refresh rate, and DOM.
- Existing trace controls can hide a real JavaScript invalidation source.
- Intersection observation can miss owner-specific hidden states; use existing
  visibility ownership without treating visible split panes as inactive.
- Pausing fallback CSS must not suppress later visible or reduced-motion states.
- The attached trace cannot establish an exact production repair in advance.
