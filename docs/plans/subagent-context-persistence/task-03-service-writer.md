---
id: "03-service-writer"
title: "Service writer and expvar counters"
status: pending
wave: 3
depends_on: ["02-repository-upsert"]
plan: "plan.md"
spec: "../../specs/agents/requirements/subagent-context-persistence.md"
---

> **Amendment 1 update:** the writer's request/identity-gate shape carries an
> `ExecutionID` and the attempted/anomalous counters aggregate as documented in
> the "Amendment 1 update" note near the top of `plan.md` — read that note for
> the current writer-health invariants before implementing from this file.

# Task 03: Service writer and expvar counters

Add the single service entry point that owns identity gating, value
normalization, terminality, and the five writer-health counters.

## Acceptance

1. `RecordSubagentContext` has **no error return**. A repository failure logs
   `WARN` with the session id and tool call id, increments `failed`, and returns
   normally — so it is structurally impossible for this writer to fail the
   enclosing message write, turn, or agent stream. (AC-27)
2. Normalization is exact: every empty nullable TEXT field becomes NULL, never
   `''`; an unreported `total_tokens` / `tool_use_count` / `duration_ms` becomes
   NULL, never `0`; a reported `tool_use_count` of `0` stays `0`; a negative
   value becomes NULL and increments `anomalous_value` while leaving the frame's
   other fields intact. (AC-7, AC-8, AC-9)
3. `settled_at` is set only from a terminal ACP `tool_status`. `agent_status` is
   stored verbatim and never consulted for terminality — including
   `async_launched` and any value Kandev does not recognise. (AC-10a, AC-11)
4. `expvar` exposes `attempted`, `persisted`, `skipped_no_identity`,
   `anomalous_value`, `failed`. Missing session id, task id, or tool call id
   writes nothing and increments `skipped_no_identity`. (AC-2, AC-26)

## Files likely touched

- `apps/backend/internal/task/service/subagent_context.go` — new:
  `RecordSubagentContextRequest`, `RecordSubagentContext`, `nilIfEmpty`, the
  metric normalizer, and a local `isTerminalToolStatus`.
- `apps/backend/internal/task/service/subagent_context_metrics.go` — new: one
  `expvar.NewMap("subagent_context_total")`.
- `apps/backend/internal/task/service/subagent_context_test.go` — new.

## Dependencies

Task 02 — the repository upsert is the collaborator under test.

## Parallelism

`sequential`.

## Inputs

- Plan § *Service writer and counters (task 03)* — carries the request struct,
  the no-error-return rationale, and the counter names.
- Spec § *Column-level normalization rules*, § *Two statuses, deliberately
  separate*, AC-2, AC-7 through AC-11, AC-26, AC-27.
- `streams.SubagentTaskPayload` at
  `internal/agentctl/types/streams/tool_payload.go` — note `ToolUseCount *int`
  is a pointer *specifically* to carry a reported `0`, and that `DurationMs` /
  `TotalTokens` are `omitempty int64` so a reported `0` is indistinguishable from
  absent for those two. Preserve that asymmetry rather than smoothing it.
- `internal/orchestrator/event_handlers_streaming.go:762` — the peer
  `isTerminalToolStatus`. It is unexported, so copy the set
  (`complete`, `completed`, `success`, `error`, `failed`, `cancelled`), name the
  peer in a comment, and add a test that pins the two lists together so a future
  edit to one fails.
- Counter convention: `internal/office/scheduler/metrics_vars.go` (package-level
  `expvar.NewMap`, counters only) and its test
  `metrics_vars_test.go` for the name-assertion shape.

## Verification

```
cd apps/backend && gofmt -l internal/task/service && make lint && go test -run 'TestRecordSubagentContext|TestSubagentContext' ./internal/task/service/... && go test ./internal/task/service/...
```

## Results

Pending. Before marking the task done, replace this with every exact command
actually run and its outcome/count, generated artifact paths, and cleanup or
teardown evidence. Record security/trust and external side-effect boundaries
when applicable, or explicitly state `None`.
