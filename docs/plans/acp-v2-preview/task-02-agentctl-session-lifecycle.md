---
id: 02-agentctl-session-lifecycle
title: Integrate v2 session and prompt handling
status: done
wave: 2
depends_on:
  - 01-sdk-and-debugger
plan: plan.md
requirements:
  - REQ-AGENTS-ACP-V2-002
  - REQ-AGENTS-ACP-V2-003
  - REQ-AGENTS-ACP-V2-004
acceptance_criteria:
  - AC-AGENTS-ACP-V2-002.1
  - AC-AGENTS-ACP-V2-002.2
  - AC-AGENTS-ACP-V2-002.3
  - AC-AGENTS-ACP-V2-002.4
  - AC-AGENTS-ACP-V2-003.1
  - AC-AGENTS-ACP-V2-004.2
system_design:
  - ../../specs/agents/system-design/acp-v2-preview.md
---

# Integrate v2 session and prompt handling

Use the new wire client within agentctl. Project ACP v2 into the existing
agent event and session contracts.

## Scope

- Add the v2 adapter to the ACP factory. Wire the SDK through a published
  fork revision for the branch. Use a temporary module override for local
  integration before that revision is available.
- Cover session setup, config options, MCP, prompt submission, ordered
  updates, approved interactions, cancellation, replay, and close.
- Reuse the process manager's delivery and permission boundaries.
  Keep the launch path internal to fixtures until the flag integrates.

The product entry points and retention across a toggle reversal are in order 03.

## Acceptance

1. The scripted peer exercises prompt acknowledgement before and after running,
   terminal failure, cancellation, and transport loss. Each turn settles once.
2. V2 setup passes advertised MCP transports and config options. Permission
   and elicitation replies use the originating session and request.
3. Replay restores the saved session, suppresses duplicate output, and
   preserves its ID on failure.

## Verification

From `kdlbs-kandev/apps/backend`:

```bash
GOWORK=off go test -trimpath ./internal/agentctl/server/adapter/... ./internal/agentctl/server/process ./internal/agentctl/server/api -count=1
GOWORK=off go test -trimpath ./internal/task/repository/sqlite ./internal/orchestrator \
  -run 'Test(CanonicalMessageSnapshotsReplaceAndClear|StreamingMessagePublishesCanonicalClearAndPreservesLegacyAppend)$' -count=1
GOWORK=off go test -trimpath -race ./internal/agentctl/server/adapter/transport/acpv2 -count=10
GOWORK=off go test -trimpath -race ./internal/agent/runtime/lifecycle ./internal/agentctl/server/api \
  -run 'Test(CanonicalMessageReplacementPreservesPromptGeneration|WSInitializeReportsProtocolFromInitializedAdapter|NativeUserInputBridgeKeepsIdentityOnRetryAndCancel)$' -count=10
```

## Files and dependencies

The wire API comes from the completed `acp-go-sdk` work package at
`docs/plans/acp-v2-preview/plan.md` in that repository. Use the tested SDK
revision and the `experimental/v2` package from its handoff.

Likely paths include `apps/backend/internal/agentctl/server/adapter/factory.go`,
`apps/backend/internal/agentctl/server/adapter/transport/acpv2/`, and
`apps/backend/internal/agentctl/server/process/`. Use the shared
`AgentEvent`, `AgentAdapter`, and `SessionRestoreCapabilities` contracts.
Guard the submission generation during delayed prompt and permission replies.

## Results

- Kandev pins the published SDK revision
  `v0.13.6-0.20261123142417-27925bf88fae`. The ACP v2 adapter and native
  question bridge race tests passed with this dependency and `GOWORK=off`.
- Wire fixtures cover acknowledgement ordering, cancellation, accepted
  failure, uncertain acceptance, request-scoped replies, typed configuration,
  HTTP MCP delivery evidence, child IDs, and replay.
- Wire tests verify that partial updates preserve tool file locations and
  subagent details. Explicit null values clear those fields.
- The ACP v2 race suite passed ten consecutive runs. A full event queue leaves
  cancellation available and preserves event order. An idle update without a
  stop reason records an unknown outcome and keeps the runtime available.
- The lifecycle and adapter race tests passed three runs for late setup
  snapshots and confirmed configuration responses. The regression first
  reproduced a default snapshot published after the execution became ready.
- Chromium preserved the two configured model labels throughout three
  multi-session restart runs. The same scenario reproduced the late
  provider default before the publication fix.
- The adapter, process manager, and agentctl API suites passed.
- The focused SQLite and orchestrator tests passed. They cover whole-message
  replacement, notification of an empty replacement, and legacy append events.
- The lifecycle and API regressions passed ten runs with the race detector.
  Canonical message replacements keep their prompt generation. Initialize
  returns the protocol version from the adapter that supplies agent info.
  Native questions keep their identity across a reconnect and cancel only the
  bundle for the originating request.
