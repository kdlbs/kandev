---
task: 07
title: "Backend projection — parkedState, sampling loop, publish rules"
wave: 2
status: done
depends_on: [02, 04, 05]
spec_acs: AC-21, AC-22, AC-24, AC-25, AC-26, AC-27, AC-28, AC-29, AC-30, AC-36, AC-37,
          AC-38, AC-39, AC-40, AC-40b, AC-41, AC-62, AC-63, AC-64, AC-68, AC-73, AC-74,
          AC-75, AC-77, AC-85
---

# Task 07 — Backend projection

The largest task. Keep to a single owner. Implements the three-term formula, the settle
hook, the sampling loop, and the full entry lifecycle for `parkedState` and `taskParkedState`.

## Data model

### `sessionParkedState` (extend from task-05)

```go
type sessionParkedState struct {
    sessionID        string
    parked           bool
    revision         uint64    // monotonic, increments on true→false and false→true transitions
    observedDetached bool      // set in task-05
    turnMarker       uint64    // set in task-05
    lastSample       string    // "live" | "settled" | "unknown"
}
```

### `taskParkedState`

```go
type taskParkedState struct {
    mu       sync.Mutex
    members  map[string]bool // sessionID → parked bool
    parked   bool            // OR over members
    revision uint64          // own monotonic counter
}
```

### Epoch

`parkedEpoch int64` — set to the backend process start time in Unix nanoseconds, stored
once on orchestrator init (or on first projection access). Exposed via the
`ParkedProjectionProvider` interface added in task-03.

```go
// In orchestrator init or Provide():
parkedEpoch = time.Now().UnixNano()
```

Epoch is the client's restart-survivable discard signal (AC-77): a new epoch means the
backend restarted and revision counters reset to 0.

## Three-term formula

```
parked = observedDetached AND lastSample == "live" AND session.State == WAITING_FOR_INPUT
```

Implement as a method on `*sessionParkedState`:

```go
func (ps *sessionParkedState) computeParked(sessionState string) bool {
    return ps.observedDetached &&
        ps.lastSample == "live" &&
        sessionState == "WAITING_FOR_INPUT"
}
```

## Settle hook — synchronous first sample

The hook plugs into `updateTaskSessionStateWithHook` (line ~800 in
`event_handlers_streaming.go`). This is the ONLY correct seam (D2):

- Do **not** hook `setSessionWaitingForInput` — three workflow sites write
  `WAITING_FOR_INPUT` directly through `updateTaskSessionState` (both step-transition
  settles included).
- Do **not** hook `handleCompleteStreamEvent` — its state write lands late.
- Do **not** use the existing `onChanged` callback — it runs before `publishTaskSessionStateChanged`
  and would delay that publish by the probe budget.

The hook fires **after** the CAS and after publish, gated on
`changed == true && nextState == WAITING_FOR_INPUT`:

```go
// In updateTaskSessionStateWithHook, after the CAS block:
if changed && nextState == WAITING_FOR_INPUT {
    s.onSessionParkedHook(ctx, sessionID)
}
```

**`onSessionParkedHook`** runs synchronously:

1. Take a 5s-budgeted context from the probe budget config.
2. Call `s.backgroundProbe.Probe(ctx, sessionID)` (the `BackgroundProbe` port from task-02).
3. Update `lastSample` on the session's `parkedState`.
4. Recompute `parked` via the three-term formula.
5. If `parked` changed, increment `revision` and publish (see publish rules below).
6. If the task-level `parked` changed, increment `taskParkedState.revision` and publish
   the task update.

The synchronous first sample covers AC-62 and AC-63: the board shows the parked state
within the same state-change publication round.

## Sampling loop

A per-session ticker runs while the session is `WAITING_FOR_INPUT` and `observedDetached`:

```go
func (s *Service) runParkingSampler(ctx context.Context, sessionID string) {
    ticker := time.NewTicker(s.cfg.BackgroundSampleInterval) // default 30s
    defer ticker.Stop()
    for {
        select {
        case <-ticker.C:
            s.sampleAndPublishParked(ctx, sessionID)
        case <-ctx.Done():
            return
        }
    }
}
```

`sampleAndPublishParked` runs one probe, updates `lastSample`, recomputes `parked`, and
publishes if changed — same logic as the hook but called on a ticker.

Start the sampler from the settle hook (after the synchronous first sample) if
`parked == true`. Stop it when `parked` transitions to false or when the session leaves
`WAITING_FOR_INPUT`.

On `turn_started` (a new prompt began): cancel the sampler, reset `lastSample = "unknown"`,
and do NOT recompute parked — the parked state will be re-evaluated at the next settle.

## Publish rules

A `parked` transition (false→true or true→false) is published as:
- A **session** WS notification (the existing session-update event) carrying the updated
  `parked_on_background_work`, `parked_epoch`, and `parked_revision` fields.
- A **task** WS notification carrying updated `parked_on_background_work` and
  `parked_revision` when the task-level OR changes.

Publication is done outside any mutex (drain inside the critical section, publish outside
— mirroring `publishCancellationPending` in `task_operations.go`).

**Publish-failure rule**: if publication fails (channel closed, bus error), log a warning
and continue. Do not re-enqueue.

## Task-level OR

`taskParkedState.members[sessionID] = parked` on every session transition. After updating:

```go
newTaskParked := false
for _, v := range ts.members {
    if v { newTaskParked = true; break }
}
if newTaskParked != ts.parked {
    ts.parked = newTaskParked
    ts.revision++
    // publish task update
}
```

Task revision is its own monotonic counter, independent of session revision (AC-29 covers
this — the spec says the task `parked_revision` is a per-task counter).

## Entry lifecycle

- **Created** on first attestation or first probe — `getOrCreateParkedState` (already exists
  from task-05). For task-07, extend it to also create `taskParkedState`.
- **Eviction reduces (retains `revision`), does not delete** for `parked == false` sessions.
  When `parked == true` at eviction time:
  1. **Publish `parked = false`** (with `revision++`) to the session and task channels.
  2. **Then** remove the row.
  Backend-shutdown eviction publishes nothing (process is exiting).
- Session entries are evicted when an execution is tombstoned or the execution store is
  cleaned. Task entries are evicted when the task is archived or deleted.

## Wire the real provider

Implement `ParkedProjectionProvider` on `*Service` (defined in task-03):

```go
func (s *Service) ParkedProjectionSnapshot(sessionID string) (bool, int64, uint64) {
    ps := s.parkedStateFor(sessionID)
    if ps == nil {
        return false, s.parkedEpoch, 0
    }
    return ps.parked, s.parkedEpoch, ps.revision
}
```

Similarly for `TaskParkedSnapshot`. Pass these providers to `EnrichParked` and
`EnrichTaskParked` at all HTTP and WS serialization points where `EnrichCancellationPending`
is called.

## Tests to write

Use `testing/synctest` to control ticker and timeout behavior.

- **Three-term formula**: test all eight combinations of the three terms; verify parked is
  true only when all three are true.
- **Settle hook**: inject `updateTaskSessionStateWithHook` with `nextState=WAITING_FOR_INPUT`
  and a probe that returns `live`; verify `parked` transitions to true and revision
  increments.
- **Sampling loop**: advance fake time past the interval; verify second probe call and
  `parked` state update on `settled` answer.
- **turn_started resets sampler**: send `turn_started` while sampler is running; verify
  `lastSample = "unknown"` and sampler stops.
- **Eviction publishes un-park**: create a `parked=true` entry, evict it; verify un-park
  notification published before row removal.
- **Task-level OR**: two sessions for one task — one parked, one not → task parked; both
  unpark → task unparked and task revision increments once.
- **AC-62**: synchronous first sample (probe returns `live`) → parked before first ticker.
- **AC-68**: probe returns `settled` mid-sampler → parked → false, published immediately.
- **AC-73**: three consecutive `live` then `settled` via injectable probe → parked transitions.

## Acceptance criteria closed

AC-21 through AC-30 (projection formula), AC-36/37 (real values now), AC-38/39/40 (publish
rules), AC-40b (MCP hook out of scope verified), AC-41 (parked clears on turn_started),
AC-62/63 (synchronous first sample), AC-64 (sampling interval), AC-68 (probe→settled
transitions), AC-73 (injectable probe sequence), AC-74/75 (session vs task revisions),
AC-77 (epoch enables restart detection), AC-85 (eviction un-park publish).
