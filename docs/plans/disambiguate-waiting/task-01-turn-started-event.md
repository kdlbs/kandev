# Task 01: `turn_started` stream event and `observed_detached` clearing

Spec: §"API surface → Turn-boundary stream event", D3, §N. ACs: AC-41, AC-41a, AC-79.

## Why first

D3 names one turn boundary for the whole feature. Nothing else in this spec can
be tested end-to-end without it, and per the spec's own implementation notes it
touches both processes and neither the probe work nor the attestation work owns
it.

## Backend changes

1. `internal/agentctl/types/streams/agent.go` — add `EventTypeTurnStarted =
   "turn_started"` to the `EventType` constants (currently ends around
   `EventTypeMCPAttachment`, `agent.go:6-80`).
2. New payload type (session ID only, no timestamp — the turn start stays in
   agentctl's own memory per the spec).
3. `.../transport/acp/adapter_prompt.go` — emit `turn_started` at the same point
   `sendPrompt` (`:71-91`) records/stamps the turn's recorded start, on **both**
   the human path and the synthetic `ScheduleWakeup` path (`fireWakeup`,
   `:403`). Do not use `humanPrompt` to gate emission — it must fire on both.
4. `internal/agent/runtime/lifecycle/manager_events.go` — relay `turn_started`
   the same way `EventTypeForegroundIdle` is relayed (`handleAgentEvent`
   switch, `:655-766`): no session-specific side effect needed here beyond the
   existing fall-through to `m.eventPublisher.PublishAgentStreamEvent`, unless
   the recorded-turn-start bookkeeping for the probe (task-03) needs a hook
   here too — confirm when task-03 lands its baseline storage.
5. `internal/orchestrator/event_handlers_streaming.go` — `handleAgentStreamEvent`
   (`:24`) gains a case for `turn_started` that clears `observed_detached` for
   the session. This must run on the same ordered consumer as
   `handleToolCallEvent`/`trackBackgroundToolUpdate` (which set
   `observed_detached` via `registerBackgroundWorkKind`) and
   `publishAgentTurnComplete` — i.e., inline in the same switch/dispatch, not a
   separate goroutine or queue (AC-79).
6. `observed_detached` storage: add alongside the parked-projection state in
   task-05's package (a session-keyed map guarded by the same mutex as the rest
   of `parkedState`), cleared by the `turn_started` handler and set by the
   background-attestation call sites. Do not add a second, competing store.

## Tests (TDD, red first)

- `apps/backend/internal/agentctl/.../transport/acp/adapter_prompt_test.go` (or
  a new `turn_started_test.go` beside it): a human prompt and a synthetic
  wakeup-fired prompt both emit exactly one `turn_started` event carrying the
  session ID and no other fields.
- `internal/orchestrator/event_handlers_streaming_test.go`: `observed_detached`
  is true after a `tool_call`/`tool_update` background attestation, then false
  after a `turn_started` for the same session; and a `turn_started` that
  arrives with no prior attestation is a no-op (no panic, no negative state).
- AC-79 ordering test: emit the attestation frame immediately before
  `publishAgentTurnComplete` on the same stream and assert `observed_detached`
  is already true when the turn-complete handler's projection computation
  runs — this only needs the dispatch order asserted, not the full probe
  (stub `BackgroundProbe` per task-05).
- Existing `agent_test.go`-style completeness check for the agent stream event
  set, if one exists for `EventType`s — otherwise skip; the spec's exhaustive
  check is on the probe's WS **action** switch (task-04), not on stream event
  types.

## Notes

`turn_started` carries `session_id` only, per the spec. It rides the existing
agent stream — no new listener, no new port.
