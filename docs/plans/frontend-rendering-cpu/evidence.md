# Rendering attribution evidence

## Capture

- Fixture: disposable Chromium E2E task with one active task and one Quick Chat
  task, both running the slow mock command.
- Runner: Chromium project, one shard and one worker, Playwright 1.61.1.
- Host capacity: 11 logical CPUs were visible to the runner. The capture does
  not use a fixed CPU percentage gate.
- Protocol: one second settle, then three 8.34-second windows per repeated arm.
- Command:

  ```bash
  (cd apps/web && KANDEV_E2E_ANIMATION_TRACE=1 \
    KANDEV_E2E_ANIMATION_TRACE_REPEATS=3 \
    rtk pnpm e2e:run --project chromium \
    tests/chat/animation-performance-trace.spec.ts)
  ```

The trace test records event counts and summed event durations. Nested trace
events are not added as a CPU total.

## Target inventory

The normal-page inventory found the host-owned targets used by the isolation
arms:

- `.spinner-grid-cube`: 18 visible CSS/animation targets in the fixture.
- `[data-testid=chat-input-glow].chat-input-glow-running`: the active composer
  pulse target.
- CSS fallback keyframes: `spinner-grid` and `chat-input-glow-pulse`.
- Other transient inventory entries included sidebar fade/collapse and dialog
  enter animations. No node-level target from the supplied trace was added to
  the production repair list.

The inventory also inspects `::before` and `::after` computed styles and the
document Web Animations list. It is diagnostic only and is not used to pause
arbitrary application animations in production.

## Three-run summaries

Values are shown as median with the observed minimum and maximum in brackets.
Target invalidations are counts of matching invalidation events.

| Arm                  | UpdateLayoutTree count | Layerize count | Target invalidations |            Grid / pulse invalidations |
| -------------------- | ---------------------: | -------------: | -------------------: | ------------------------------------: |
| Normal page          |               6 [0, 6] |       3 [0, 3] |          76 [0, 114] |                72 [0, 108] / 4 [0, 6] |
| CSS fallback control |         272 [235, 272] | 239 [218, 244] | 5,168 [4,465, 5,168] | 4,896 [4,230, 4,896] / 272 [235, 272] |
| Grid paused          |         297 [289, 334] | 279 [275, 303] |       297 [289, 334] |                    0 / 297 [289, 334] |
| Pulse paused         |            32 [31, 33] |    30 [29, 33] |       576 [558, 594] |                    576 [558, 594] / 0 |
| All motion paused    |               1 [0, 1] |       1 [0, 1] |             0 [0, 0] |                                 0 / 0 |

The script-disabled compositor control recorded zero target invalidations. The
normal-page trace includes a zero-event first window, which is retained in the
range rather than discarded. CSS fallback target animations were verified as
running before and after every isolation arm.

## Attribution result

The isolation control is working: pausing the grid removes grid invalidations,
pausing the pulse removes pulse invalidations, and pausing all motion removes
the matched invalidation stream. This attributes the fixture's own fallback
work to those two targets.

It does not attribute the supplied 5.57-second production trace's persistent
Layerize work. That trace lacks node-level style invalidation and already
running-animation attribution, and the small disposable fixture does not
reproduce its continuous Layerize cadence. Task 03 therefore remains pending;
no speculative production repair was made.

The separate confirmed context-ring issue remains bounded to its short value
transition. Task 02 addresses that contract independently.

## Post-integration verification

An earlier post-integration run on 2026-09-10 recorded normal-page medians of
14 `UpdateLayoutTree` events, 12 `Layerize` events, and 228 matched target
invalidations. That run was retained as historical evidence; the controls were
subsequently hardened to keep targets alive and compare each paused group with
the matched script-enabled fallback baseline.

## Review-remediation verification

The three-repeat Chromium command passed on 2026-09-11 after the visibility and
trace-control remediation. Every arm retained 18 grid targets and one pulse
target. The normal compositor arm recorded zero target invalidations in all
three windows. The disabled-script CSS-fallback control recorded 646 target
invalidations at the median [608, 703], split into 612 grid [576, 666] and 34
pulse [32, 37]. The matched script-enabled CSS-fallback baseline recorded 665
target invalidations [627, 703], split into 630 grid [594, 666] and 35 pulse
[33, 37].

| Arm               | Target invalidations | Grid / pulse invalidations |
| ----------------- | -------------------: | -------------------------: |
| CSS fallback baseline | 665 [627, 703] | 630 [594, 666] / 35 [33, 37] |
| Grid paused       | 12 [11, 17]          | 0 / 12 [11, 17]            |
| Pulse paused      | 468 [414, 504]       | 468 [414, 504] / 0          |
| All motion paused | 0 [0, 180]           | 0 [0, 180] / 0             |

The paused group is suppressed against its matched script-enabled baseline in
each selective arm. The unpaused group remains active: pulse invalidations
continue in the grid-paused arm, and grid invalidations continue in the
pulse-paused arm. Residual invalidations in one all-motion window are retained
in the range rather than treated as a strict-zero gate; its median remains
below both baseline group medians. This verifies the bounded attribution
control without identifying the supplied trace's production rendering trigger.
