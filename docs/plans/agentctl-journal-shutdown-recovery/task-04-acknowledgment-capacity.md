---
id: "04-acknowledgment-capacity"
title: "Recover acknowledgments and contain delivery pressure"
status: completed
wave: 2
depends_on:
  - "01-journal-shutdown"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-001
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.4
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.4
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.5
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.15
system_design:
  - ../../specs/platform/system-design/durable-agent-stream-processing.md
  - ../../specs/platform/system-design/durable-agent-reattachment.md
---

# Task 04: Recover acknowledgments and contain delivery pressure

## Summary

Keep acknowledgments working after a stream moves to a replacement connection.
Drain verified retained data and contain capacity failures without losing the original conversation or mistaking storage failure for process death.
This work runs after Task 01 and before Task 02 despite its later file number.

## In scope

- Reproduce the retained ACK worker using an obsolete client after replacement with the same stream ID. Start with a failing regression.
- Bind workers to the authenticated execution/runtime epoch; cancel and replace stale bindings without letting old teardown cancel the new worker.
- Recover pending ACK progress from SQL on attachment, restart, and quiet-stream reconciliation. Preserve projection-before-ACK.
- Use bounded exponential retry with jitter and observable failure categories; maintain one in-flight ACK per stream.
- Reconcile a journal behind SQL without another event or prompt. Verify owner, contiguous projection, and journal bounds before pruning.
- Exercise a full journal with acknowledged cursor 1193 and projected/high-water cursors 2074 using synthetic data and reduced quotas.
- Introduce pressure, guarded admission, hysteresis, and reserved control capacity from the owning design.
- Keep ACK, replay, health, and Stop operational at capacity. Backpressure only adapters that can keep control responsive; otherwise cancel the exact owned turn.
- Separate delivery health from process liveness at status, automatic resume, and stale-cleanup boundaries.
- Ensure recovery of delivery pressure does not resend the old prompt or create a new native conversation.
- Add bounded ACK-lag/pressure diagnostics and large tool-output/chunk coverage. Reuse existing event representation and accounting.

## Out of scope

Live database edits, automatic journal deletion, larger limits as the repair, new deployment settings, inbox-only ACK, a generic provider pause API, or unacknowledged stream rollover.
Task 03 owns the rendered delivery-pressure state.

## Acceptance

1. ACK progresses through the current connection after runtime replacement and backend restart, including a quiet stream, with no duplicate effects or stale-owner mutations.
2. Under synthetic sustained output and lost ACKs, memory/storage stay bounded, control stays responsive, and verified pruning restores capacity without another prompt.
3. A live harness with unhealthy delivery is not deleted by focus-triggered recovery. Unrecoverable pressure preserves uncertainty and the original conversation.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -trimpath -race ./internal/agent/runtime/lifecycle ./internal/agent/runtime/agentctl ./internal/agentctl/journal ./internal/agentctl/server/api ./internal/agentctl/server/process ./internal/orchestrator/executor ./internal/orchestrator -count=1)
git diff --check
```

Required regressions include same-stream/client replacement, quiet ACK retry, canceled old worker, late old response, expired lease, and mismatched ownership.
Test crashes between SQL projection, ACK request, journal ACK commit, and response receipt.
Use configurable test quotas rather than hundreds of megabytes of fixtures. Test event, stream, journal, and disk limits independently.
Include slow projection, permanent ACK failure, lost final ACK, reserved-space exhaustion, failed cancellation, and an unaffected sibling session.
Assert logical retained-byte recovery separately from physical compaction. Large cumulative output must remain within existing serialized event bounds.
Task 03 adds desktop/mobile crash-resume-ACK-pressure coverage through the shipped UI.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/durable_delivery_ack.go` and adjacent tests
- `apps/backend/internal/agent/runtime/lifecycle/streams.go`, `stream_event_processor.go`, and `durable_delivery_stream.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go`
- `apps/backend/internal/agent/runtime/agentctl/client_delivery.go` and `client.go`
- `apps/backend/internal/agentctl/journal/` accounting, acknowledgment, and quota tests
- `apps/backend/internal/agentctl/server/process/manager.go` and `delivery_batch.go`
- `apps/backend/internal/agentctl/server/api/` status and durable delivery routes
- `apps/backend/internal/orchestrator/executor/executor_state.go` and `executor_resume.go`
- `apps/backend/internal/orchestrator/task_operations.go` and focused liveness regressions
- Existing bounded metric owners and structured diagnostic sources

## Dependencies

Task 01 supplies safe journal lifetime handling. Task 02 consumes the corrected attachment and pressure-recovery behavior.

## Risks

Backend projection success is not evidence that the remote ACK committed. Reconstruct pending progress after restart.
An old worker may finish after its replacement starts; cursor and ownership checks must survive that race.
Finite storage cannot retain unlimited output during an indefinite outage. Preserve explicit cancellation and uncertain-outcome semantics.
Keep the user-authorized incident repair separate from the permanent product behavior.

## Parallelism

`sequential`

## Inputs

- [Projection and acknowledgment scheduling](../../specs/platform/system-design/durable-agent-stream-processing.md#projection-and-acknowledgment-scheduling).
- [Pressure recovery before exhaustion](../../specs/platform/system-design/durable-agent-stream-processing.md#pressure-recovery-before-exhaustion).
- Existing `durable_delivery_ack_test.go`, journal quota tests, and the earlier [streaming repair package](../durable-agent-stream-repair/plan.md).

## Results

Passed the exact seven-package race command on 2026-10-09. This includes current-client replacement, quiet retry, owner-mismatch cancellation, journal capacity, pending-write resume after ACK, and shutdown regressions. `git diff --check` passed. Browser pressure and recovery controls remain in Task 03.
