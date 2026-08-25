---
id: "01-attachment-evidence-contract"
title: "Establish the attachment evidence contract"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/mcp-session-observability.md"
---

# Task 01: Establish the attachment evidence contract

## Acceptance

- One versioned type system represents attachment history, attempts, servers,
  connection IDs, evidence events, test results, and stable reason codes.
- A new attempt supersedes prior display evidence even when it reuses the same
  execution.
- The pure reducer implements the spec's Active, Connected,
  Delivered/unverified, Failed, and Filtered/Unavailable precedence.
- Histories retain at most three attempts and 64 evidence events per attempt.
- URL, stdio target, and error sanitizers cannot retain credentials, URL
  paths/query/fragment, headers, environment, arguments, or raw payloads.
- Typed and JSON-rehydrated metadata produce the same report.

## Verification

- `cd apps/backend && go test ./internal/agentctl/types/streams ./internal/agent/runtime/lifecycle`

Write table-driven RED tests for status precedence, reset-within-execution
supersession, bounds, URL user-info/query stripping, stdio basename reduction,
and malformed metadata before implementing the contract.

## Files likely touched

- `apps/backend/internal/agentctl/types/streams/agent.go`
- `apps/backend/internal/agentctl/types/streams/mcp_attachment.go`
- `apps/backend/internal/agentctl/types/streams/mcp_attachment_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/event_types.go`
- `apps/backend/internal/agent/runtime/lifecycle/event_types_test.go`

## Dependencies

None.

## Parallelism

Sequential foundation. Every later task consumes these event names, reason
codes, bounds, or persisted shapes.

## Inputs

- Spec sections `Evidence model`, `Session, execution, and attempt ownership`,
  and `Release-safe attachment report`
- ADR ownership and privacy decisions
- Existing `SessionModelsSnapshot` typed/JSON rehydration pattern

## Output contract

Report the RED assertions, final wire/persistence shapes, sanitizer examples,
exact test result, files changed, blockers, and risks. Mark this task `done`
and update its plan checkbox in the same conversation.

## Execution record

- RED: the new contract tests failed because the attachment types and snapshot
  loader did not exist; the labeled configuration privacy regression then
  failed by exposing `API_KEY`.
- GREEN: versioned attempt/history/server/evidence types, bounded reduction,
  URL/stdio/error sanitizers, and typed/JSON metadata loading now satisfy the
  focused tests.
- Verification: `cd apps/backend && go test ./internal/agentctl/types/streams
  ./internal/agent/runtime/lifecycle` — passed.
