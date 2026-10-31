---
id: "01-structured-provider-failure-signals"
title: "Classify structured provider failure signals"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001
acceptance_criteria:
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.22
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.32
system_design:
  - ../../specs/platform/system-design/provider-error-recovery-03.md
---

# Task 01: Classify structured provider failure signals

## Summary

Carry an explicitly supplied HTTP error status and opaque provider code on the
provider error projection, and feed the known structured signals to the shared
classifier from every consumer that classifies a provider diagnostic.

## In scope

- `routingerr.Input` gains `ACPCode`, `ErrorKind`, and `ProviderCode`;
  `Classify` returns an already normalized `*routingerr.Error` unchanged.
- `routingerr/structured.go` maps HTTP error statuses, ACP `-32000`, OpenCode
  `ProviderAuthError`, and Claude ACP error kinds; anything else stays on the
  text catalogue.
- `streams.ProviderError` gains `HTTPStatus` and `ProviderCode` and a
  `RoutingInput` helper.
- The ACP projection reads only the scalar `statusCode` and `errorCode` keys
  from `RequestError.Data`.
- ACP, Kanban, Office, retained turn-failure, quota-metadata, and lifecycle
  diagnostic consumers build their classifier input through `RoutingInput`.

## Out of scope

New text rules, retry policy, candidate ordering, response body or header
parsing, and changes to recovery gates.

## Acceptance

1. A Claude ACP failure with message `Internal error` and error kind
   `rate_limit_error` or `overloaded_error`, or with an explicit 429/529 status,
   preserves its allowlisted metadata and classifies as provider capacity.
   Runtime retention separately requires a proven remote response and the
   current-turn/runtime gates from [ACP prompt continuity](../../specs/platform/system-design/acp-prompt-error-continuity.md).
2. Another adapter's kind, a generic wrapper kind, a fractional status, and an
   opaque provider code alone do not change classification.
3. ACP `-32000` classifies as authentication required; `-32603` does not.
4. Metadata is lent only to a diagnostic whose normalized text matches.

## Verification

Current-main integration uses
`TestRetainedACPApplicationErrorPreservesStructuredProviderSignals` with the
existing real SDK error-response fixture. Every remote case retains a safe
projection; the same positive/negative metadata cases still assert capacity
classification independently. Local errors remain excluded by the existing
continuity tests. The original pre-continuity verification below is historical.

- `go test ./internal/agentctl/server/adapter/transport/acp/ -run TestRetainableACPApplicationErrorUsesStructuredProviderSignals`
  fails on the base revision (four structured-only cases are not retained) and
  passes with the change.
- `go test ./internal/agent/runtime/routingerr/ -run 'TestClassifyStructuredProviderSignals|TestClassifyKeepsAlreadyNormalizedError'`.
- Existing `routingerr`, `agentctl/...`, `office/scheduler`, `office/service`,
  and `orchestrator` package tests.

## Files likely touched

- `apps/backend/internal/agent/runtime/routingerr/routingerr.go`
- `apps/backend/internal/agent/runtime/routingerr/structured.go` (new)
- `apps/backend/internal/agent/runtime/routingerr/structured_test.go` (new)
- `apps/backend/internal/agentctl/types/streams/provider_error.go`
- `apps/backend/internal/agentctl/types/streams/provider_error_routing.go` (new)
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_prompt.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_prompt_failure.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_prompt_failure_structured_test.go` (new)
- `apps/backend/internal/agentctl/server/adapter/transport/acp/opencode_stderr.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/provider_error_metadata.go` (new)
- `apps/backend/internal/office/scheduler/routing_lifecycle.go`
- `apps/backend/internal/office/service/failure.go`
- `apps/backend/internal/orchestrator/dynamic_evidence.go`
- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/orchestrator/event_handlers_transient_messages.go`
- `apps/backend/internal/orchestrator/event_handlers_turn_failure.go`
- `docs/specs/platform/requirements/provider-error-recovery.md`
- `docs/specs/platform/system-design/provider-error-recovery-03.md`

## Dependencies

None.

## Inputs

- [Owning requirement](../../specs/platform/requirements/provider-error-recovery.md)
- [Owning design](../../specs/platform/system-design/provider-error-recovery-03.md)
- [Plan](plan.md)

## Risks

Reading a kind outside its adapter namespace, or letting prose relabel an
unmapped status, would misreport the failure; both are covered by negative
cases.

## Parallelism

`sequential`
