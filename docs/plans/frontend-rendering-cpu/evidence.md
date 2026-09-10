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

After the visibility helper was integrated into the spin, grid, and pulse
primitives, the same three-repeat command passed again on 2026-09-10. The
normal-page medians were 14 `UpdateLayoutTree` events, 12 `Layerize` events,
and 228 matched target invalidations. The ranges were 11–23, 11–17, and
209–482 respectively. The median target split was 216 grid and 12 pulse
invalidations, with ranges of 198–462 and 11–20.

The final run's CSS-fallback control emitted zero matching invalidations in all
three windows. Because the fallback arm did not produce a running target
stream, it is not used as new attribution evidence; the earlier fallback
capture above remains the diagnostic comparison. The grid-paused,
pulse-paused, and all-motion-paused arms each emitted zero matched target
invalidations and zero `UpdateLayoutTree` and `Layerize` events in all three
windows. This verifies the pause controls without identifying the supplied
trace's production rendering trigger.
