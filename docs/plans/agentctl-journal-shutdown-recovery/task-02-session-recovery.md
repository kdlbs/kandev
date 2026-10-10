---
id: "02-session-recovery"
title: "Recover interrupted sessions after execution removal"
status: completed
wave: 3
depends_on:
  - "01-journal-shutdown"
  - "04-acknowledgment-capacity"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-003
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
  - REQ-PLATFORM-AGENT-RUNTIME-AVAILABILITY-003
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-003.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-003.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.3
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.4
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.5
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.6
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.11
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.12
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.14
  - AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-003.1
  - AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-003.3
  - AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-003.5
  - AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-003.6
system_design:
  - ../../specs/platform/system-design/durable-agent-reattachment.md
  - ../../specs/platform/system-design/agent-runtime-availability.md
---

# Task 02: Recover interrupted sessions after execution removal

## Summary

Keep recovery possible after the live execution disappears.
Return a typed retry result and authorize explicit native continuation only through the existing ownership and admission guards.

## In scope

- Reproduce initial prompt dispatch followed by runtime loss and execution removal with real SQL/journal stores.
- Trace the incident's `agent delivery submission not found` path. Persist initial canonical identity before dispatch and reuse it for loss events.
- Preserve existing incomplete records as visible blocked recovery. Reconstruct only uniquely proven associations.
- Resolve retry from durable identity before requiring an in-memory execution. Reopen retained journals only through authenticated exclusive ownership.
- Project retained output and terminal outcomes once. Preserve other recovery blocks, queue claims, and old uncertain records.
- Return bounded outcome, reason, allowed actions, and recovery revision through `session.recover`.
- Extend explicit resume with interruption acknowledgment, observed identity/revision, a new instruction, and idempotency key.
- Recheck process death and independent guards at admission. Defer automatic initial prompts and submit only the new instruction.
- Preserve native identity and worktree. Missing native state remains the existing explicit history-continuation choice.
- Close the observed native-load/admission gap: use a new delivery generation with the same native conversation, then retire only the identified interrupted submission while retaining its uncertain outcome.
- Ensure retired submissions do not keep recovery descriptors or prompt admission unresolved. Cover interrupted recovery at each persistence boundary.
- Add bounded batch recovery with per-session authorization, revisions, idempotency, progress, and independent failures. Never create replacement sessions or native conversations.
- Keep Stop honest when cancellation cannot be confirmed, including while another retry is pending.

## Out of scope

UI implementation, automatic old-prompt retry, fabricated completions, remote replacement policy, and broad manual block clearing.

## Acceptance

1. Retry after execution removal either replays verified retained evidence or returns a specific blocked/uncertain result. It never dispatches a prompt.
2. Explicit eligible continuation sends one new instruction, preserves prior uncertainty, and rejects duplicate, stale, unauthorized, or ownership-ambiguous requests.
3. Initial and subsequent prompts keep canonical identity across failure. Reload/restart, mixed live/dead sessions, Office, and queue paths retain their guards.
4. A batch preserves every eligible session and native conversation ID, accepts one continuation per item, and reports blocked items independently.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -trimpath -race ./internal/agent/runtime/lifecycle ./internal/orchestrator ./internal/orchestrator/handlers ./internal/orchestrator/executor ./internal/task/repository/sqlite -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -trimpath -race ./internal/persistence/storeconformance -count=1)
git diff --check
```

Add proposed regressions to new bounded files where existing test files exceed limits:
`TestRetrySessionDeliveryWithoutExecutionReplaysRetainedTerminal`,
`TestRetrySessionDeliveryMissingCanonicalSubmissionStaysVisible`,
`TestInitialSubmissionIdentitySurvivesRuntimeLoss`, and
`TestInterruptedResumeDispatchesOnlyNewInstructionOnce`.
Cover locked/corrupt journals, mismatched generations, late old callbacks, missing native tokens, and mixed live/dead owners.
Reproduce successful native restore followed by `durable delivery requires reconciliation before a new prompt`.
Prove both identity preservation and accepted continuation, including batch retry after partial success and process restart.
If storage methods or schema change, run the same PostgreSQL tests with `KANDEV_TEST_POSTGRES_DSN` configured.
A missing PostgreSQL fixture is a recorded release gate, not a passing result.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/runtime_replacement.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go`
- `apps/backend/internal/agent/runtime/lifecycle/delivery_reconciliation.go`
- `apps/backend/internal/orchestrator/session_launch.go`
- `apps/backend/internal/orchestrator/agent_delivery_submission.go`
- `apps/backend/internal/orchestrator/agent_delivery_recovery_state.go`
- `apps/backend/internal/orchestrator/agent_delivery_settlement.go`
- `apps/backend/internal/orchestrator/handlers/handlers.go`
- `apps/backend/internal/orchestrator/executor/executor_delivery_identity.go`
- Existing task repository recovery and submission contracts, with adjacent regression tests

## Dependencies

Task 01 provides safe journal lifetime behavior.
Task 04 provides current-owner ACK scheduling and safe pressure recovery before native continuation is admitted.

## Risks

Missing in-memory state is not process-death evidence. Older records can lack enough identity for safe reconstruction.
Never use the generic recovery resolver to erase unrelated causes or release the interrupted queue claim for redispatch.

## Parallelism

`sequential`

## Inputs

- [Design: recovery without an in-memory execution](../../specs/platform/system-design/durable-agent-reattachment.md#recovery-without-an-in-memory-execution).
- [Runtime replacement](../../specs/platform/system-design/agent-runtime-availability.md#session-recovery).
- Existing `session_delivery_retry_test.go`, `agent_delivery_recovery_state_test.go`, and `runtime_replacement_test.go`.

## Results

Completed on 2026-10-09.

- The exact five-package race command passed with SQLite and with the isolated PostgreSQL fixture. The PostgreSQL run used `-timeout 30m`; its SQLite repository suite took 988.887 seconds under concurrent runner load.
- `go run ./cmd/sqlguard ./internal` passed.
- `go test -trimpath -race ./internal/persistence/storeconformance -count=1` passed with `KANDEV_TEST_POSTGRES_DSN` configured (148.855 seconds in the final run).
- The normal commit hook found a direct lifecycle import in the new retry handler. Recovery types and errors now pass through the established public runtime aliases. Full architecture lint and focused runtime/orchestrator race checks passed after that correction.
- `golangci-lint run ./... --new-from-rev=3fed5570cec533f468c25ed03c84967bdc972588 --timeout 5m` passed.
- Real journal and repository tests cover retained terminal replay, duplicate projection, canonical initial submission identity, missing execution, locked storage, ownership refusal, and late callbacks.
- Explicit continuation preserves the Kandev session, native conversation, workspace, old unknown submission, and queue claim. Its atomic commit creates only the acknowledged new instruction. Duplicate and batch retries do not repeat an accepted instruction.
- The canonical chat message for that instruction uses a stable snapshot-derived ID and the accepted turn ID. Retained output is projected once without lifecycle callbacks; Retry also settles a previously projected terminal when its recovery block is already resolved.

Linux process-termination proof was exercised locally. Darwin and Windows host behavior and real provider CLI continuation remain platform checks; unsupported process proof fails closed.

PR review follow-up:

- Prepared continuation checkpoints now retry through the current admission gates. Accepted checkpoints can finish failed bookkeeping from the canonical acceptance message without another prompt. Missing acceptance evidence remains blocked. Focused race regressions passed.
- The runtime accepts an orchestrator-admitted first instruction only when its canonical identity, owner, generation, dispatch attempt, and payload match, and the execution has not dispatched a prompt. Launch-handoff and admission-callback regressions passed; duplicate dispatch and foreign ownership remain blocked.
- Three desktop CI scenarios and the phone Configuration Chat scenario reproduced the initial-admission failure before the fix. All four passed after the fix through the managed browser runner.
- The final exact five-package SQLite race run passed: lifecycle 133.808 seconds, orchestrator 183.684 seconds, handlers 5.746 seconds, executor 10.428 seconds, and repository results in the retained test log. SQL guard, SQLite store conformance, and changed-scope Go lint passed. Storage contracts did not change in this review follow-up; the earlier isolated PostgreSQL results remain the package evidence.

## Follow-up package

[Missing delivery record recovery Task 02](../durable-agent-record-recovery/task-02-atomic-reconstruction.md)
builds on this recovery and continuation flow to restore uniquely verified
retained submissions whose canonical backend association is missing. The
dependency work-order results above remain unchanged.
