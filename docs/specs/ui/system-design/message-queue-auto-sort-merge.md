---
status: current
system: ui
requirements:
  - REQ-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001
---

# Message Queue Sorted Auto-merge System Design

## Purpose and boundaries

Issue #5086 extends Auto-merge with same-sender sorted folding and atomic
session-enable compaction. The maintainer/user design revision removes the
separate Auto-sort-merge policy rather than adding a third automation option.
Auto-merge remains ON by default and owns both fold paths. The existing
[automation-controls design](message-queue-automation-controls.md) owns policy
inheritance, identity, ordering, authorization, and client operation tokens.

The alternative of independent switches created a dependent preference and
another persisted/wire tuple for the same merge outcome. The cutover keeps one
operator decision and removes that unused state. Ordinary `auto_merge.go` and
`buildAutoMergedEntry` remain unchanged from `origin/main`; only target selection
and the session-enable transition change. Send Now, Auto-run, manual merge,
append, coalescing, restore, and retry retain their contracts.

## Requirement mapping

| Criteria of `REQ-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001` | Design sections |
| --- | --- |
| .1-.5, .14-.16 | Policy model, Persistence and global settings, Status and frontend |
| .6-.9 | Always-active queue corrections, Target selection and admission |
| .10-.13 | Compaction pass, Session policy mutation |

## Components and responsibilities

- `internal/system/queuesettings` persists Auto-merge's global value/revision and
  publishes `GlobalQueuePolicy{AutoMerge}` from one committed settings envelope.
- `messagequeue.Service` resolves the existing override, snapshots edit leases,
  and serializes session changes through the admission gate.
- Memory and SQL repositories validate Auto-merge and exact session identity,
  choose fold targets, and atomically compact on session effective OFF-to-ON.
- `QueueHandlers` retains `message.queue.auto_merge.set` and publishes the final
  identity-bound queue status. All status producers carry only Auto-merge.
- The queue API, `useQueue`, `queue-policy-meta.ts`, session slice, and WebSocket
  handler retain Auto-merge ordering and recovery. `QueueAutomationPills`
  renders only Auto-run and Auto-merge.

## Always-active queue corrections

These invariants apply regardless of the Auto-merge setting:

- Rows use `(position, id)` total order. Restored reservations can tie positions;
  SQL list, reserve, predecessor, tail, reorder, compaction, managed-input, and
  recovery reads and memory `queueOrderLess` use the same tie-break.
- A tied predecessor is the previous `(position, id)` row, including a row at
  the same position. Automatic and manual merge use that predecessor.
- `idx_queued_messages_session_order(session_id, position, id)` replaces the old
  position-only index through replay-safe DROP/CREATE at initialization.
- Edit operations acquire `editLeaseMu` before the repository fence/write.
  Folding receives a lease snapshot captured under the admission gate; it never
  acquires the service edit-lease mutex from a repository transaction.

## Policy model

`GlobalQueuePolicy` contains only `AutoMerge AutoMergePolicy`. `AdmissionPolicy`
contains that effective snapshot, the in-process leased-entry set, and the
logger. `sortedEnabled()` is exactly `policy != nil && policy.AutoMerge.Enabled`.
There is no independent sort flag, override, revision, validator, or resolver.

`resolveAdmissionAutoMergePolicy` loads global settings before any queue
transaction, then reads the session override. A readable explicit override
still governs both adjacent and sorted admissions when the global loader is
unavailable. An inherited admission with unavailable global settings, or any
unreadable session override, admits separately only if capacity permits.

The repository revalidates the Auto-merge source/value/revision under its
transaction or memory mutex. A changed override returns
`ErrAutoMergePolicyChanged`; the service reloads policy outside the transaction
and retries. This preserves cross-connection ordering without a nested settings
read on SQLite's shared single-connection writer.

## Persistence and global settings

The existing `auto_merge_enabled` setting, internal `auto_merge_revision`, and
nullable `queue_session_state.auto_merge_override` / `auto_merge_revision`
remain the only automatic-fold policy. Startup and loaders hydrate that pair.
Settings PATCH/GET contain no independent sorting field.

Because the branch is unreleased, the removed sort columns are absent from both
fresh CREATE TABLE and replay-safe ALTER declarations. No compatibility shim,
destructive drop migration, or retained SQL reads/writes are added for them.
Existing development databases may retain inert columns, but fresh schemas and
runtime state do not depend on them. The removed memory map and incarnation,
transfer, purge, and snapshot references disappear too.

Global saves update only the global value/revision. They never iterate sessions,
rewrite pending rows, or trigger compaction. Override lifetime, deletion,
archive, transfer, and incarnation rules remain owned by the automation design.

## Target selection and admission

Folding uses the persisted row stream, including rows hidden from the panel.
`sortedFoldSender` permits only an agent source with a non-empty sender task
that is not a fold barrier. `isFoldBarrier` recognizes reserved/in-flight,
durable-delivery, workflow, server, and move-task rows.

`sortedFoldTarget` walks upward from the source and stops at the first barrier
or same-sender row. Only that latest same-sender row is checked for compatibility
and leases. An incompatible/leased target never opens a search for an older
same-sender row. If sorted folding cannot succeed, the ordinary immediate
predecessor remains the fallback. User and sender-less sources never move.

All existing admission variants use this selection when Auto-merge is ON:

- Two-transaction admission finalization selects against persisted rows above
  the inserted source and applies the existing target CAS and edit-lease guard.
- Client-queue-ID receipt admission folds inside its admission transaction.
- At/above capacity, an unstaged candidate can fold directly into a sorted or
  adjacent target without inserting a row. Staged attachments still require
  insertion capacity and are rejected before a claim when full.

SQL's sorted metadata walk uses keyset batches on `(position, id)`, stopping
before decoding older backlog payloads once it has the latest target or barrier.
Ties at batch boundaries are not skipped. No full-backlog copy is added to
ordinary admission. The target retains its ID/position, uses the later queued
time, and receives content/context through the unchanged compatibility helper.

## Compaction pass

`compactSessionQueueTx` and `compactSessionQueueLocked` run the same FIFO pass
through `compactQueueRows`. A per-segment latest-by-sender index tracks the last
kept row of each agent sender. A source first tries that row, then the immediately
preceding kept row. Barriers reset the index and remain untouched. A leased row
remains in the index but can neither move nor receive content; other senders may
cross it without using it as a target.

SQL snapshots durable lease IDs inside the transaction and unions the service's
in-process snapshot. It also reads `queue_task_management_fences`. A fold is
allowed only when target/source management claims match, including installation,
instance, and generation, or neither row is fenced. The survivor therefore never
contains instructions under an unrelated claim that could later be discarded
by stale-claim reservation. Memory has no persisted management fences.

`compaction_accumulator.go` accumulates target content, attachment validation,
and additive context without repeatedly copying the growing body or decoding
kept attachments. Each input is consumed once; dirty targets materialize once
at the end. Target lookup is O(1) per row and reset cost is proportional to each
segment, not the largest previous segment. The pass covers the entire backlog,
including above-capacity/unlimited queues, with no new product limit.

SQL writes dirty targets and deletes folded rows using existing CAS boundaries.
The pass preserves surviving IDs/positions, barriers, leases, Auto-run, and
unmerged relative order. It starts no prompt delivery.

## Session policy mutation

`SetSessionAutoMerge` returns `SessionPolicyResult{AutoMerge, CompactedCount}`.
The service enters the session admission gate, loads one global snapshot, and
passes it with the lease snapshot to `SetSessionPolicyAndCompact`.

The repository locks the task and then the session, validates its incarnation,
resolves the prior Auto-merge value, writes the explicit override/revision, and
resolves the result against the same supplied global snapshot. Only
`!before.AutoMerge.Enabled && after.AutoMerge.Enabled` invokes compaction.
Policy and rows commit atomically; an error rolls back both. ON-to-ON and OFF
writes never compact, and disabling never splits merged content.

The handler publishes one final authoritative status after commit. The response
includes `compacted_count` (removed source rows) but is not an ordered status.
The client holds its shared mutation token through the authoritative refetch,
including error recovery, and releases it only for the same incarnation and
operation generation.

## Status and frontend

Only the existing Auto-merge tuple remains:

- `auto_merge_available`
- `auto_merge_enabled`
- `auto_merge_source`
- `auto_merge_revision`

`message.queue.auto_merge.set` is the only automatic-merge action. The removed
sort action, controller, API helper, hook action, tuple, and status producers
have no aliases. Repository snapshot generation is allocated before global
resolution; a delayed failing loader therefore keeps its earlier generation.
Unavailable status preserves entries and the last known Auto-merge value.
Source/revision ordering and epoch/incarnation rejection remain unchanged.

The settings page has one automatic-merge draft with normal shared Save/Discard
behaviour, no dependent sort draft. Copy describes adjacent folding, agent
sub-task same-sender folding, immediate session-enable compaction, and global
changes affecting later messages only. All locales are updated; Traditional
Chinese and pseudo catalogs are generated, with no em dashes in user copy.

Desktop/mobile use the existing inline queue panel and two wrapping pills. The
removed sort pill's touch help drawer is removed, not replaced. Existing
coarse-pointer 44px targets, single queue scroller, and remaining tooltip
`data-queue-escape-boundary` markers are retained.

## Failure, security, and observability

- Unreadable policy disables automatic folds; it never invents ON.
- A compaction conflict/storage error rolls back override and rows, then the
  client refetches and reports the localized failure.
- Drain, Send Now, edit, reorder, and policy writes serialize at the existing
  identity/lock boundary; none observes a partial compaction.
- Existing triplet authorization and non-enumerating not-found errors apply;
  sender identity comes from trusted persisted metadata. Only admins save the
  global setting, while session access permits the session mutation.
- Sorted admission logs target/source IDs and crossed rows at debug level.
  Compaction logs session, removed-row count, and `trigger: auto_merge` at info.
  Policy-read failures warn. No new metric or retry mechanism is added.

## Verification strategy

Memory/SQLite and env-gated PostgreSQL coverage drives sorted folds through
default/global/session Auto-merge. It includes interleaved senders, users that
stay in place but adjacent-merge, OFF disabling both paths, full capacity,
latest-only/barrier/lease rejection, keyset ties, accumulator scaling, claim
fences, atomic rollback, session-only compaction, loader resilience, and
cross-connection policy races. PostgreSQL is unverified when
`KANDEV_TEST_POSTGRES_DSN` is absent.

Focused frontend tests retain policy ordering, recovery, operation-token
ownership, and settings draft races. Desktop/mobile browser flows use real
mock-agent sub-task messaging, the Auto-merge pill, persisted queue assertions,
reloads, and mobile target/overflow checks. Existing adjacent-user compaction
and settings scenarios run alongside them.

## Related decisions

- [Inherit Queue Auto-merge Until Overridden](../../../decisions/2026-09-04-inherit-queue-auto-merge-until-overridden.md)
- [Keep Queue Auto-run Server Owned](../../../decisions/2026-08-16-server-owned-queue-auto-run.md)
