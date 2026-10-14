---
id: "02-reconnect-and-usage"
title: "Reconnect and usage identity"
status: done
wave: 2
depends_on:
  - "01-approval-lifecycle"
plan: "plan.md"
requirements:
  - REQ-AGENTS-CODEX-NATIVE-002
  - REQ-AGENTS-CODEX-NATIVE-003
  - REQ-AGENTS-CODEX-NATIVE-004
  - REQ-COSTS-CONVERSATION-USAGE-001
  - REQ-COSTS-CONVERSATION-USAGE-002
  - REQ-COSTS-CONVERSATION-USAGE-004
acceptance_criteria:
  - AC-AGENTS-CODEX-NATIVE-002.1
  - AC-AGENTS-CODEX-NATIVE-002.3
  - AC-AGENTS-CODEX-NATIVE-003.3
  - AC-AGENTS-CODEX-NATIVE-004.2
  - AC-COSTS-CONVERSATION-USAGE-001.2
  - AC-COSTS-CONVERSATION-USAGE-001.4
  - AC-COSTS-CONVERSATION-USAGE-001.5
  - AC-COSTS-CONVERSATION-USAGE-002.4
  - AC-COSTS-CONVERSATION-USAGE-004.3
system_design:
  - ../../specs/agents/system-design/codex-app-server.md
  - ../../specs/costs/system-design/conversation-usage.md
---

# Task 02: Reconnect and usage identity

## Summary

Preserve native conversation identity and usage attribution across reconnect, resume, and delayed events.
Reuse the existing process manager and ledger; fix only gaps that remain after the original implementation.

## In scope

- Test application reconnect with a surviving agentctl/RPC client and fresh-process resume as separate paths.
- Preserve pending request ownership where the same provider request survives. Expire requests from terminated processes explicitly.
- If native-stream reattachment exists, prevent request-ID reuse while previous responses remain possible. Otherwise document that the existing client owns continuity.
- Verify root, child, turn, and item identities remain scoped across replay and forks. Copied history does not produce new ledger entries.
- Capture model/provider attribution at the measurement's originating turn or response, using provider evidence where available.
- Test delayed prior-turn usage after a model switch and late child usage during an unrelated turn.
- Test cumulative resets, duplicate snapshots, missing baselines, response/fallback exclusivity, and interrupted-turn recovery.
- Keep absent model evidence or incomplete measurements visible. Do not invent spend or use the currently selected model for older work.

## Out of scope

New persistent hosts, arbitrary live-thread attachment, rollout-file readers, a second ledger writer, and changes to legacy token arithmetic.

## Acceptance

1. Reconnect does not reinitialize a retained client. Fresh-process resume retains the saved thread and never silently creates a replacement.
2. Replay, forks, and delayed child events cannot duplicate spend or change unrelated active-turn state.
3. A model switch does not reprice prior work. Reset or incomplete counters remain incomplete rather than producing invented deltas.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -race ./pkg/codexappserver ./internal/agentctl/server/adapter/transport/codexappserver ./internal/agentctl/server/instance)
(cd apps/backend && go test ./internal/task/usage ./internal/common/costs)
git diff --check
```

Add or map `TestReconnectPreservesNativeClient`, `TestNewProcessResumesSameThread`, `TestDelayedUsageRetainsTurnModel`, and `TestCounterResetDoesNotCreateSpend`.
Exercise ledger replay with the repository's existing integration fixture. If a repository test requires an external database, record the exact additional command and outcome.
Extend both accounting consumers' tests if their wire contract changes; keep the existing Office and task meanings aligned.

## Files likely touched

- `apps/backend/internal/agentctl/server/adapter/transport/codexappserver/adapter.go`, `activity.go`, `usage.go`, and tests.
- `apps/backend/internal/agentctl/server/instance/manager_native_protocol_test.go` and applicable reconnect ownership code.
- `apps/backend/internal/task/usage/` tests and attribution code if needed.
- Existing native fork and usage bindings; preserve their current ownership boundaries.

## Dependencies

Task 01's pending-request lifecycle.

## Risks

Provider events may not establish the actual model or every response. Preserve that limitation instead of treating configuration as authoritative billing evidence.

## Parallelism

`sequential`

## Inputs

- [Agent design](../../specs/agents/system-design/codex-app-server.md), children, execution, and fork sections.
- [Usage design](../../specs/costs/system-design/conversation-usage.md), attribution and persistence sections.
- [Comparison report](../codex-app-server/research-agent-orchestrator.md), reconnect, usage, and scoped-ID findings.

## Results

Reconnect and fresh-process resume retain the native thread identity through the existing agentctl lifecycle. `TestIsAttachedStaysTrueAcrossAnOverlappingReconnect` covers a live agentctl reconnect. `TestResumeRestoresNativeChildBindingFromThreadHistory` exercises `thread/resume` with the stored native thread ID and rejects a fallback to `thread/start`.

Race tests passed for `pkg/codexappserver`, the native adapter transport, agentctl instance and process handling, and both usage packages. Usage observations retain the model and generation captured for their originating native turn. Tests cover delayed root usage after a model switch, late child usage during another turn, and counter resets without invented spend. No task or Office ledger contract changed.

Validation used fake app-server fixtures. It did not establish that Codex always reports an exact model or complete multi-response turn usage.
