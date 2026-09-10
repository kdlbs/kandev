# Task 05: Parked projection (backend, session-level, `BackgroundProbe` port)

Spec: §"Data model → Parked projection", D1, D2, D3, D8, State machine.
ACs: AC-21, AC-24, AC-25, AC-26, AC-27, AC-40, AC-40a, AC-53, AC-54, AC-68.
Round-5 F2 (AC-35 architecture test) and F7 (ordering) close here.

Depends on: task-01 (`observed_detached`), task-02 (recogniser registry),
task-04 (`BackgroundProbe` port + config). Does **not** depend on task-03 (the
real probe) — build and test against the port's test double, per the spec's
own "buildable in parallel" guidance.

## State

```go
type parkedState struct {
    parked          bool
    revision        uint64 // increments on transition only, never on read
    observedDetached bool
    lastSample      probeResult // live | settled | unknown, zero value = unknown
    lastSampledAt   time.Time
}
// keyed by session ID, one mutex, mirroring CancellationPendingSnapshot's
// single-critical-section pattern (task_operations.go:4478).
```

`parked` is true iff all three hold: `observedDetached`, `lastSample == live`,
session state `== WAITING_FOR_INPUT`. No `sampling` field (round-4 f14,
deliberately removed).

## F7 — ordering (round-5, load-bearing)

`session.turn_finished` MUST be emitted **before** the synchronous probe is
taken during turn-settle handling, so AC-76 (sibling spec's guard, still
asserted by this spec as "not delayed") holds unconditionally regardless of
how long the probe takes. Locate the turn-settle handler
(`publishAgentTurnComplete`, `event_handlers_streaming.go:436`) and insert the
probe call **after** the point that publishes `turn_finished`, not before.
Write the ordering test first (assert publish order via the recording event
bus) before wiring the probe call in, so this cannot regress silently.

## D2 — synchronous first sample

Only taken when `observedDetached` is true for the settling turn (AC-40a: zero
probe latency otherwise). Bounded by `KANDEV_PARKED_PROBE_BUDGET` via
`context.WithTimeout`. Timeout/error → `unknown`, not parked, settlement not
delayed beyond the budget (AC-40).

## Sampling loop (backend-owned)

One loop, all parked sessions, `KANDEV_PARKED_PROBE_INTERVAL` cadence. Starts
when a session enters parked; samples each parked session per tick
(concurrent samples for one session serialized, D6); stops on: probe returns
non-`live`, session leaves `WAITING_FOR_INPUT`, session stopped/deleted/ended,
or backend shutdown (AC-53). A session that is never parked is never probed
(AC-54 — zero probes for a plain `WAITING_FOR_INPUT` session with no
attestation).

## D8 / AC-68 — session-state term clears without a re-sample

When the session leaves `WAITING_FOR_INPUT` (self-resume or admitted prompt),
`parked` must flip to `false` **immediately** via the third term, without
waiting for or forcing a new probe sample (`lastSample` may still read `live`
and is never re-read for this transition). Wire this off the existing
session-state-transition path, not the sampling loop.

## F2 — AC-35 architecture test (round-5 disposition, see plan.md)

Add `internal/orchestrator/parked_projection_no_flag_test.go` (or similar)
that reads the **source text** of the specific new files this task and task-06
add (list them by exact path in the test — do not glob a package) and asserts
none contain `claudeBackgroundPromptHandoffEnabled` or
`claudeBackgroundPromptHandoff`. This is the scoped, satisfiable form (round-5
F2): package-granularity is impossible since the projection necessarily lives
in `internal/orchestrator`, the same package as the flag's accessor.

## Tests

- AC-21/24/25/26/27: each combination of `observedDetached` × probe result ×
  session state, using the `BackgroundProbe` test double.
- AC-40/40a: budget timeout → `unknown`; no attestation → probe never called
  (assert via a spy on the test double).
- AC-53/54: loop start/stop conditions, zero-probe case.
- AC-68: session leaves `WAITING_FOR_INPUT` → `parked` flips to `false`
  without a new `Probe()` call (assert via spy call count).
- F7 ordering test (see above).
- F2 architecture test (see above).
- AC-37 end-to-end: unregistered agent → `observedDetached` never set →
  never parked, regardless of probe result.
