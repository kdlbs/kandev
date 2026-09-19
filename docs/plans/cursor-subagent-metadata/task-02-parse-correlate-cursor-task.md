---
id: "02-parse-correlate-cursor-task"
title: "Parse and correlate cursor/task onto the subagent payload"
status: done
wave: 2
depends_on: ["01-sdk-extension-seam"]
plan: "plan.md"
spec: "../../specs/agents/requirements/cursor-subagent-metadata.md"
---

# Task 02: Parse and correlate cursor/task onto the subagent payload

Parse `cursor/task`, correlate it to the standard subagent `tool_call` by
`toolCallId`, and merge its metadata (`description`, `prompt`, `model`,
`agentId`) onto the existing `SubagentTaskPayload`. Also map the completion
`rawOutput.isBackground` onto `IsAsync`.

## Acceptance
- A subagent `tool_call` plus a matching `cursor/task` (in either arrival order)
  produces one `SubagentTaskPayload` carrying Cursor's description, prompt,
  model, and agent id.
- An object-shaped `subagentType` is ignored (never coerced to the string
  `SubagentType`); missing fields leave their targets zero-valued.
- An already-set field is not blanked by a later empty Cursor value.
- A `cursor/task` with no matching `tool_call` is dropped at session teardown.
- Completion `rawOutput.isBackground:true` sets `IsAsync`; `false`/absent does
  not.

## Verification
`cd apps/backend && go test ./internal/agentctl/server/adapter/transport/acp/...`

## Files likely touched
- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_cursor.go` (new)
  — parse params, `applyCursorTaskMeta`.
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_cursor.go` (new)
  — store pending Cursor metadata, merge it in either arrival order, and clear
  it on teardown.
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter.go` — set
  the `WithCursorTaskHandler` callback; per-session `map[toolCallId]cursorTaskMeta`;
  clear on teardown.
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_tools.go` —
  merge pending metadata onto the first matching `tool_call` payload.
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_session.go`
  — clear unmatched pending metadata on session reset/teardown.
- `apps/backend/internal/agentctl/server/adapter/transport/acp/subagent.go` —
  extend `cursorSubagentResult` to read `isBackground` → `IsAsync`.
- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_cursor_test.go` (new)
- `apps/backend/internal/agentctl/server/adapter/transport/acp/subagent_test.go` — isBackground + correlation cases.

## Dependencies
Task 01 (needs the `CursorTaskHandler` seam).

## Parallelism
sequential.

## Inputs
- Spec: "API surface" (exact param shape), "Failure modes", "Scenarios".
- Plan: "Area 2", "Area 3".
- Code refs: `subagent.go:224,349,371` (`extractSubagentResult`,
  `cursorSubagentResult`, `applySubagentResult`); `tool_payload.go`
  `SubagentTaskPayload`; `adapter.go:449` connection wiring.
- Wire fixture: `acp-debug/cursor-acp-linger-20260820-145213.jsonl`.

## Output contract
Summary, files changed, tests run + counts, blockers, risks; update this task's
status and `plan.md` Wave 2 checkbox and Verification Results.

## Results
- Added defensive Cursor param parsing for `toolCallId`, `agentId`,
  `description`, `model`, and `prompt`, ignoring object-shaped
  `subagentType`.
- Added per-session correlation state so `cursor/task` and the subagent
  `tool_call` merge into one `SubagentTaskPayload` in either arrival order, and
  unmatched pending state is dropped on teardown.
- Extended Cursor result enrichment so `rawOutput.isBackground:true` sets
  `IsAsync`, which reuses the existing background-chip UI.
- `cd apps/backend && go test ./internal/agentctl/server/adapter/transport/acp/...` passed.
