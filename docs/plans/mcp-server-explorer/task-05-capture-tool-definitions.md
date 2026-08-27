---
id: "05-capture-tool-definitions"
title: "Capture tool definitions and estimates"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/mcp-session-observability.md"
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

Implemented the current-attempt tool-definition projection. Each tool can now
carry a complete valid `input_schema`, an `input_schema_truncated` marker, and
an `estimated_tokens` value. The Kandev server reports
`o200k_base:mcp-tool-json-v1` when estimates are available. Superseded attempts
remove the schemas, estimates, estimator name, and catalog entries while they
retain the tool count.

The projection keeps the existing 128-tool and 1,024-byte description limits.
It omits a complete schema when it exceeds 64 KiB or when storing it would
exceed the 512 KiB combined limit. It never stores partial or invalid JSON.
Token estimates use the complete compact JSON produced by
`mcp.Tool.MarshalJSON` before the bounded projection removes any fields.

Added `github.com/tiktoken-go/tokenizer` v0.8.1. It embeds the `o200k_base`
vocabulary and works without a network connection. The first focused RED run
failed nine assertions across wire retention, schema limits, MCP observation,
and known tokenizer vectors. The GREEN run passed 322 tests in the three
focused packages.

Release-size evidence from `make build`:

- `bin/kandev`: 61,703,688 bytes before and 74,025,960 bytes after. The raw
  increase is 12,322,272 bytes.
- `bin/agentctl`: 31,918,536 bytes before and 43,769,224 bytes after. The raw
  increase is 11,850,688 bytes.
- Comparable gzip output increased by 3,035,259 bytes for `kandev` and
  2,925,001 bytes for `agentctl`.

The compressed increase is close to the required vocabulary size. A smaller
alternative would download data at runtime, use an inexact heuristic, or add a
shared external-data contract to every launcher. The implementation keeps the
reviewed exact offline behavior.

Verification passed from `apps/backend`:

```text
go test ./internal/agentctl/types/streams ./internal/mcp/server ./internal/mcp/tooltokens ./internal/agent/runtime/lifecycle ./internal/orchestrator
go test ./internal/mcp/tooltokens -run TestKnownO200kVectors -count=1
make build
```

The build completed with the existing warnings that cross-built Darwin
`agentctl` binaries were not code-signed locally.
