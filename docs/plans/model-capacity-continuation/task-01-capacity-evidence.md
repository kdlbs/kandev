---
id: "01-capacity-evidence"
title: "Attest completed work for live capacity continuation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-TURN-CONTINUITY-003
acceptance_criteria:
  - AC-PLATFORM-TURN-CONTINUITY-003.1
  - AC-PLATFORM-TURN-CONTINUITY-003.6
system_design:
  - ../../specs/platform/system-design/transient-turn-runtime-continuity.md
---

# Task 01: Attest completed work for live capacity continuation

## Summary

Add immutable current-prompt evidence for completed-work continuation on a live conversation.
Prove Codex support with a disposable native trace before its dialect advertises the new capability.
This work order supplies evidence without changing retry admission.

## In scope

- Read backend, agentctl, and mock-agent scoped guidance before edits.
- Add `CapacityContinuationSnapshot` beside the existing continuation contract.
- Track real tool completion, unresolved permissions, and unknown/background outcomes in the ordered ACP worker.
- Collect evidence independently of the experimental transport-loss toggle for supported dialects.
- Copy the snapshot through process outcomes, JSON streams, lifecycle terminal snapshots, and watcher conversion.
- Keep omitted, unknown-version, stale, malformed, duplicate-ID, and overflow evidence conservative.
- Capture sanitized Codex native version, live conversation continuity, and actual tool outcome frames in Results.
- Add isolated mock construction support for Task 02's end-to-end scenarios.

## Out of scope

Retry admission, changes to `ContinuationSafetySnapshot.SafeFor`, new feature toggles, and production support for additional providers.

## Acceptance

1. `TestCodexCapacityContinuationEvidence` proves eligibility after completed read, shell, write, and MCP tools, with pending/failed/unknown negatives.
2. `TestCapacityContinuationSnapshotRemoteRoundTrip` proves immutable identity and omission handling across process, lifecycle, and watcher boundaries.
3. A disposable Codex trace proves a subsequent prompt uses the same initialized process and conversation with completed results in history.

Use both completed and pending tools in one negative case.
Capture evidence before synthetic prompt-end cancellation, not after it.
Keep unaccounted Codex collaboration ineligible.
If native evidence cannot establish the capability, leave production support disabled and report the work order incomplete.

## Verification

Use TDD for protocol evidence and propagation regressions.
The initial positive case must fail because the snapshot is absent, not because a test harness cannot start.
Run from the repository root:

```bash
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/agentctl/types/streams ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/server/process ./internal/agentctl/server/instance ./internal/agent/runtime/lifecycle ./internal/orchestrator/watcher -run 'Capacity|Continuation|PromptFailureDisposition|TurnFailure' -count=1)
git diff --check
```

Native probe procedure:

1. Record `codex-acp --version` for the installed tested artifact.
2. Initialize a disposable ACP conversation using that artifact.
3. Complete an output-only turn and a turn with completed shell/write tools in the disposable workspace.
4. Submit a continuation instruction on the same connection and native session.
5. Verify retained history, process identity, initialization count, tool-status shapes, and no repetition of the completed side effect.
6. Remove the disposable workspace and redact secrets, paths, prompts, and account identifiers from recorded evidence.

The probe need not induce a real capacity event.
Committed fixtures separately prove ordered terminal capacity evidence through the production Codex dialect.
Existing native continuation tooling can supply the raw ACP harness.

## Files likely touched

- `apps/backend/internal/agentctl/types/streams/capacity_continuation.go` and its tests (new).
- `apps/backend/internal/agentctl/types/streams/agent.go`.
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_capacity_continuation.go` and its tests (new).
- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_codex.go`, `dialect_mock.go`, and `dialect.go`.
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_prompt.go`, `adapter_updates.go`, and `adapter_permissions.go`.
- Prompt-local state beside `adapter_continuation.go`.
- Process turn-outcome forwarding and retrieval beside existing continuation propagation.
- `apps/backend/internal/agent/runtime/lifecycle/event_types.go`, `events.go`, and `manager_events.go`.
- `apps/backend/internal/orchestrator/watcher/watcher.go` and conversion tests.
- Scoped agentctl `AGENTS.md` if the new wire invariant needs a local note.

## Dependencies

None.

## Risks

Synthetic completion and missing background outcomes can incorrectly authorize recovery.
Strictly preserve provider-confirmed terminal evidence and current prompt identity.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/platform/requirements/transient-turn-runtime-continuity.md), criteria `.1` and `.6` under requirement `003`.
- [Design](../../specs/platform/system-design/transient-turn-runtime-continuity.md#capacity-continuation-evidence).
- Existing `adapter_continuation.go`, `dialect_codex_capacity_test.go`, `continuation_safety_test.go`, and retained-outcome round-trip tests.

## Results

Native Codex trace passed in a disposable workspace. Tested `@agentclientprotocol/codex-acp` 1.13.1 with Codex CLI/app-server 0.160.0 via the package's documented `CODEX_PATH` override. The nested 0.156.1 binary rejected the profile's selected model before tool use, so the trace used the installed compatible CLI without changing the saved profile.

- One initialize call and one ACP process served an output-only turn, a turn with one completed `execute` and one completed `edit` tool, and a same-session continuation.
- The continuation returned the prior tool result from native history, made no tool calls, and left the unique workspace side effect present exactly once.
- No permission request or background activity occurred. ACP session ID and process identity were unchanged across all prompts.
- Deleted the disposable native thread and workspace; no prompt, account identifier, path, or raw frame was retained.

Protocol evidence and cross-boundary propagation are complete.

- `(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/agentctl/types/streams ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/server/process ./internal/agentctl/server/instance ./internal/agent/runtime/lifecycle ./internal/orchestrator/watcher -run 'Capacity|Continuation|PromptFailureDisposition|TurnFailure' -count=1)`: passed.
- `git diff --check`: passed.
