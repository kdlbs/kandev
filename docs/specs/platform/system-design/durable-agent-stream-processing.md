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
