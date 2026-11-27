# ADR-2026-10-05-backend-delivery-retirement-checkpoints: Bounded backend replay evidence

**Status:** accepted
**Date:** 2026-10-05
**Area:** backend, protocol

## Context

The backend stores every delivery event and its completed generic effect indefinitely.
Full payload comparison currently proves duplicate content. Simply deleting these rows makes replay look new.
Canonical history and unresolved settlement have separate lifetimes from redundant transport payloads.
Retaining one hash per event forever still creates storage proportional to streaming chunk count.

## Decision

Use a versioned normalized hash for recent compact events and a monotonic contiguous retirement checkpoint for older eligible events.
Beyond the retained hash window, reject replay explicitly instead of claiming that unknown content matches.
Authenticated state-only recovery can retry ACK from the exact projected cursor and reconnect after it.
It cannot resend a prompt, recreate a missing cursor or replay a tool action.

Retain immutable submission summaries, terminal evidence, authoritative workflow receipts and cursors until explicit session deletion.
Pending terminal settlement and unresolved authority remain protected regardless of age.
Retire deleted identities atomically and purge their rows through durable bounded jobs.
A textual session ID cannot be reused until its purge job finishes. Later recreation requires a fresh incarnation.
Deterministic plugin conversation primary-session IDs can recur after deletion (`conversationPrimarySessionID` in task service). This rule permits complete historical purge without an unbounded foreground inventory copy.

Pruning changes the database recovery format irreversibly. Supported rollback binaries must understand its checkpoints and compact evidence.
A pre-retention binary cannot safely use a pruned database, even though SQL evolution is additive. Merged PR #3598/current main is still pre-retention; the merge does not supply the proposed release N reader.
The format marker cannot force an old binary that ignores it to refuse startup.

## Consequences

Healthy transport storage depends on recent traffic and grace, not total lifetime streaming count.
Compact submission, terminal and workflow evidence still grows with canonical work history.
Protected gaps and cleanup overload remain visible growth sources.
An expired event receives a typed local error. Recovery must use authenticated SQL checkpoint evidence, not the event body.
This changes internal duplicate handling without changing agentctl protocol version 1.

Every foreground delivery writer and collector must share transaction serialization. Reuse the merged SQLite factory's immediate native BEGIN and separate readers; its 5-second busy wait is distinct from the 250 ms admitted-work budget.
Managed deletion installs retirement only with final canonical deletion and its committed marker; reserved/prepared admission is reversible.
Logical payload deletion does not guarantee physical file shrinkage or removal from backups.
Use an expand/contract release boundary without a feature flag: N ships Tasks 01-02 as compatible format-1 readers/writers with no collector; N+1 ships Tasks 03-08 and activates pruning. N is the required tested rollback target; do not activate N+1 until a real N artifact passes pruned-database recovery/write/deletion/forward-upgrade tests.
Rollback to N pauses collection but preserves jobs, retirement admission and compact evidence. Pre-retention rollback requires a verified pre-pruning restore with explicit external-action uncertainty.
SQLite version-change VACUUM INTO snapshots are rollback artifacts, but only two survive rotation; preserve a verified external copy. PostgreSQL requires a verified operator pg_dump before N+1 upgrade and a tested restore. This release split delays activation to provide a compatible rollback window.

## Alternatives considered

- Keep all payloads: preserves comparisons but leaves unlimited redundant storage.
- Keep a permanent hash/effect row per event: reduces bytes but retains unlimited streaming row/index growth.
- Treat every sequence at or below a cursor as an identical duplicate: loses content-conflict detection and accepts expired unauthenticated evidence.
- Require confirmed remote ACK before payload cleanup: pins content after lost replies even though canonical state and checkpoints already support safe ACK retry.
- Delete all old completed effects: can repeat workflow transitions and removes terminal evidence used by late block binding.
- Delete all session delivery rows in the foreground deletion transaction: creates unbounded locking and shutdown latency for large streams.
- Age-expire unresolved submissions: replaces uncertainty with an unsafe inference that external work did not run.

## Related records

- [Requirements](../specs/platform/requirements/backend-agent-delivery-retention.md).
- [System design](../specs/platform/system-design/backend-agent-delivery-retention.md).
- [Implementation plan](../plans/backend-agent-delivery-retention/plan.md).
- [Existing store boundary](2026-09-10-durable-harness-session-boundaries.md).

The user authorized implementation on 2026-10-09. Final storage, rollback and browser evidence remains in Task 08.
