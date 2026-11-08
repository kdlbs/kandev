---
status: draft
system: platform
created: 2026-10-27
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-001
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-007
---

# Durable delivery failure handling

## Boundary

Platform owns delivery health between agentctl and the backend.
Process liveness, delivery health, historical outcome, and new-prompt admission remain separate facts.
This design extends [stream processing](durable-agent-stream-processing.md) and uses [existing recovery convergence](session-recovery-convergence.md).
It does not replace their ownership, terminal, or uncertainty rules.

| Requirement | Design sections |
| --- | --- |
| REQ-PLATFORM-DURABLE-AGENT-DELIVERY-001 | Error classification and producer failure handling |
| REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006 | Independent observation, recovery, and user surface |
| REQ-PLATFORM-DURABLE-AGENT-DELIVERY-007 | Retained peers and verification |

## Confirmed failure

An oversized typed search completion stopped `Manager.forwardUpdates`.
Agentctl cancelled Codex, which recorded an interrupted turn immediately.
The backend stayed at sequence 132 and kept its active execution.
The five-minute watchdog produced a waiting notice without observing delivery health.
The execution-less session sweep did not apply to this retained execution.

User cancellation hours later escalated after ten seconds.
The session became `WAITING_FOR_INPUT`, while the SQL submission remained `dispatching`.
Its next prompt encountered `unresolved_durable_work`.
A status connection and a live process did not establish that event forwarding still worked.

## Error classification

Keep distinct bounded categories for `event_too_large`, `storage_full`, `storage_corrupt`, `storage_unavailable`, and `writer_failed`.
An oversized event returns a distinct typed error before sequence allocation.
It never waits for ACK or enters a storage-capacity retry.
Valid large tool display data uses [bounded preparation](bounded-tool-delivery.md) instead of this failure path.

Actual disk exhaustion retains the existing pressure, reserve, and exact-owner cancellation rules.
A corrupt or closed store remains unavailable. No error path recreates the journal, downgrades to legacy, or clears accepted records.
Cancellation intent remains separate from the event-serialization mutex.

## Producer failure handling

Capture a failure snapshot at the write boundary before cancellation or teardown.
The snapshot contains a closed reason code and the immutable event owner:

- Execution and process generation.
- Session, incarnation, harness generation, and stream.
- Submission and prompt generation, when present.
- A process-local failure revision, event kind, attempted encoded size, and applicable limit.

Do not include output text, prompt bodies, credentials, or arbitrary provider errors.
Publish this snapshot through the existing authenticated `/api/v1/status` response under `delivery_health`.
The optional snapshot extends the existing JSON status object. It does not enter the transcript stream or grant outcome authority.
Return a cached failure snapshot even when journal access fails.
Status and Stop cannot wait on the failed writer or its queue.

Do not return from the sole adapter consumer immediately after the first failed ordinary write.
Enter a bounded cancellation-drain phase and keep consuming enough to release the native prompt and its readers.
Cancel only the captured current submission. Revalidate its ownership before the cancellation call.
A delayed failure cannot cancel a successor.

When the adapter supplies an actual terminal event, try to commit that compact event through the existing reserved-capacity path.
Only a committed actual terminal can settle the journal submission.
A cancellation request, health snapshot, process idle status, or failed write cannot manufacture that terminal.
When terminal persistence also fails, retain uncertainty and the failure snapshot.

Do not publish uncommitted ordinary output or claim complete delivery after losing observations.
Previously accepted records remain intact and replayable.
Stop and shutdown bound the drain by their existing contexts and join its goroutines.
The consumer and writer close only after the owned drain or teardown finishes.

## Independent observation

Attach one lifecycle health monitor to each current durable updates-stream owner in `StreamManager`.
Use its existing owned-goroutine and stop-context patterns.
The monitor runs while the stream is attached, including quiet native activity outside an outstanding prompt call.
It does not depend on browser focus, another output event, or the five-minute stall notice.

Probe the existing status endpoint every two seconds, with one request in flight and a two-second request deadline.
Share or coalesce an existing status probe when that owner already has one.
Do not add per-browser polling or an independent global session registry.
An authenticated failure snapshot enters reconciliation immediately.
With reachable status and available backend storage, persist and publish recovery within ten seconds of the reportable failure.

Capture execution, stream-connection epoch, and prompt identity for each probe.
Before state changes, compare the response's failure identity with current lifecycle and SQL ownership.
Ignore a delayed old-owner or old-prompt response.
Manager, stream, prompt, shutdown, and replacement locks must not remain held over the HTTP request.

A healthy silent result keeps ordinary long-running work active.
A status timeout is transport uncertainty, not proof of process death or journal corruption.
Use existing connection-recovery limits and typed feedback.
One unhealthy execution cannot block another execution's monitor or Stop.

## Recovery and persistence

Reuse `session_recovery_blocks`, delivery submissions, native recovery notices, and existing queue admission.
Create or update an exact `agent_delivery` recovery fence with the existing `unresolved_durable_work` reason.
Retain a bounded diagnostic cause so the UI can distinguish failed delivery from ordinary agent silence.
No new SQL table, session-state enum, or admission lock is required.

Observe health before publishing an apparently idle, prompt-ready session.
A coarse `WAITING_FOR_INPUT` state does not clear the fence.
Both direct and queued prompts retain the fence through cancellation races, browser reload, and backend restart.

If a real terminal commits, replay and project all preceding accepted records before applying its actual outcome.
Use the existing exact inventory and native-readiness proof before resolving the matching fence.
A known outcome cannot be downgraded to unknown.
Independent recovery blocks and active submissions retain their authority.

When no terminal can be persisted, preserve historical uncertainty.
Retry connection queries the same work without dispatch.
Explicit Resume uses the existing authenticated native restoration/retirement rules.
Stop targets the current owned process and retains any unresolved historical outcome.
Only existing verified evidence can restore admission for a distinct new user instruction.
Never resend the interrupted instruction, infer completion from idle status, or reset cursors to skip missing evidence.

A recovered producer must reestablish a usable writer and current ownership before health becomes healthy again.
A capacity read alone cannot erase a sticky writer failure.
Repeated recovery uses the same existing idempotent operations.
This package does not repair historical live sessions by direct database mutation.

## User surface and mobile behavior

Reuse task chat and `SessionRecoveryCard`, together with the existing interrupted-delivery presentation.
Use the current recovery model, copy, and bounded technical-details presentation.
Only the timing and correctness of the existing recovery state change.
Stop remains available when process control is available.
Retry connection remains state-only. Resume appears only under existing eligibility rules.
No workflow transition follows from the health report.

The nearest shipped exemplar is the recovery card's wrapping desktop actions and stacked phone actions.
Keep that composition and the existing footer allocation.
The card retains its `max-h-[50dvh]` internal scroll owner.
Phone/coarse-pointer actions measure at least 44 pixels. Desktop actions retain their ordinary density.
Keep safe-area clearance, keyboard access, wrapped details, and composer draft preservation.
A new drawer, banner, or application-wide blocking screen is unnecessary.

No new markup, recovery control, label, or layout is planned.
The [package](../../../plans/resilient-agent-output-delivery/plan.md) specifies regression coverage for the existing states.
Public recovery guidance explains that a live process can still require delivery recovery.
It never promises a known tool outcome after failed persistence.

## Retained peers and diagnostics

New peers provide the correlated snapshot.
For an older retained peer, combine its authenticated `DeliveryHealth` with a fresh matching delivery descriptor.
Change state only when current ownership and submission identity can be established.
Otherwise expose the existing typed recovery refusal. Do not invent missing evidence or replace the retained process.
The ten-second guarantee applies to reachable compatible peers that expose correlatable failure evidence.

Reuse the existing `agent_delivery_journal_errors_total` family with closed error categories.
Logs carry exact identities, event kind, size, limit, and cause without content.
Metrics never use session, submission, stream, path, or provider text as labels.

## Verification

Use the real process manager with a failure-injected ordinary append and a later real cancellation/terminal event.
Prove bounded drain, journal settlement when storage permits, and uncertainty when terminal persistence fails.
Use fake time for the lifecycle monitor, without another event or browser request.

Exercise healthy silence, old-owner replies, successor prompts, unavailable status, concurrent Stop, and sibling isolation.
Cross restart, projection, ACK, fence resolution, and native retirement boundaries without replaying an instruction.
Desktop and phone E2E must prove visible recovery, draft retention, reachable actions, and a distinct later instruction after valid recovery.
Exact tests and commands belong to the [work orders](../../../plans/resilient-agent-output-delivery/plan.md).

## Decision

[All-tool delivery boundary](../../../decisions/2026-10-27-all-tool-delivery-boundary.md).
