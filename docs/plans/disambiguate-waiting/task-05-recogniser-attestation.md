---
task: 05
title: "Recogniser + attestation (observed_detached, turn_marker)"
wave: 1
status: done
depends_on: [01]
spec_acs: AC-69, AC-69a, AC-37 (second GIVEN), AC-41b (turn_marker)
---

# Task 05 — Recogniser + attestation

Track two facts per session in the orchestrator's ordered stream consumer:
1. **`observed_detached`**: has this session ever seen a `IsDetachedBackgroundLaunch() &&
   Kind == BackgroundWorkKindShell` tool call?
2. **`turn_marker`**: which turn (monotonic counter, incremented on `turn_started`) was
   active when the most recent detached shell launch was observed?

These feed the projection formula in task-07 without doing any projection yet.

## Key constraints from the spec

- **The recogniser registry is in agentctl** (already exists as `stampBackgroundShellWork`
  in `normalize.go`). The backend reads one vendor-neutral predicate:
  `NormalizedPayload.IsDetachedBackgroundLaunch()`. Do **not** key anything backend-side
  on `payload.AgentID` or `session.AgentProfileSnapshot["agent_name"]` (ADR-0049).
- **The `Kind == BackgroundWorkKindShell` filter is not optional.** `stampSubagentBackgroundWork`
  stamps `Detached=true` with `Kind=subagent` for both `claudeAgentID` and `mockAgentID`.
  An unfiltered predicate breaks AC-37's second GIVEN in every `dev` and `e2e` profile.
- **Both fields live in the ordered consumer** (`handleAgentStreamEvent`), not in the
  adapter or the normaliser. The attestation is proven to survive the process boundary
  via `MarshalJSON` / `UnmarshalJSON` on `background_work` in `tool_payload.go`.

## Files to change

### 1. New `internal/orchestrator/parked_state.go`

Define the per-session struct that holds `observed_detached` and `turn_marker` (and, in
task-07, `last_sample`, `parked`, and `revision`). For task-05, only these two fields:

```go
// sessionParkedState is the runtime-only per-session tracking for the
// parked-on-background-work projection.
type sessionParkedState struct {
    observedDetached bool
    turnMarker       uint64 // increments per turn_started event
}
```

Add a map `parkedStates map[string]*sessionParkedState` to the orchestrator `Service`
struct (protected by an appropriate mutex — see lock-order note below).

Add helpers:
```go
func (s *Service) getOrCreateParkedState(sessionID string) *sessionParkedState
func (s *Service) parkedStateFor(sessionID string) *sessionParkedState // returns nil if absent
```

### 2. `internal/orchestrator/event_handlers_streaming.go`

**`handleAgentStreamEvent`** — add two new cases:

**Case A: `EventTypeTurnStarted`** — replace the Wave-0 no-op placeholder:

```go
case streams.EventTypeTurnStarted:
    ps := s.getOrCreateParkedState(event.SessionID)
    ps.turnMarker++
    return
```

(Lock guard on the parked-state map, not a global service mutex.)

**Case B: tool-call event with a detached shell background launch** — inside the existing
tool-call branch (`handleToolCallEvent` or after the switch), add a check:

```go
if event.Payload != nil &&
    event.Payload.IsDetachedBackgroundLaunch() &&
    event.Payload.Kind() == streams.BackgroundWorkKindShell {
    ps := s.getOrCreateParkedState(event.SessionID)
    ps.observedDetached = true
    // turnMarker is already current: turn_started precedes the tool-call
    // on the same FIFO queue.
}
```

`IsDetachedBackgroundLaunch()` is at `types/streams/background_work.go:62-64`. The Kind
accessor is from the same package. Do not inline the detection logic.

### 3. Lock ordering

The parked-state map is accessed only from the single ordered-consumer goroutine
(`handleAgentStreamEvent`), so no mutex is needed if it is exclusively owned by that
goroutine. If the map must also be read from the sampling loop (task-07) or the settle
hook, use a dedicated `sync.RWMutex` for `parkedStates`, NOT the session-model mutex
(to avoid deadlock with the settle seam that holds the session mutex while calling the
hook). Document the lock order in the file header.

### 4. Session eviction / cleanup

When an execution is tombstoned or removed, delete its entry from `parkedStates`.
Follow the pattern where `observedDetached` resets: the sampling loop (task-07) will
re-probe from zero on the next session start.

For task-05 (wave 1), just log a debug line when a `turn_started` arrives for a session
with no prior `observed_detached` — it is the normal case and not worth tracking.

## Tests to write

- **`observed_detached` set on matching event**: inject a tool-call event with
  `IsDetachedBackgroundLaunch()==true` and `Kind==BackgroundWorkKindShell`; verify
  `parkedStateFor(sessionID).observedDetached == true`.

- **Kind filter**: inject a tool-call event with `IsDetachedBackgroundLaunch()==true` but
  `Kind==BackgroundWorkKindSubagent`; verify `observedDetached` stays `false`.

- **`turn_marker` increment**: inject two `EventTypeTurnStarted` events for the same
  session; verify `turnMarker == 2`.

- **Reset on eviction**: verify that after session cleanup, `parkedStateFor` returns nil.

- **AC-69 (seam present)**: the `BackgroundLaunchRecognizer` seam exists — verified by
  the fact that adding a second agent recogniser would only require extending
  `stampBackgroundShellWork` in agentctl, touching no backend code. This is an
  architecture assertion (see task-08 for the AC-35 completeness test).

- **AC-69a (agentID not used)**: confirm the orchestrator check reads only
  `IsDetachedBackgroundLaunch()` and `Kind()`, never `payload.AgentID` or any agent-name
  map. Cover with a test that injects a mock-agent event with Kind==shell and verifies
  `observed_detached` is set (since the mock uses the same normalised predicate as Claude).

## Acceptance criteria closed

- **AC-69**: recogniser seam present in agentctl (existing `stampBackgroundShellWork`);
  addition of a second vendor requires no backend change.
- **AC-69a**: backend reads only the kind-neutral predicate.
- **AC-37 (second GIVEN)**: kind filter prevents subagent launches from setting
  `observed_detached`.
- **AC-41b** (turn_marker increment): `turn_marker` increments on `turn_started` via FIFO
  ordering established in task-01.
