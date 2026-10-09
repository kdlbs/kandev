---
status: draft
system: platform
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-001
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-003
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-004
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-007
---

# Durable agent stream processing

This document specifies stream processing details for the [durable delivery design](durable-agent-delivery.md).
The parent design retains identity, admission, executor storage, and rollout ownership.

The [streaming repair package](../../../plans/durable-agent-stream-repair/plan.md) owns the remaining streaming and capacity corrections.
The earlier implementation results do not establish completion of this repair.
Existing requirements remain authoritative; this section specifies their implementation details.

### Journal batching

Normal journal batches contain at most 64 KiB and wait at most 20 ms before starting their commit.
One supported indivisible event above 64 KiB uses a singleton transaction within the 1 MiB event limit.
These are batching bounds, not guarantees about disk completion latency.

### Replay and legacy handling

The WebSocket path and adoption path both replay bounded pages through a captured high-water mark.
The journal bridges events committed between that mark and live attachment.
A missing sequence triggers catch-up or typed recovery, never silent advancement.
A positively identified legacy stream uses the existing projection path without durable inbox lookups.
Missing identity on a v1 stream is a protocol error, not evidence of legacy support.

### Projection and acknowledgment scheduling

A per-stream worker batches at most 64 KiB of compatible output with a maximum 20 ms batching wait.
A larger supported indivisible event uses a singleton batch within the event limit.
The bounded intake budget is 4 MiB; storage work does not hold control-response locks.
Every original event retains its sequence and effect identity.
Tool, permission, terminal, turn, and owner changes flush the preceding batch.
Canonical message changes and projected cursor advancement commit atomically.
Notification follows successful projection and does not depend on HTTP acknowledgment success.

This repair retains the current projection-before-acknowledgment rule.
A cumulative ACK scheduler has at most one in-flight request per stream.
It flushes pending progress after 20 ms or 256 projected events, whichever occurs first.
A terminal boundary requests an immediate ACK without blocking committed output notification.
Failed requests retain the highest pending cursor for bounded retry under the existing recovery window.
Owner replacement cancels the scheduler. Reconnection reconstructs progress from SQL.
The scheduler's stream key identifies durable ordering, not a permanent HTTP client.
Bind each worker to the current authenticated execution and runtime epoch. Replace its client binding when that owner changes.
Cancel the previous worker before granting replacement send authority. Late responses and teardown must not mutate or cancel the replacement worker.
Initialize pending progress from the committed projected cursor, including when no new event arrives.
Compare it with the authenticated journal's acknowledged cursor during attachment and later reconciliation.
Retain projection-before-ACK in this repair; changing to inbox-only acknowledgment requires a separate projector recovery proof.
Retry transient ACK failures with jittered exponential delay from 100 ms to 5 seconds and a 3-second request timeout.
Keep at most one request in flight per stream. Record bounded error categories and surface persistent lag instead of silently retrying every 20 ms.
Owner mismatch stops that worker and returns through authenticated attachment. It never retries indefinitely with an obsolete credential or lease.
ACK recovery is independent of prompt admission, event arrival, terminal arrival, and browser focus.
Projection failure enters typed recovery and stops cursor advancement.
It cannot leave an apparently healthy stream consuming later events behind a permanent gap.

### Capacity and retained identities

Logical accounting includes retained submission payloads and metadata, as well as event records.
Existing journal counters are reconciled before admission after upgrade.
Pruning uses deletion-safe iteration and preserves unacknowledged data.
Only bounded terminal and health metadata can consume reserved space.
If ordinary capacity is exhausted, admission stops and cancellation targets the exact current owner.
Status and Stop remain available even if the event writer cannot commit.
Failure to persist a terminal outcome preserves uncertainty.

### Pressure recovery before exhaustion

The 256 MiB stream limit bounds a temporary retained backlog, not the total transcript or task lifetime.
Keep existing finite defaults while fixing retention. Size future changes from measured serialized event rate and the supported disconnected interval.
Per-stream limits, per-journal limits, physical file size, and host free space remain separate budgets.
A larger per-stream limit increases the worst-case footprint across concurrent sessions; it cannot correct stalled acknowledgments.

Introduce pressure at 75 percent of ordinary retained capacity and an admission/producer guard at 90 percent.
Clear pressure below 60 percent after verified ACK pruning. Apply these bounds to both stream and journal budgets.
Reserve enough capacity for all bounded pending writes, one maximum event, and terminal/health records before admitting additional output.
The guard must engage earlier when those bounds exceed the remaining headroom. Percentages alone are insufficient admission checks.
Existing disk-space checks can engage the same state before the logical limit.

On pressure, immediately reconcile the backend's projected cursor and the journal's acknowledged cursor.
Advance only to a verified contiguous committed cursor with matching owner identity, then prune through the existing journal transaction.
Do not fabricate acknowledgment, clear submissions, or require a new prompt to drain the backlog.
Do not increase limits automatically or roll over an unacknowledged stream to evade its budget.

Use producer backpressure only when the adapter can preserve control responsiveness and bounded pending data.
If it cannot, request cancellation of the exact owned turn before exhausting the reserved capacity.
There is no assumption that arbitrary providers support pause-and-resume of an in-flight RPC.
A failed commit enters a typed delivery-unavailable state and reports out of band if the journal cannot store the notice.
Keep process liveness separate from delivery health. Session focus and stale-execution cleanup must not kill a live harness because its delivery status is unhealthy.
After space returns, resume the existing delivery pump when its state is recoverable. Otherwise preserve uncertainty and use explicit same-conversation continuation.

Report retained/unacknowledged bytes, ACK pending age, last successful ACK, retry/error categories, pressure transitions, and projection lag.
Keep metric labels bounded; session/stream identifiers belong in structured diagnostics only.
Use the existing recovery card for a localized delivery-storage explanation and Retry connection/Stop, with desktop and phone parity.
Raw SQL cursors and internal storage paths are diagnostic details, not required user decisions.

Oversized text must use the existing bounded chunk representation. Add coverage for cumulative tool-output snapshots that can multiply retained bytes.
Do not silently truncate authoritative output. Any representation change requires the existing version and rollback compatibility checks.

Idle rollover changes transport stream identity without creating a new harness conversation.
It requires settled submissions and completed projection/acknowledgment evidence for the old stream.
The old stream is sealed before new admission, and old submission IDs remain non-executable.
Journal and SQL checkpoint changes require restart-reconcilable intent and ownership checks.
A journal already above its submission limit blocks new admission until safe rollover.
Representation changes require a versioned migration and an explicit supported rollback boundary.
An older binary must not read a changed format as an empty or newly dispatchable stream.

### Visible failure and overload

Durable queue overload reconnects through the committed cursor.
Legacy intake uses bounded flow control where control responses remain responsive.
If legacy output cannot be retained, the session exposes incomplete delivery and an uncertain outcome.
Neither mode silently drops output or automatically resends a prompt.
The existing chat recovery surface shows reconnecting or uncertain state on desktop and phone.
Stop remains reachable. Retry connection only queries and reconnects the original work.
Storage failure alone does not authorize context continuation.

The [journal shutdown and recovery package](../../../plans/agentctl-journal-shutdown-recovery/plan.md) owns the October ACK replacement and capacity recurrence repairs.
