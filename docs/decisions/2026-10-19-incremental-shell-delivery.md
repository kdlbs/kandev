# ADR-2026-10-19-incremental-shell-delivery: Incremental shell output before durable acceptance

**Status:** accepted
**Date:** 2026-10-19
**Area:** protocol, backend

## Context

The accepted 2026-10-13 snapshot coalescer bounds frequency, but not lifetime
bytes. A controlled-time real-adapter fixture produced 1,890,158,400 emitted
JSON bytes from 115,200 new bytes over two simulated hours with a saturated
256 KiB stdout tail. This is separate from the parent's pruned historical data.
Digest-backed gzip output also stores different moving tails under new digests.

## Decision

Use explicit version-2 incremental shell events before journal acceptance,
with byte-progress/event-count checkpoints, and atomic incremental canonical
tail persistence. Keep full provider-neutral normalization and current live
cadence. Keep version-1 retained runtimes readable without rewriting evidence.
The [design](../specs/platform/system-design/incremental-shell-delivery.md)
defines the version, recovery and byte budgets. The reviewed package was implemented on 2026-10-19; the v1 contract remains
supported for retained runtimes.

## Consequences

Append/sliding-tail storage becomes proportional to changes plus bounded metadata
and checkpoint overhead, including slow streams after truncation. Recovery needs
explicit base revisions, typed failure and an atomic canonical tool projector.
New output requires new readers; old runtimes retain old costs until safely
replaced. New journal format and SQL storage make transparent in-place downgrade
unsupported. Existing UI APIs and command/result semantics remain stable.

ADR-0036 continues to own normalization. ADR-0042's browser projection remains;
its full-blob persistence choice is extended for v2 live output. The existing
snapshot-admission ADR remains the v1 path and the logical coalescing rule for v2.

## Alternatives considered

- **Longer/adaptive snapshot interval:** Simple, but time-based snapshots still
  multiply tail size by duration, or break current live availability. A byte-only
  publication threshold starves sparse output until completion.
- **Incremental events with no checkpoints:** Linear output bytes but long replay
  chains and fragile producer restart. Bounded progress checkpoints limit that work.
- **Compact inbox only:** Leaves producer journal volume and canonical blob churn.
  Re-encoding already accepted events also complicates hash/duplicate evidence.
- **Chunk-addressed snapshots on both sides:** Can reuse most text, but creates a
  content-addressed distribution protocol, reference-GC and page dependency graph.
  Explicit tool-scoped patches are easier to order and validate.
- **Compress every snapshot:** Data-dependent savings; incompressible tails still
  repeat. Compression can supplement the chosen representation.
- **Shorten grace, prune unacknowledged bodies, or VACUUM:** Changes recovery or
  physical allocation after the producer already wrote repeated snapshots.

## Related records

- [Snapshot admission](2026-10-13-superseding-shell-snapshots.md).
- [Shell normalization](0036-normalize-acp-shell-output-at-adapter-boundary.md).
- [Browser projection](0042-project-shell-output-and-fetch-on-demand.md).
