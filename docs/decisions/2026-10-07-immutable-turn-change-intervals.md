# ADR-2026-10-07-immutable-turn-change-intervals: Immutable repository intervals for turn history

**Status:** accepted
**Date:** 2026-10-07
**Area:** backend

## Context

Mutable workspace Git status cannot establish the content boundary of one agent turn.
Write-tool events miss shell commands, generators, commits, and external writers.
Cumulative additions/deletions cannot reconstruct a patch or identify unchanged pre-existing dirty content.
Executor cleanup can remove the only repository holding captured objects.

The user confirmed fresh endpoint capture, immutable object IDs, per-checkout identity, and content preservation before cleanup.
This decision records those constraints. Numeric retention limits remain proposals in the draft design.

## Decision

Tasks owns a typed change set bound to durable turn identity and runtime execution generation.
Each enabled turn captures a fresh start and end endpoint for every eligible attached Git checkout.
Capture uses a private writable index and immutable Git objects on the executor.
It preserves the user's index, HEAD, branches, and operation state.

Store exact object IDs and write-once endpoint acceptance.
Retain unique hidden refs while their content remains a required source.
Materialize required historical rendering data into durable backend storage before ephemeral executor cleanup.
Keep compact summary delivery separate from on-demand historical content reads.

Capture ordering belongs to synchronous runtime admission and terminal boundaries.
Event-bus subscribers alone cannot guarantee those boundaries.
Completion state authority remains unchanged; checkpoint processing is a separate state.

Resolve and persist the initiating user's settings policy before dispatch.
Missing settings enable capture; explicit false disables subsequent capture.
The viewer's setting controls card visibility independently of retained history.

## Consequences

Historical comparisons remain stable after later edits, staging, commits, restart, and executor cleanup within retention limits.
Capture needs bounded Git I/O, durable content export, and lifecycle ordering tests.
Shared-checkout interval changes can include other writers; capture does not establish exclusive authorship.
Missing endpoints, partial content, and expiry require explicit states rather than reconstructed history.

## Alternatives Considered

- Tool-call counting misses non-tool writes and cannot produce immutable historical patches.
- Subtracting cumulative line counts loses file identity, reversions, and content.
- Reusing the previous endpoint includes edits between turns and violates the fresh-baseline decision.
- Copying the whole checkout duplicates unchanged files and increases capture cost.
- Retaining refs only on the executor loses promised history when ephemeral compute disappears.
- Capturing from turn-event subscribers permits provider or successor writes before capture completes.

## Related artifacts

- [Requirements](../specs/tasks/requirements/turn-changed-files.md)
- [System design](../specs/tasks/system-design/turn-changed-files.md)
- [Implementation plan](../plans/turn-changed-files/plan.md)
