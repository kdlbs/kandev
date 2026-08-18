---
id: "05-capture-tool-definitions"
title: "Capture tool definitions and estimates"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/mcp-session-observability/spec.md"
---

# Task 05: Capture Tool Definitions and Estimates

## Acceptance

- The current Kandev catalog stores bounded input schemas from the actual
  `tools/list` response.
- Each normal tool has a deterministic `o200k_base` token estimate for its
  complete compact MCP tool JSON.
- The server reports `o200k_base:mcp-tool-json-v1` when estimates are present.
- A schema over 64 KiB is omitted and marked. All stored schemas stay within a
  512 KiB combined limit.
- Historical attempts contain no schemas or token estimates.
- The tokenizer works offline. It does not use a character-count fallback.

## Verification

```bash
cd apps/backend && go test ./internal/agentctl/types/streams ./internal/mcp/server ./internal/mcp/tooltokens ./internal/agent/runtime/lifecycle ./internal/orchestrator && go test ./internal/mcp/tooltokens -run TestKnownO200kVectors -count=1 && make build
```

Write failing tests for the wire contract, limits, history removal, and known
tokenizer vectors before the implementation. Record the release-binary size
before and after the dependency change.

## Files likely touched

- `apps/backend/go.mod`
- `apps/backend/go.sum`
- `apps/backend/internal/agentctl/types/streams/mcp_attachment.go`
- `apps/backend/internal/agentctl/types/streams/mcp_attachment_test.go`
- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/server_test.go`
- `apps/backend/internal/mcp/tooltokens/estimator.go`
- `apps/backend/internal/mcp/tooltokens/estimator_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/mcp_attachment_snapshot_test.go`
- `apps/backend/internal/orchestrator/event_handlers_streaming_test.go`

## Dependencies

None. This task extends the completed Task 01 contract.

## Parallelism

Sequential. Task 06 consumes the new optional fields.

## Inputs

- Spec sections `Kandev tool catalog` and `Failure modes`.
- ADR `2026-08-18-session-mcp-tool-definition-details`.
- Existing `mcp.Tool.MarshalJSON`, `AddAfterListTools`, and catalog bounds.

## Output contract

Report the wire fields, limit behavior, estimate method, dependency version,
binary-size change, tests, blockers, and risks. Update this task and the plan
status in the same session.

## Results

Pending.
