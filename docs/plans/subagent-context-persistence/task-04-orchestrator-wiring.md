---
id: "04-orchestrator-wiring"
title: "Orchestrator frame wiring"
status: pending
wave: 4
depends_on: ["03-service-writer"]
plan: "plan.md"
spec: "../../specs/agents/requirements/subagent-context-persistence.md"
---

> **Amendment 1 update:** the orchestrator must supply the execution identity
> (`agent_execution_id`) alongside the existing frame fields — see the
> "Amendment 1 update" note near the top of `plan.md` for the current wiring
> shape.

# Task 04: Orchestrator frame wiring

Call the writer from the two frame handlers that already parse the subagent
payload, and wire the adapter in `backendapp`.

## Acceptance

1. A `subagent_task` frame on `handleToolCallEvent` records a context with the
   active turn id; a later frame for the same tool call on
   `persistToolUpdateMessage` records again so the upsert's merge path runs. A
   nested frame (`ParentToolCallID` set) is recorded, not dropped. (AC-1, AC-3,
   AC-6)
2. A non-`subagent_task` normalized kind records nothing, and a nil recorder is
   safe — no panic, and the message write still happens. (AC-27)
3. Neither handler exceeds the package's `funlen` limit (80 lines / 50
   statements); the guard and request build live in one
   `recordSubagentContextFromFrame` helper.

## Files likely touched

- `apps/backend/internal/orchestrator/service.go` — new
  `SubagentContextRecorder` interface beside `MessageCreator` (line ~86) and an
  optional `subagentContexts` field on `Service`.
- `apps/backend/internal/orchestrator/event_handlers_streaming.go` — new
  `recordSubagentContextFromFrame` helper; call it from `handleToolCallEvent`
  (~line 312, after the `CreateToolCallMessage` block) and from
  `persistToolUpdateMessage` (~line 565, after `UpdateToolCallMessage`).
- `apps/backend/internal/backendapp/adapters.go` — new `subagentContextAdapter`
  over `*taskservice.Service`, wired where `messageCreatorAdapter` is wired.
- `apps/backend/internal/orchestrator/event_handlers_streaming_subagent_test.go`
  — new.

## Dependencies

Task 03 — the service method is what the interface abstracts.

## Parallelism

`sequential`.

## Inputs

- Plan § *Orchestrator wiring (task 04)* — carries the interface shape, both call
  sites, and the turn-id sourcing rule.
- Spec AC-1, AC-1a, AC-2a, AC-3, AC-6, AC-11, AC-27.
- **Turn id must come from what each call site already resolved**, not from a
  fresh lookup: `handleToolCallEvent` uses `s.getActiveTurnID(payload.SessionID)`
  and `persistToolUpdateMessage` uses its local `turnID`, which is
  `peekActiveTurnID` for terminal frames and `getActiveTurnID` otherwise. A
  terminal frame on a settled turn deliberately resolves to `""`, and AC-2a's
  original-turn preservation lives in the upsert, not here — do not re-derive a
  turn to "fix" an empty one.
- `payload.SessionID` **is** the Kandev task session id, despite the
  `agentSessionID` parameter name downstream: `messageCreatorAdapter` passes the
  same value into `CreateMessageRequest.TaskSessionID`
  (`internal/backendapp/adapters.go:879`).
- Guard on `payload.Data.Normalized.Kind() == streams.ToolKindSubagentTask` and a
  non-nil `Normalized.SubagentTask()`.
- Test patterns: `event_handlers_streaming_test.go:906`
  (`newServiceBackedMessageCreator(repo)`) for the end-to-end
  handler → service → repo case, and `mockMessageCreator` for the unit cases.

## Verification

```
cd apps/backend && gofmt -l internal/orchestrator internal/backendapp && make lint && go test -run 'Subagent' ./internal/orchestrator/... && go test ./internal/orchestrator/... ./internal/backendapp/...
```

## Results

Pending. Before marking the task done, replace this with every exact command
actually run and its outcome/count, generated artifact paths, and cleanup or
teardown evidence. Record security/trust and external side-effect boundaries
when applicable, or explicitly state `None`.
