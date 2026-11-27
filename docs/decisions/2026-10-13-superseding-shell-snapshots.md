# ADR-2026-10-13-superseding-shell-snapshots: Bound superseding shell snapshots before durable admission

**Status:** accepted
**Date:** 2026-10-13
**Area:** backend, protocol

## Context

ACP shell notifications are normalized into full accumulated stdout/stderr snapshots.
A read-only incident trace found 9,954 large updates in six minutes, totaling
2,676,905,950 payload bytes, although the final stdout retained 256 KiB.
Cleanup later released these bodies but SQLite kept the empty pages, and backups
copied them. Individual event byte bounds do not bound repetition of full snapshots.

## Decision

Reduce only consecutive, complete, superseding shell-output snapshots before they
enter the lossless normalized-event channel and receive a journal sequence.
Continue normalizing every input delta. Reuse the ACP notification worker with one
pending immutable snapshot and a fixed 250 ms flush window. Emit initial output and
terminal results immediately; flush before ordered barriers and quiet trailing output.
No new wire representation, journal format or database schema is introduced.

Already accepted journal events remain immutable and lossless. Conversation chunks,
reasoning, permissions, lifecycle/configuration boundaries, background work, unknown
shapes and non-shell tools are never discarded by this rule. Graceful completion
flushes pending output; shutdown joins the existing worker and cancels its timer.
Unjournaled pending output has no durable acceptance claim.

Implementation and validation are recorded in the [repair package](../plans/delivery-stream-storage/plan.md).

## Consequences

Continuous shell snapshot storage depends on elapsed time and bounded output size,
rather than provider chunk count. Intermediate visual snapshots can be superseded,
while final bounded stdout/stderr and command state remain identical.
Ordered barriers can force more frequent emission. Other transports keep their current
behavior until their normalized snapshot semantics have separate tested coverage.

## Alternatives Considered

- Shorten payload retention: changes recovery windows and still writes every snapshot.
- Compress all inbox bodies: reduces average bytes but leaves repetition and event/index growth.
- Rewrite or coalesce committed inbox/journal rows: breaks sequence/hash and replay guarantees.
- Introduce output deltas on the wire: needs versioned readers, tail reconstruction and rollback.
- Only suppress identical snapshots: bounded shell output retains a moving tail, so noisy
  commands continue producing different full snapshots after truncation.
- Rate-limit by dropping raw ACP notifications: loses deltas needed for the correct final output.

## Related records

- [Storage requirements](../specs/platform/requirements/pr-watch-and-bounded-storage.md).
- [Stream storage design](../specs/platform/system-design/delivery-stream-storage.md).
- [Replay checkpoints](2026-10-05-backend-delivery-retirement-checkpoints.md).
