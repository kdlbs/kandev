---
id: "02-normalize-native-events"
title: "Normalize native events and root usage"
status: done
wave: 2
depends_on:
  - "01-qualify-native-transport"
plan: "plan.md"
requirements:
  - REQ-AGENTS-COPILOT-NATIVE-COMPACTION-001
  - REQ-AGENTS-COPILOT-NATIVE-COMPACTION-003
  - REQ-AGENTS-COPILOT-NATIVE-COMPACTION-004
  - REQ-AGENTS-ACP-SESSION-EVENTS-002
acceptance_criteria:
  - AC-AGENTS-COPILOT-NATIVE-COMPACTION-001.1
  - AC-AGENTS-COPILOT-NATIVE-COMPACTION-001.2
  - AC-AGENTS-COPILOT-NATIVE-COMPACTION-001.3
  - AC-AGENTS-COPILOT-NATIVE-COMPACTION-001.4
  - AC-AGENTS-COPILOT-NATIVE-COMPACTION-001.5
  - AC-AGENTS-COPILOT-NATIVE-COMPACTION-001.6
  - AC-AGENTS-COPILOT-NATIVE-COMPACTION-003.1
  - AC-AGENTS-COPILOT-NATIVE-COMPACTION-003.2
  - AC-AGENTS-COPILOT-NATIVE-COMPACTION-003.3
  - AC-AGENTS-COPILOT-NATIVE-COMPACTION-003.4
  - AC-AGENTS-COPILOT-NATIVE-COMPACTION-004.6
  - AC-AGENTS-COPILOT-NATIVE-COMPACTION-004.7
  - AC-AGENTS-ACP-SESSION-EVENTS-002.8
system_design:
  - ../../specs/agents/system-design/copilot-native-compaction.md
  - ../../specs/agents/system-design/acp-session-events.md
---

# Task 02: Normalize native events and root usage

## Summary

Add provider-scoped native subscription and ordered normalization with explicit root attribution.
Emit minimal typed evidence into the existing journal path and use root native usage for the gauge while retaining normal accounting.

## In scope

- Add exact-method SDK opt-in only on enabled Copilot connections.
- Add an optional `copilot-native-compaction.v1` agentctl capability and `CopilotNativeSessionEvents` request/config bit.
- Carry the bit through instance/process/adapter configuration. Keep it off when omitted by an older backend.
- Extend live initialize metadata only; retain utility/inference capabilities and existing Cursor/Devin/Claude behavior.
- Add pure Copilot wire recognizers beside `dialect_copilot.go` and route their immutable work through the shared FIFO.
- Validate provider, method, native binding, absent root `agentId`, types, success, applied outcome, and attempt identity.
- Emit typed completion/reporting observations with bounded fields. No summary, tracing, or billing payload belongs in count evidence.
- Accept valid native observations outside active prompts using native ownership, without inventing prompt/turn/workflow transitions.
- Map start/completion to the existing timeline converter without adding generic timeline counting authority.
- Use root `currentTokens`/`tokenLimit` for context events and keep Copilot inference disabled.
- Preserve ordinary `recordUsageAndMaxSize` accounting before suppressing competing mixed gauge events.
- Serialize creation/load/reset binding, prompt completion barriers, pending-shell flushes, full queues, and shutdown.
- Normalize missing/omitted root completion evidence as partial reporting, never as applied success.

## Out of scope

- Authoritative SQL increments, unbounded buffering, another notification worker, or fallback identity heuristics.
- Parsing raw native files, changing token/cost calculations, relaxing other providers, or rendering coverage controls.

## Acceptance

1. Real transport tests and wire fixtures prove root-only accepted evidence, completion without start, omitted/malformed/failure/non-applied rejection, and exact provider scope.
2. Deterministic barriers prove native/standard ordering and correct ownership across creation, load, reset, and shutdown. Child usage never replaces the root gauge.
3. Snapshot/JSON/journal round trips retain minimal evidence and accounting behavior; standard ACP timeline rows retain their existing non-counting semantics.

## Verification

Run from the repository root after Task 01's immutable SDK dependency resolves.
New tests use the listed prefixes so these commands must discover them.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agentctl/server/acp -run 'Extension|CopilotNative|Connection' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agentctl/server/adapter/transport/acp -run '^TestCopilotNative|^TestUsageTracker|^TestConvertUsageUpdate|^TestHandleACPUpdate' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/agentctl/server/adapter/transport/acp -run '^TestCopilotNative' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agentctl/types/streams ./internal/agentctl/journal ./internal/agentctl/server/process -run 'ContextCompaction|CopilotNative|DeliveryEvent' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agentctl/server/api ./internal/agentctl/server/instance -run 'CopilotNative|Identity|Capability' -count=1)
git diff --check
```

Run `bash scripts/lint-go-changed` from the root with every changed Go path, recording the complete invocation in Results.
Tests must send the raw vendor method through the real SDK, not only call `handleExtensionNotification` directly.
No production subscription is enabled by a backend that lacks Task 03's consumer.

## Files likely touched

- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter.go`, `adapter_helpers.go`, `adapter_updates.go`, and `adapter_prompt_suggestion.go`.
- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_copilot_session_events.go` (new).
- `apps/backend/internal/agentctl/server/adapter/transport/acp/copilot_native_compaction_test.go`, `copilot_native_qualification_test.go`, and `testdata/copilot-native/*` (new).
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_session_events.go` only for reuse needed by native normalization.
- `apps/backend/internal/agentctl/types/streams/agent.go`, `context_compaction_observation.go`, and its tests (new carrier files).
- `apps/backend/internal/agentctl/server/api/identity.go`, `server/config/config.go`, `server/instance/manager.go`, `server/adapter/transport/shared/config.go`, and process adapter-config construction.
- `apps/backend/internal/agent/runtime/agentctl/control.go` for the additive request field.

## Dependencies

Task 01: exact-method SDK support and qualified wire fixtures/pin.

## Risks

Processing native events outside the FIFO can race standard usage or reset binding.
A default-true request bit would break old-backend/new-agentctl compatibility.
Omitted completion data must not be repaired from token differences or a timeline status.

## Parallelism

`sequential`

## Inputs

- [Provider design](../../specs/agents/system-design/copilot-native-compaction.md), transport through root usage.
- [Provider requirements](../../specs/agents/requirements/copilot-native-compaction.md), listed criteria.
- [Standard timeline design](../../specs/agents/system-design/acp-session-events.md) and criterion 002.8.
- Existing `notifWork`, `syncNotifQueueThen`, `convertCompactionUpdate`, `recordUsageAndMaxSize`, and typed stream/journal snapshots.
- Scoped backend/agentctl guidance and ADR 0044.

## Results

Done.

- Copilot connections opt into the exact vendor method through the SDK option only when `CopilotNativeSessionEvents` is set.
- `clientCapabilitiesForAgent` advertises exactly `session.compaction_start`, `session.compaction_complete`, and `session.usage_info` under `_meta["github.com/copilot"].events`; standard `session.compaction` remains independent.
- `copilot-native-compaction.v1` is advertised as an optional agentctl capability and carried through instance/process/adapter configuration.
- `decodedCopilotNativeEvent` validates the envelope, exact method, native session ID, event type, root/child attribution, and omitted data. Child, malformed, failed, non-applied, and incomplete completions add nothing.
- Vendor work is queued on the existing `notifWork` FIFO. Creation/load/resume binding barriers drain a bounded pending buffer in order; stale conversations are dropped.
- Root usage updates the gauge through the existing accounting tracker. Mixed usage keeps accounting but emits no competing gauge while native collection is enabled.
- Valid starts/completions reuse the existing compaction timeline identity. Only the typed `context_compaction_observation` carries counting evidence.
- Transport, fixture, binding, ordering, accounting, and qualification tests pass.

Review remediation added: native root samples no longer mutate prompt token/cost accounting; unqualified runtime versions keep the legacy gauge and report unavailable; unusable root completions and binding-buffer overflow emit sticky partial evidence; creation/load/resume install the binding and drain buffered events in one FIFO barrier. Focused accounting, qualification, malformed-payload, overflow, and ordering tests pass.

Limitations: provider payload and in-flight limits remain provider guarantees, not durable delivery guarantees. Omitted data is reported as partial and never inferred as success.
