---
id: "01-dispatch"
title: "Classify rejection and fence dispatch"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-UNDELIVERED-PROMPT-001
  - REQ-PLATFORM-DELIVERY-RETENTION-001
acceptance_criteria:
  - AC-PLATFORM-UNDELIVERED-PROMPT-001.1
  - AC-PLATFORM-UNDELIVERED-PROMPT-001.3
  - AC-PLATFORM-DELIVERY-RETENTION-001.3
system_design:
  - ../../specs/platform/system-design/undelivered-prompt-recovery.md
---

# Task 01: Classify rejection and fence dispatch

## Summary and scope

Carry typed non-admission evidence through the executor and runtime. Settle only
proven rejection. Fence all prompt sources before preparation and dispatch.
Cover initial prompts, ordinary sends, queue dispatch, automatic retry, and steering.
Include settled unsent payloads in the existing bounded retention sweep.

## Out of scope

Provider connectivity repair, generic force-clear controls, and Office scheduling.

## Acceptance

- Proven rejection settles failed with no unknown block. Lost responses remain uncertain.
- Blocked sources create no new submission and preserve the original block binding.
- A racing accepted or terminal record cannot be overwritten.

## Verification

Run from repository root. Write failing regression tests before implementation.

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/orchestrator ./internal/agent/runtime/lifecycle ./internal/agent/runtime/agentctl ./internal/task/repository/sqlite -run 'Test(UndeliveredPromptDispatch|BlockedPromptAdmission|PrepareAgentDeliverySubmission|NativeResume|RecoverSessionUnknownPrompt|UnknownQueueDispatchOutcome|UnknownSendNowOutcome|SendPrompt_RetriesUntilUpdateStreamReconnects)' -count=1)
```

Run `bash scripts/lint-go-changed` with the exact changed Go paths after tests.
For repository changes, repeat the relevant tests with `KANDEV_TEST_POSTGRES_DSN`
pointing to a disposable database. Record unavailable infrastructure as unverified.

## Files likely touched

- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/executor/executor_interaction.go`
- `apps/backend/internal/orchestrator/agent_delivery_settlement_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/session.go`

## Dependencies

None.

## Risks

Lost replies, delayed admission, replaced journals, partial commits, and stale owners.

## Parallelism

Sequential. No delegation is authorized.

## Inputs

Read the linked requirement and design, scoped AGENTS.md, and existing native
Resume tests. Read the relevant existing fixture before adding another fixture.

## Results

Implemented after RED tests reproduced duplicate admission, incorrect unknown
classification, lost-response replay, reconnect proof loss, and stale-owner
settlement. The transport marks only failures before a write as not sent.
Settlement checks the captured owner and submission identity. The final admission
check scans all open unknown blocks and preserves their original binding.
Peer replies without a durable non-admission receipt remain uncertain.

Rebased focused orchestrator race tests passed on 2026-11-15, including queue/send-now recovery
and capability-refresh regressions. Runtime regressions include both a lost reply
that must not retry and a reconnect failure that retains non-delivery evidence.
Client stream regressions, SQL guard, and store conformance passed. Final runtime
checks are recorded in the plan.

PR review exposed initial-prompt settlement and payload-retention gaps. New RED
regressions reproduced both before the follow-up implementation. Runtime now keeps
the initial reservation through dispatch, settles before the failure callback, and
preserves proof when the client disappears. Retention recognizes the local
non-delivery receipt while keeping grace, budget, identity, terminal-evidence, and
recovery-block guards. Focused race tests passed for the runtime, SQLite, and
orchestrator. SQL guard and store conformance passed. PostgreSQL was unavailable
locally; its cases remain enabled for an environment with a disposable database.

The follow-up command was:

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/agent/runtime/lifecycle ./internal/task/repository/sqlite ./internal/orchestrator -run 'Test(Undelivered|InitialPrompt|InitialAdmissionPersistence|PreparedDeliveryAdmission|UnsentPromptRetention|DeliveryRetention(Prompt|PartialTerminal|WorkflowOutcome|IndependentBlocks|Integration|ProtectedSweepRepository)|BlockedPromptAdmission)' -count=1)
```
