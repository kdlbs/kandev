---
id: "01-normalize-capability"
title: "Normalize feature evidence and capability projection"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-NATIVE-SUGGEST-001
  - REQ-AGENTS-NATIVE-SUGGEST-002
acceptance_criteria:
  - AC-AGENTS-NATIVE-SUGGEST-001.1
  - AC-AGENTS-NATIVE-SUGGEST-001.2
  - AC-AGENTS-NATIVE-SUGGEST-001.3
  - AC-AGENTS-NATIVE-SUGGEST-001.4
  - AC-AGENTS-NATIVE-SUGGEST-001.5
  - AC-AGENTS-NATIVE-SUGGEST-001.6
  - AC-AGENTS-NATIVE-SUGGEST-002.1
  - AC-AGENTS-NATIVE-SUGGEST-002.2
  - AC-AGENTS-NATIVE-SUGGEST-002.3
  - AC-AGENTS-NATIVE-SUGGEST-002.5
system_design:
  - ../../specs/agents/system-design/native-prompt-suggestion-capability.md
---

# Task 01: Normalize feature evidence and capability projection

## Summary

Replace namespace detection with the feature-evidence rule in the Agents design.
Carry independent generic support and effective enablement through existing events.
Preserve verified pinned Claude support and older effective-value consumers.

## In scope

- Pure Claude feature detection, metadata construction, and notification decoding
  in the planned `dialect_claude_prompt_suggestions.go`.
- Generic cached support in `Adapter`, replacing `claudeCodeAgent`.
  Keep the existing prompt-queueing recognizer unchanged.
- Exact legacy tuple and the explicitly new vendor extension.
  Present false or malformed declarations override legacy matching.
- Optional support-only field in stream, lifecycle, broadcast, and TypeScript DTOs.
  Preserve the existing effective field and source marker.
- Mock initialize support independent of queueing. Add a fixture-only
  `KANDEV_MOCK_AGENT_PROMPT_SUGGESTIONS` control, default true, with literal false
  advertising unsupported. Do not change runtime profiles or release flags.
- Update test fixtures that currently toggle queueing to imply suggestion support.

## Out of scope

Session-transition changes, composer interaction, new providers, version bumps,
new settings, upstream publication, and protocol registries.

## Acceptance

1. Behavioral RED rejects empty and queue-only namespaces for arbitrary bridges.
   GREEN covers explicit true/false, malformed values, unrelated metadata,
   arbitrary configured/reported names, unsupported Claude models, and exact
   legacy identity/version boundaries.
2. Support remains true with preference off while effective enablement is false.
   Both fields survive all projection hops. Absent raw support retains older
   effective-field behavior.
3. The mock earns support through an explicit declaration independent of queueing.
   Existing origin tests retain their assertions after helper migration.
   No provider-specific decision appears outside adapter integration.

## Verification

Run the RED command after adding the smallest failing evidence table.
Then run the complete block after the implementation.
Initially exercise the existing namespace helper for the negative cases.
After behavioral RED, migrate those cases to the complete initialize-response normalizer.

```bash
(cd apps/backend && go test -trimpath -tags fts5 -p=1 -count=1 ./internal/agentctl/server/adapter/transport/acp -run 'NativePromptSuggestionSupport')
(cd apps/backend && go test -trimpath -tags fts5 -p=1 -count=1 ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/types/streams ./internal/agent/runtime/lifecycle ./internal/orchestrator ./cmd/mock-agent -run 'PromptSuggestion|AgentAdvertisesPromptQueueing|DelayedSuggestionAfterNewerTurn|HandedOffPromptMakesSuccessorOriginAmbiguous')
```

Planned new tests: `TestNativePromptSuggestionSupport`,
`TestPromptSuggestionCapabilitiesJSON`, and
`TestPromptSuggestionCapabilitiesForwarded`.
Extend existing `prompt_suggestion_test.go` and mock initialize tests.
Assert JSON false versus omission explicitly; compile-only success is insufficient.

After Task 01, Task 02 and Task 03 own their new integration and browser checks.
Do not mark the plan implemented based on these unit checks.

## Files likely touched

- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_prompt_suggestion.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_claude_prompt_suggestions.go` (new)
- `apps/backend/internal/agentctl/server/adapter/transport/acp/prompt_suggestion_test.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/prompt_suggestion_origin_test.go` (helper migration only)
- `apps/backend/internal/agentctl/types/streams/agent.go` and focused serialization test
- `apps/backend/internal/agent/runtime/lifecycle/event_types.go`, `events.go`, and focused projection test
- `apps/backend/internal/agent/runtime/lifecycle/prompt_suggestions.go` (generic contract comment only)
- `apps/backend/internal/orchestrator/event_handlers_streaming.go` and capability projection test
- `apps/backend/cmd/mock-agent/main.go`, `mock_agent_test.go`, and suggestion tests
- `apps/web/lib/types/session-runtime-payloads.ts`
- Existing suggestion fixture declarations located by `rg`

## Dependencies

None. Read the ADR and the complete evidence/compatibility rule before implementation.

## Risks

A boolean coercion, overly broad version comparison, or fallback after an explicit
denial would reintroduce incorrect eligibility. Updating the mock through an
agent-name exception would fail to exercise the real boundary.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/native-prompt-suggestion-capability.md)
- [Design](../../specs/agents/system-design/native-prompt-suggestion-capability.md)
- [Decision](../../decisions/2026-10-28-native-prompt-suggestion-capability.md)
- Backend and agentctl `AGENTS.md`; TDD backend test reference
- Current suggestion, queueing, serialization, and capability forwarding tests

## Results

Implemented. `dialect_claude_prompt_suggestions.go` owns pure Claude evidence,
session metadata construction, and `_claude/sdkMessage` decoding.
`Adapter.promptSuggestionSupport` caches support-only evidence;
`supports_prompt_suggestions` keeps effective enablement, and the optional
`native_prompt_suggestions_supported` field travels through stream, lifecycle,
orchestrator, browser, and mock payloads. The mock advertises the explicit
suggestion declaration independently of prompt queueing.

Verification on 2026-10-28 from repository root:

- `go test -trimpath -tags fts5 -p=1 -count=1 ./internal/agentctl/server/adapter/transport/acp -run 'NativePromptSuggestionSupport'` passed.
- The documented multi-package focused block passed for ACP, streams, lifecycle,
  orchestrator, and mock-agent.
- `go build ./...` passed.
- `bash scripts/lint-go-changed <changed-go-files>` reported 0 issues.

