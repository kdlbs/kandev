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
  - REQ-PLATFORM-DURABLE-AGENT-SEGMENTATION-001
---

# Durable agent stream processing

This document specifies stream processing details for the [durable delivery design](durable-agent-delivery.md).
The parent design retains identity, admission, executor storage, and rollout ownership.

The [streaming repair package](../../../plans/durable-agent-stream-repair/plan.md) owns the remaining streaming and capacity corrections.
The earlier implementation results do not establish completion of this repair.
Existing requirements remain authoritative; this section specifies their implementation details.

The [durable text segmentation package](../../../plans/durable-agent-text-segmentation/plan.md)
implements [REQ-PLATFORM-DURABLE-AGENT-SEGMENTATION-001](../requirements/durable-agent-text-segmentation.md)
under [the segment identity decision](../../../decisions/2026-10-26-durable-agent-text-segment-identity.md).

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

The proposed [pruned-history recovery](pruned-delivery-recovery.md) addresses
typed startup diagnosis and unavailable acknowledged intervals.
It preserves actual receipt/projection cursors; the current expiry reconciliation
does not authorize skipping missing receipt.

### Projection and acknowledgment scheduling

A per-stream worker batches at most 64 KiB of compatible output with a maximum 20 ms batching wait.
A larger supported indivisible event uses a singleton batch within the event limit.
The bounded intake budget is 4 MiB; storage work does not hold control-response locks.
Every original event retains its sequence and effect identity.
Tool, permission, terminal, turn, and owner changes flush the preceding batch.
Canonical message changes and projected cursor advancement commit atomically.
Notification follows successful projection and does not depend on HTTP acknowledgment success.

### Canonical text segments

The backend SQL delivery cursor owns the committed text-segment boundary. It
stores the sequence of the latest projected top-level tool call. Projection
updates that sequence in the same transaction as the tool event and projected
cursor. A tool call with a parent ID and a tool update do not advance it.
Every projection route that commits a top-level tool event, including normal
`ProjectAgentDeliveryEvent`, retained replay, and canonical shell-start
projection, updates the boundary in that transaction. A mixed projection batch
applies boundaries in event sequence order.

For an ID-less assistant message chunk, canonical projection combines its
stable base identity, built from the existing stream and turn or submission
scope, with the committed boundary sequence. Chunks before the next top-level
tool call therefore share one canonical message ID. Keep base and resolved
identity distinct: retries and reconnects derive only from the base, never
from an already-resolved ID. Do not mutate caller-owned event payloads before
the transaction commits. Reasoning messages and messages with a native protocol
ID keep their existing identity rules. The `canonical_message_id` inbox column
is the authoritative committed projection ID. The projector persists it in the
same transaction as the event's reconstructable payload metadata and returns
that exact committed value to the notification path. `canonical_message_id` is
projection-only metadata; transport identity and payload hash comparisons
exclude it, and duplicate admission preserves the column value.

Payload compaction changes the payload, pruning timestamp, hash, and byte count
without changing `canonical_message_id`. For a projected row created before
the column existed, an empty column may fall back to the exact ID in an
unpruned stored payload. If that payload has already been pruned, duplicate
projection returns an empty ID and suppresses notification. It does not infer
an ID from incoming reconstructed metadata or the current segment boundary,
rewrite a transcript row, or notify a row that cannot be identified. This
fallback applies only while the inbox receipt still exists. Whole-row receipt
retirement deletes the column with the receipt, advances `retired_sequence`,
and retains the existing expired-replay rejection semantics.

An unprojected retained event resolves its canonical ID from the SQL cursor in
sequence order. Projection persists that resolved ID with the event and message
effect before advancing the cursor, then exposes the result only after commit.
An already projected event returns its stored column value and is not rewritten.
The inbox ID, boundary, and message effect commit atomically, so restart,
duplicate delivery, and partial batch retries cannot advance them independently.
Add the boundary column as nullable for pre-existing streams. On their first
projection after upgrade, initialize a NULL boundary once from the then-current
projected sequence, or from the current sequence if that event is itself a
top-level tool call. New streams initialize the boundary to zero. Later reopen,
owner/generation changes, and cursor updates preserve a non-NULL boundary. This
can begin a fresh text segment after upgrade when earlier boundary evidence was
pruned; it never changes an existing projected transcript row. Initialization,
message effect, resolved event identity, boundary, and projected cursor commit
atomically.

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

Remote agents retain output and continue independent work during prolonged backend absence, including a week, while usable disk remains.
Default production retention has no fixed logical byte ceiling. The former 256 MiB stream and 2 GiB journal limits do not stop execution.
Explicit positive internal limits remain available for bounded-quota tests.
Keep the existing bbolt format, sequence checks, and owner checks.
Keep the 1 MiB event limit and bounded pending producer buffers and replay pages.
Unacknowledged records grow on the executor volume without age-based expiry or deletion to reclaim space.

Disk pressure uses available capacity on that volume, a 32 MiB control/recovery reserve, and pending write headroom.
Capacity sampling is cached and bounded independently of event count.
Unknown disk statistics are not proof of exhaustion. Actual write failures retain typed handling.
ACK pruning reuses free database pages. Optional compaction cannot start if its temporary copy would breach the reserve.
Capacity accounting may credit reusable bbolt pages against payload writes, but not against the physical reserve or allocation headroom.
Exclude pending pages held by readers and deduct in-flight reservations. Report reusable journal pages separately from available filesystem bytes.

On pressure, reconcile the backend's projected cursor with the journal's acknowledged cursor.
Advance only to a verified contiguous committed cursor with matching owner identity, then prune through the existing journal transaction.
Do not fabricate acknowledgment, clear submissions, or require a new prompt to drain the backlog.
When the disk cannot safely accept more output, request cancellation of the exact owned turn before the reserve is exhausted.
Keep status, Stop, and ACK processing available. A failed terminal write preserves uncertainty.
Process liveness remains separate from delivery health. Session focus and stale-execution cleanup cannot kill a live harness because delivery is unhealthy.

Backend shutdown preserves remote runtimes. Local survival remains optional and off by default.
An outage can delay operations that require the backend even while independent agent work continues.
The journal format remains compatible. Older binaries retain their older quota behavior after downgrade.
Downgrade does not grant the long-outage guarantee or permit deletion of unacknowledged records.
Tests cover real retention beyond 256 MiB, accounting beyond 2 GiB, a deterministic elapsed week, reconnect/replay, low disk, and compaction headroom.

Report retained/unacknowledged bytes, ACK pending age, last successful ACK, retry/error categories, pressure transitions, and projection lag.
Keep metric labels bounded; session/stream identifiers belong in structured diagnostics only.
Use the existing recovery card for a localized delivery-storage explanation and Retry connection/Stop, with desktop and phone parity.
Raw SQL cursors and internal storage paths are diagnostic details, not required user decisions.

Oversized assistant/reasoning text uses the existing chunk representation.
[Bounded tool delivery](bounded-tool-delivery.md) defines marked presentation
reduction before acceptance, including generic nested results and duplicate
carriers. Accepted output must not be truncated during replay or projection.
Representation changes require the existing version and rollback checks.
Keep coverage for cumulative snapshots that can multiply retained bytes.

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

## Resumed bootstrap recovery

Generation-one admission includes native conversations first entering durable
delivery; a saved native resume token does not prove SQL authority has committed.
The processor owns its connection context. Processing failure cancels that
connection immediately and reaches disconnect reconciliation with its original
error, even if no later frame arrives. A replacement connection remains independent.

For a native generation-one bootstrap that never received its first event,
explicit startup may establish a checkpoint by receiving the actual retained
sequence-one setup event. Require the matching committed native conversation,
authenticated stream and current execution, a fully retained prefix from one,
and zero peer acknowledgments. The first event must have no submission, turn
content or terminal outcome. The normal SQL receive transaction rejects orphaned
receipts, retired sessions and changed owners; checkpoint creation accompanies
the real receipt.
Preflight performs no projection, acknowledgment, prompt dispatch or block resolution.
Subsequent replay projects retained events through the normal idempotent pipeline.

An initialized silent-restore successor can reconnect before its generation
commit after a failed SQL write. Preflight permits replay from zero only when
the authenticated stream retains the complete prefix from sequence one, has
no acknowledgments or submissions, and SQL still identifies the pinned native
predecessor. This path creates no checkpoint or receipt. Normal event admission
continues to wait for the successor commit under the new bounded restore owner.

## Explicit fresh start

An explicit Start fresh launch is a replacement native conversation. When the
session already has a committed harness generation, the launch uses the next
generation and its own stream, never the predecessor's stream. Before the first
prompt, the peer retires the predecessor's unresolved submissions as
interrupted-unknown without dispatch. The ACP session-created event marks the
replacement. Token acceptance is a prerequisite. The generation transaction
revalidates the task, session incarnation, execution and native token under the
same SQL locks used by executor rotation, then commits the adjacent successor
with compare-and-set on the predecessor and creation reason `native_replaced`.
A rotation after token acceptance therefore cannot commit an obsolete event. Until that commit, the
replacement's events wait under the same bounded wait as other successor
generations; a failed or stale commit leaves them unadmitted and fails startup.
The predecessor's retained stream, submissions and recovery block stay as
history; current-generation admission no longer consults them. Fresh start never
receives, projects, or acknowledges the predecessor stream.
Delivery: [Fresh-start delivery generation](../../../plans/fresh-start-delivery-generation/plan.md).
