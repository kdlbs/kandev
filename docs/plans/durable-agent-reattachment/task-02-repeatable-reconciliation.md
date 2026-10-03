---
id: "02-repeatable-reconciliation"
title: "Unify disconnect and later retry under one reconciler"
status: done
wave: 2
depends_on: ["01-detached-journal"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-003
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-004
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-003.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-004.3
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.3
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.4
system_design:
  - ../../specs/platform/system-design/durable-agent-reattachment.md
---

# Task 02: Unify disconnect and later retry under one reconciler

## Summary

Unify disconnect and later retry under one reconciler. Preserve original ownership and the no-resend contract.

## In scope

Replace one-shot disconnect querying with the typed identity-bound operation in the design.
Use three attempts within ten seconds, cancellable waits, and one active cycle per execution.
Route disconnect and RetrySessionDelivery/RecoverAgentPromptStream through it without ACP initialization or prompt dispatch.
Persist initial reconnecting/uncertain phase through an existing task-owned seam; Task 04 completes presentation integration.
Return transport_unavailable if the executor tunnel is absent. Do not implement redial or a long-lived scheduler here.
Honor local runtime epoch retirement while keeping remote identity independent of the local runtime owner.
Record the reusable operation's input/output contract for the contributor's follow-up.

## Out of scope

Other work orders, contributor-owned executor redial, long-horizon scheduling, and detached MCP policy.

## Acceptance

- All named regressions exercise the actual owning boundary and pass.
- No stale owner, unrelated recovery cause, or automatic resend crosses the repaired path.
- Partial failure remains visible, bounded, and restart-reconcilable without deleting retained evidence.

## Tests

TestDeliveryReconcileBoundedWindow and TestDeliveryReconcileRearmedAfterOutage use fake clocks and barrier-controlled peers.
TestDeliveryReconcileSingleFlight races disconnect and user retry; assert one stream reader and zero harness dispatches.
TestDeliveryReconcileRejectsStaleOwner races Stop, runtime retirement, and a successor with delayed status replies.
TestDeliveryReconcileLivePeer reports attached-running without clearing the active-turn admission fence.

## Verification

Run from repository root after implementation. All commands are independently rooted.

```bash
(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle ./internal/agent/runtime/agentctl ./internal/orchestrator -count=1)
make -C apps/backend lint
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/durable_delivery_stream.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_events.go`
- `apps/backend/internal/orchestrator/session_launch.go`

Add the named regression files beside the owning source packages.

## Dependencies

Task 01: Make detached durable output independent of the notification queue.

## Risks

Concurrent old events and new ownership can race settlement or cleanup. Use immutable identity and compare-and-set at the mutation boundary.

## Parallelism

`sequential`

## Inputs

- [Manifest](plan.md).
- [Requirements](../../specs/platform/requirements/durable-agent-delivery.md).
- [Design](../../specs/platform/system-design/durable-agent-reattachment.md).

## Results

Implemented one lifecycle reconciler for stream-disconnect and explicit retry. It binds session, execution owner, incarnation, harness generation, stream, submission, startup/prompt generation, and local runtime epoch. It queries current authenticated status and durable evidence up to three times inside a ten-second deadline, with cancellable one- and two-second waits and three-second per-attempt caps. Duplicate callers join one cycle; later calls can start a new cycle. Missing client/tunnel is returned as `transport_unavailable`. Reattachment only opens the updates stream and never calls ACP initialize/load/new or dispatches a prompt.

The execution phase callback records reconnecting/uncertain against the exact active submission and preserves unrelated failure causes. A live socket retains the in-flight turn fence. Uncertain disconnects no longer publish an agent terminal failure or close the orchestrator's durable turn; the existing uncertain error still returns to the caller. Stop, removal, stream-manager shutdown, local runtime retirement, and successor identity checks cancel or reject stale work. Terminal evidence remains unsettled until Task 03 supplies exact durable block settlement.

Validation:

- Passed: `(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle ./internal/orchestrator -run "^(TestDeliveryReconcile.*|TestUncertainDeliveryDisconnectDoesNotCompleteExecutionTurn|TestUncertainPromptErrorLeavesTurnOpenForReconciliation|TestRetrySessionDelivery.*)$" -count=10 -timeout=120s)`.
- Passed: `(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle -run "^(TestDefaultPrepareScriptKubernetesReusesRetainedPVCWorkspace|TestDefaultPrepareScript_SpritesMaterializesNonEmptyWorkspace|TestLocalPreparer_RejectsRepoBackedNonGitWorkspace)$" -count=1 -timeout=90s)` with a private `/tmp` mount and `TMPDIR=/tmp`; these path-sensitive tests pass when their expected temporary root is preserved.
- `go test -race ./internal/agent/runtime/lifecycle ./internal/agent/runtime/agentctl ./internal/orchestrator -count=1` did not pass as a whole. The lifecycle run used the owned workspace scratch path as `TMPDIR`, which broke the three path-sensitive preparer tests above. The orchestrator run ended with an existing background queue goroutine panicking after its test database closed (`executeQueuedMessageWithReservation` called `promptTask` with a nil executor). The focused affected regressions pass under the race detector. The agentctl package passed in the full run.
- `gofmt` and `git diff --check` passed.

Follow-up validation on 2026-09-28:

- The complete lifecycle race suite passed with `go test -race ./internal/agent/runtime/lifecycle -count=1` when run under a private `/tmp` mount backed by the workspace scratch directory. Keeping `TMPDIR=/tmp` preserved path-sensitive workspace expectations.
- The later five-package race run passed orchestrator, and a focused multi-package race run passed lifecycle, orchestrator, executor, SQLite, and agentctl API tests for delivery reconciliation, settlement, and adoption.
- The earlier orchestrator panic remains recorded as a failure from that one invocation; it did not recur in the later race runs.
