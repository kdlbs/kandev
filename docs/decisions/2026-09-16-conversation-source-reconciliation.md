# ADR-2026-09-16-conversation-source-reconciliation: Read original conversation records

**Status:** accepted
**Date:** 2026-09-16
**Area:** backend, frontend, protocol

## Context

PR #3588 adds sanitized message and turn versions, a primary event journal, and a separate Host event database.
Startup copies existing history. Database triggers copy subsequent mutations.
Retention removes old events but preserves the latest version of each record.
Every installation bears this cost, including installations without a conversation plugin.

The user requested a replacement design package after review of this cost.
The existing browser facade remains useful. Durable replay is not necessary for a current-state history panel.
The earlier [maintainer direction](https://github.com/kdlbs/kandev/issues/3567#issuecomment-5619730030) also requested justification for durable replay.

## Decision

Original task messages and turns remain the only authoritative conversation payloads.
The Host exposes authorized, sanitized reads and manages current-state reconciliation.
A small per-session revision supports consistency checks without payload history.
Revision changes commit with source mutations. Transient receipts bind complete change intervals to their persisted results.
Subscriptions deliver updates by ID and coverage-only batches for irrelevant changes.
Applied revisions advance only through complete contiguous batches or source snapshots.
Observed revisions cannot establish cache correctness by themselves.
Hashes are excluded. They add maintenance and write cost without establishing delivery completeness.
Periodic checks cover lost notifications and writes outside the event bus.

Browser hook signatures, task-panel capabilities, and native navigation remain compatible.
The plugin owns filtering, derived entries, durations, and presentation.
An optional plugin cache stores only data necessary for its feature under existing storage permissions.
A plugin does not need a full transcript copy to display Prompt History.

The Host does not preserve intermediate revisions or exactly-once event delivery for this facade.
A future durable replay requirement needs a separate decision and measured storage costs.

## Consequences

Startup no longer copies history for plugin access. Normal writes update only a small revision record for this capability.
Normal live changes update individual IDs without rereading loaded history.
Recovery can read the loaded range again after missing delivery or reconnect.
The design bounds transient batching and checks revisions without reading payloads.
Mutation writers must capture complete receipts inside their transactions. Exceptional untracked writes trigger repair.
Live history is not an immutable snapshot across page requests.

Cleanup must remove existing triggers, tables, and the specific Host event database.
A source revert alone leaves database triggers active.
SQLite space becomes reusable after cleanup but physical shrink requires the existing operator compaction workflow.
Downgrade after cleanup requires backup restoration. Mixed old and new backend binaries are unsupported during cutover.

## Refinement, 2026-09-16

The user selected incremental updates without hashes after discussion of broad refresh costs.
This refinement replaces the initial invalidation-only plan. Database cleanup remains unchanged.

## Alternatives Considered

- Hash each message: rejected because hashes do not prove delivery coverage and introduce content-hash maintenance.
- Refresh on every revision: rejected because unrelated streaming repeatedly rereads unchanged prompts.
- Keep the journal and tune retention: rejected because latest versions still duplicate content and startup still copies history.
- Move the full journal into a plugin: rejected because the panel needs current records, not a second transcript history.
- Rely only on the existing event bus: rejected because missed events and direct database mutations need a repair path.
- Hold database snapshots across browser requests: rejected because slow or suspended clients prolong database transactions.
- Revert all of PR #3588: rejected because the useful API, navigation, and display boundaries can remain.

## Related artifacts

- [Host requirements](../specs/plugins/requirements/prompt-history-extraction-host.md)
- [Replacement design](../specs/plugins/system-design/conversation-source-reconciliation.md)
- [Implementation plan](../plans/conversation-storage-replacement/plan.md)
- [Browser facade decision](2026-09-06-browser-plugin-conversation-facade.md)
