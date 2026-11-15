---
status: current
system: ui
requirements:
  - REQ-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001
---

# Message Queue Auto-sort-merge System Design

## Purpose and boundaries

This design adds the Auto-sort-merge policy end to end, from the global setting
and session override through admission folding, the session compaction pass,
queue status, and the queue-panel pill. It reuses the policy, identity,
locking, and status-ordering boundaries of
[Message Queue Automation Controls](message-queue-automation-controls.md) and
the [Auto-merge inheritance decision](../../../decisions/2026-09-04-inherit-queue-auto-merge-until-overridden.md).
Those documents stay authoritative. This design describes only the parts that
are added or differ.

It does not change the Auto-merge compatibility predicate
(`buildAutoMergedEntry`), the manual merge, reorder, Send Now, or Auto-run. The
only Auto-merge behaviour it changes is that a session Auto-merge OFF-to-ON
write can now trigger a compaction pass
(`AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.10`).

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001` .1, .2, .3, .13 | [Policy model](#policy-model), [Persistence](#persistence), [Global settings](#global-settings) |
| `REQ-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001` .6, .7, .8, .9, .14 | [Target selection](#target-selection), [Admission flow](#admission-flow) |
| `REQ-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001` .10, .11, .12 | [Compaction pass](#compaction-pass), [Policy mutations](#policy-mutations) |
| `REQ-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001` .14, .15 | [Status contract](#status-contract), [Frontend state](#frontend-state) |
| `REQ-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001` .4, .5, .16 | [Queue header and settings UI](#queue-header-and-settings-ui) |

## Components and responsibilities

| Component | Responsibility |
| --- | --- |
| `internal/system/queuesettings` | Persists `auto_sort_enabled` and its internal revision in the `message_queue` settings value, then publishes the committed pair to the queue service. |
| `internal/orchestrator/messagequeue.Service` | Holds the immutable global Auto-sort-merge snapshot, resolves the session preference, computes the effective policy, and orders policy writes with admissions using the existing admission gate. |
| `internal/orchestrator/messagequeue` repositories (memory, SQLite/PostgreSQL) | Persist the nullable session override and its revision. They select same-sender targets during admission and run the compaction pass in the policy-write transaction. |
| `internal/orchestrator/handlers.QueueHandlers` | Adds `message.queue.auto_sort_merge.set` and projects the policy tuple on every session-scoped status. |
| `pkg/websocket` | Declares the new action name. |
| `queue-api.ts`, `useQueue`, the session slice, and `ws/handlers/agent-session.ts` | Carry, order, and preserve the Auto-sort-merge tuple, and send the mutation. |
| `QueuePanelHeader` (`queued-ghost-panel-header.tsx`) | Renders the third pill and its dependency on Auto-merge. |
| `MessageQueueSettings` (`message-queue-settings.tsx`) | Renders the dependent global switch. |

## Policy model

`types.go` adds types parallel to the Auto-merge ones:

```go
type AutoSortMergePolicy struct {
    Enabled  bool            // resolved preference, not the effective value
    Source   AutoMergeSource // "global" | "session"
    Revision int64
}
type AutoSortMergeOverride struct { Enabled bool; Revision int64 }

// GlobalQueuePolicy replaces the single global Auto-merge snapshot.
type GlobalQueuePolicy struct {
    AutoMerge     AutoMergePolicy     // Source = global
    AutoSortMerge AutoSortMergePolicy // Source = global
}
```

Both global values are published as **one** immutable `GlobalQueuePolicy`
behind one `atomic.Pointer`. `SetAutoMergePolicy` and `SetAutoMergePolicyLoader`
are replaced by `SetGlobalQueuePolicy(GlobalQueuePolicy)` and
`SetGlobalQueuePolicyLoader(func(ctx) (GlobalQueuePolicy, error))`. The loader
performs one persisted settings read and returns both value/revision pairs from
that read. An admission, status read, or session policy transition therefore
never combines an Auto-merge value from one settings write with an
Auto-sort-merge value from another; each pair keeps its own revision for
client ordering. Every existing caller of the replaced setters (startup wiring,
`queuesettings` targets, test doubles) migrates in the same change.

The loader is never called inside a repository transaction. Its ordering
differs by path:

- **Admission attempts and their policy-changed retries, and
  `SetSessionPolicyAndCompact`:** the service loads one combined snapshot
  before opening the mutation transaction and passes it in (see
  [Policy mutations](#policy-mutations)).
- **`Service.Snapshot` (status reads):** the existing order is kept.
  `repo.Snapshot` captures entries, both override read outcomes, and the
  status generation in its transaction and commits. Only then does the service
  load one combined global snapshot. A loader failure marks each inherited
  policy unavailable on its own, and the generation already allocated is kept.
  This keeps the ADR rule that the generation is allocated before policy
  resolution, so a delayed failing status cannot receive a newer generation
  than a later successful one.

Each admission resolves both session overrides against its one loaded
snapshot into `AdmissionPolicy{AutoMerge, AutoSortMerge}`.
`effectiveSort = AutoMerge.Enabled && AutoSortMerge.Enabled`. The preference is
stored and projected separately from the effective value, so turning
Auto-merge OFF never rewrites the Auto-sort-merge preference (`.2`, `.5`).

The repository re-validates the Auto-sort-merge policy inside the transaction
in the same way as `validateAutoMergePolicyTx`: a `global` source requires that
no override row exists, and a `session` source requires the stored value and
revision to match. A mismatch returns the existing
`ErrAutoMergePolicyChanged` retry signal, so the service loop re-resolves both
policies against a freshly loaded snapshot and retries.

## Persistence

`queue_session_state` gains two columns:

```sql
auto_sort_merge_override INTEGER NULL,
auto_sort_merge_revision BIGINT  NOT NULL DEFAULT 0
```

Both the fresh schema and the replay-safe `ALTER TABLE` path in
`repository_sqlite.go` add these columns. Duplicate-column errors are ignored,
as for `auto_merge_override`. `NULL` means inherit. A change of session
incarnation resets the override to `NULL` and the revision to `0`, in the same
places that reset the Auto-merge override. Workflow transfer deletes the source
row's state and keeps the destination's state. Session, task, and workspace
deletion remove it in the same transaction. Archive keeps it. The memory
repository adds an `autoSortMergeOverrides` map that sits beside
`autoMergeOverrides` under the same mutex and incarnation tagging.

`GetAutoSortMergeOverride` and `SetAutoSortMergeOverride` mirror the
Auto-merge pair. A write upserts the value, sets `revision + 1`, and is guarded
by the incarnation.

## Global settings

`queuesettings.Settings` adds `AutoSortMergeEnabled bool` (JSON
`auto_sort_merge_enabled`) and `AutoSortMergeRevision int64` (`json:"-"`).
`SettingsPatch` and `Effective` gain the matching fields. `storedSettings` reads
an omitted value as `false` (`.1`). `Service.Update` increments the revision
only when the value changes, commits the value and revision in one write, and
then publishes one `GlobalQueuePolicy` built from the committed envelope. The
optional `revisionedAutoMergeTarget` and `autoMergePolicyLoaderTarget`
interfaces in `queuesettings/service.go` are replaced by one
`globalQueuePolicyTarget` (publish + loader); the loader reads the envelope
once. Startup in `backendapp/orchestrator.go` hydrates the combined snapshot
from `queuesettings.Resolution` before admissions start.

The PATCH accepts `auto_sort_merge_enabled` whatever the value of
`auto_merge_enabled`. The dependency is enforced by evaluating the effective
policy, not by rejecting writes. Global saves never scan or compact sessions
(`.13`). Only admins may change the setting, through the existing PATCH
authorization.

## Target selection

All fold decisions use the **persisted** position-ordered row stream of the
session, including rows hidden from status (`IsReservedInFlight`). Visibility
is a client projection only; it never defines adjacency. This matches
`readMergeTarget`, which sees the real persisted predecessor.

`isFoldBarrier(row)` is a fold-specific predicate, separate from the
ownership/security predicate `IsReservedQueuedBy` (which includes `agent` and
is unchanged):

```go
func isFoldBarrier(m *QueuedMessage) bool {
    return m.IsReservedInFlight() || m.IsDurableDelivery() ||
        m.QueuedBy == QueuedByWorkflow || m.QueuedBy == QueuedByServer ||
        m.QueuedBy == QueuedByMoveTask
}
```

`sortedFoldTarget(rowsAbove, source)` is one pure function shared by both
repositories and by admission. `rowsAbove` is the persisted rows above
`source`, in position order.

1. If `source.QueuedBy != agent` or `sender_task_id` is empty, return none
   (`.8`).
2. Scan upward from the row directly above `source`. Stop and return none on
   `isFoldBarrier`. User rows and agent rows from other sender tasks are
   crossed.
3. Return the first row whose `QueuedBy == agent` and whose `sender_task_id`
   equals the source's. This is the latest same-sender entry. The caller
   checks compatibility on that row only and never falls back to an older
   same-sender row (`.7`).

The caller applies the unchanged `buildAutoMergedEntry(target, source)` and
the edit-lease check on that target. If either fails, there is no sorted fold
and the caller tries the ordinary adjacent fold (the existing `readMergeTarget`
predecessor rule). The source otherwise keeps its tail position.

## Admission flow

The existing Auto-merge admission flow is unchanged except for target
selection when `effectiveSort` is true. Every path tries the sorted target
first and then the ordinary adjacent target.

- **Two-transaction identity path** (`finalizeAutoMerge` →
  `autoMergeIntoAbove`): the fold transaction reads the persisted rows above
  the inserted source. Both the repository edit-lease check
  (`editLeaseBlocksEntryTx`) and the in-process `Service.editLeases` check
  apply to the chosen target; `editLeaseBlocksTail` is generalized to
  `editLeaseBlocksEntryLocked(target)`.
- **One-transaction client-queue-id path**
  (`mergeInsertedAdmissionCandidateTx`): the same selection inside its
  transaction.
- **Full queue** (`autoMergeCandidateIntoAbove`, `admitFullQueueCandidateTx`):
  for a candidate that needs no staged attachments, try the sorted target,
  then the existing adjacent tail fold. The existing compare-and-swap on
  position, content, attachments, metadata, and `queued_at` applies to
  whichever target is chosen. When neither folds, the outcome stays
  `queue_full`. Staged-attachment admissions keep returning `queue_full`
  before any claim (`.9`).
- The fold write is the existing `applyMergeWrites`. The target keeps its ID
  and position, `queued_at` becomes `latestQueuedAt`, and the source row is
  deleted (`.6`).
- If the Auto-sort-merge override read fails, `effectiveSort` is false for that
  admission and ordinary Auto-merge still applies (`.14`). If the Auto-merge
  read fails, the existing fail-closed path disables both.

Lifecycle coalescing, append, restore, requeue, and the separate-admission
paths still never fold.

## Compaction pass

`compactSessionQueueTx(tx, identity, leased)` runs inside an already-locked,
identity-validated transaction over the full persisted row stream:

```text
latestBySender := map[sender_task_id]*row   // reset only at fold barriers
prev := nil                                  // last kept row (any kind)
for row in persisted rows by position:
    if isFoldBarrier(row):
        latestBySender = {}; prev = row; continue      // segment boundary
    if row in leased:                    // untouched, but not a barrier
        prev = row
        if row is agent with sender S: latestBySender[S] = row
        continue
    target := none
    if row is agent with sender S: target = latestBySender[S]
    if target != none && target not leased && buildAutoMergedEntry(target,row):
        fold row into target in memory; mark target dirty; delete row
        continue                         // prev unchanged
    if prev != none && !isFoldBarrier(prev) && prev not leased &&
       buildAutoMergedEntry(prev,row):
        fold row into prev; mark prev dirty; delete row; continue
    keep row; prev = row
    if row is agent with sender S: latestBySender[S] = row
write dirty targets (CAS on previous content/metadata/attachments/queued_at)
delete folded rows; positions of kept rows unchanged
```

Hidden reserved/in-flight rows and other fold barriers are kept untouched and
reset the sender index, so no fold crosses them (`.8`). An edit-leased row is
not a barrier: it is kept untouched and is never a fold source or target
(`.12`), but it stays in the sender index and becomes `prev`. A later
same-sender row therefore selects the leased row as its latest target and does
not fold, and other senders' folds may cross it. When the leased row is a
target this matches admission, which checks the lease only on its chosen
target: for A1, leased U1, A2 both produce A1+A2, U1 with U1 unchanged. A
leased row is also never consumed as a source, which in-order admission would
do (for U1, leased U2 admission yields U1+U2; compaction keeps U1, U2).
SQL compaction also loads `queue_task_management_fences` for the session and
folds two rows only when their task, installation, instance, and generation
are identical (or neither row is fenced). The survivor therefore keeps a fence
that matches all its content, so a later stale-claim check at reservation can
never discard an instruction admitted under the current claim. The memory
repository stores no fences. With those lease and claim exceptions, each step
uses the target's already-merged state, so the result equals in-order
admission with the policy ON (`.11`). Target lookup is O(1) per row through
the per-segment index; the
pass is O(N) lookups plus one compatibility check per row, one update per
dirty target, and one delete per folded row. N is the session's current
persisted row count, which is not bounded by the current capacity: capacity
may be non-positive (unlimited) and lowering it never prunes rows. The pass
therefore runs over the whole backlog and adds no new product cap.

The pass changes no Auto-run state and starts no delivery. SQL reads leased
entry IDs with `queue_edit_leases` in the same transaction; the service also
passes its in-process lease set, as reorder does.

## Policy mutations

Both session writes share one result type and one repository operation:

```go
type SessionPolicyResult struct {
    AutoMerge      AutoMergePolicy
    AutoSortMerge  AutoSortMergePolicy
    CompactedCount int
}
func (s *Service) SetSessionAutoMerge(ctx, identity, enabled bool) (SessionPolicyResult, error)
func (s *Service) SetSessionAutoSortMerge(ctx, identity, enabled bool) (SessionPolicyResult, error)
```

`SetSessionAutoMerge` changes its return type from `AutoMergePolicy`; the
`QueueAutoMergeController` interface in `handlers/queue_handlers.go`, the new
`QueueAutoSortMergeController`, and every test double (for example
`handlers/queue_admission_timeout_test.go`) migrate together. Both methods run
under the admission gate. Before opening any repository transaction, the
service loads one `GlobalQueuePolicy` (the persisted loader, or the atomic
snapshot when no loader is installed) and passes it to
`SetSessionPolicyAndCompact(ctx, identity, change, global)`, which:

1. locks the task row and then the session lock, and re-validates the
   identity;
2. reads both overrides and resolves `before := merge && sort` against the
   supplied `global`;
3. writes the changed override, which increments its revision;
4. resolves `after` against the same `global`. When `!before && after` and
   pending rows exist, it runs `compactSessionQueueTx` (`.10`);
5. commits everything as one transaction. On failure nothing changes and the
   handler returns the existing error response (`.12`).

The persisted settings loader is never invoked inside a repository
transaction. On SQLite the settings store and the queue repository share the
single-connection writer pool (`internal/db/sqlite.go`), so a nested
`GetConsistent` read from inside the held transaction would wait on its own
connection. A global change that commits between the load and the
transaction is ordered after this session write; since global changes never
compact (`.13`), the session transition is evaluated consistently against the
one snapshot it loaded.

A write that does not turn the effective value from OFF to ON never compacts
(`.13`). The handler publishes one identity-bound status through
`publishStatusForIdentity` after commit.

## Status contract

`QueueStatus` adds fields beside the Auto-merge tuple, using the existing
naming (`auto_merge_enabled`):

```json
{
  "auto_sort_merge_available": true,
  "auto_sort_merge_enabled": false,
  "auto_sort_merge_source": "global",
  "auto_sort_merge_revision": 0
}
```

`auto_sort_merge_enabled` is the resolved preference. Clients compute the
effective value as `auto_merge_enabled && auto_sort_merge_enabled`. If the
override read fails, the status sends `auto_sort_merge_available: false` and
omits the other three fields, without making Auto-merge unavailable (`.14`).
`Service.Snapshot` fills the tuple, so every session-scoped producer that
serializes `QueueStatus` carries it. The producers to verify are
`handlers/queue_operations_handlers.go` (`publishStatusForIdentity`,
`publishIdentityStatus`), `orchestrator/event_handlers_agent.go`, and
`mcp/handlers/handlers.go` (`publishQueueStatusEvent`). Task-scoped recount
events carry no policy, as before.

New WebSocket action:

```text
message.queue.auto_sort_merge.set
request:  { task_id, session_id, session_incarnation_id, enabled }
response: { task_id, session_id, session_incarnation_id,
            auto_sort_merge_available, auto_sort_merge_enabled,
            auto_sort_merge_source, auto_sort_merge_revision,
            compacted_count }
```

The `message.queue.auto_merge.set` response gains `compacted_count`. Mutation
responses carry no status epoch or generation. The client does not apply them
to `QueueMeta`; the identity-bound refetch (and the published status) are the
only state sources, which keeps ordering on the existing
`status_epoch`/`status_generation` fields. Authorization, triplet validation,
and the non-enumerating session-not-found response follow the existing queue
actions.

## Frontend state

- `QueueStatus`, `QueueStatusChangedPayload`, and `QueueMeta` gain
  `autoSortMerge`, `autoSortMergeAvailable`, `autoSortMergeSource`, and
  `autoSortMergeRevision`. Wire types keep the snake-case names.
- `queueAutoMergeMeta` in `agent-session.ts` and `shouldPreserveSessionPolicy`
  and `shouldPreservePolicyRevision` in `session-slice.ts` are generalized to
  order each policy tuple on its own. Each tuple follows the existing rules:
  epoch/generation, irreversible `global` to `session`, and same-source
  revision (`.15`). Every `setQueueEntries` path that rebuilds metadata
  preserves the new tuple.
- `hooks/domains/session/use-queue-refetch.ts` `queueStatusMeta` projects all
  four sort fields from `message.queue.get`, only when present (no invented
  defaults). This is the authoritative path for initial load, reload,
  reconnect, and mutation recovery, because `useBooleanQueueAction` ignores
  the mutation result and refetches.
- `queue-api.ts` adds `setQueueAutoSortMerge`. `useQueue` exposes
  `autoSortMerge`, `autoSortMergeAvailable`, and `setAutoSortMerge` through
  `useBooleanQueueAction`, using the shared operation token, which disables all
  queue controls while a mutation is pending. A failure refetches and shows the
  localized error toast (`.12`).

## Queue header and settings UI

`QueueAutomationPills` renders a third bordered pill, **Auto-sort-merge**
(test id `queue-auto-sort-merge`), after Auto-merge in the same wrapping group.
The pill is:

```text
checked  = autoMerge && autoSortMerge
disabled = controlsDisabled || !autoMergeAvailable || !autoSortMergeAvailable || !autoMerge
```

When Auto-merge is OFF, the pill's accessible description and tooltip say
that it requires Auto-merge. When the policy is unavailable, it reuses the
existing unavailable tooltip. Coarse-pointer sizing reuses the existing pill
classes (`.4`, `.5`, `.16`).

`MessageQueueSettings` adds `AutoSortMergeToggleFields` (test id
`message-queue-auto-sort-merge-enabled`) below the Auto-merge row. While the
Auto-merge draft is OFF, the switch is disabled and renders OFF. Turning the
Auto-merge draft OFF resets the sort draft to the last saved value and clears
its dirty flag. The PATCH never includes `auto_sort_merge_enabled` while the
Auto-merge draft is OFF; otherwise it includes it only when that field is
dirty. A save response updates the saved baseline and only clears dirty
fields whose draft still equals the value that was sent, so an older response
cannot clobber a newer edit. Turning the Auto-merge draft back ON shows the
saved sort value (`.2`).

The existing Auto-merge notice (`messageQueueAutoMergeNotice` in
`system.json`, currently "This applies only to new admissions") is rendered in
the settings help and in the session Auto-merge pill tooltip. It is reworded:
global changes apply only to later messages, while turning a session's
Auto-merge ON with Auto-sort-merge ON compacts the pending queue immediately
(`.10`, `.13`). The pill tooltip uses the session wording. All copy goes
through `t()` in the `system` and `settings` namespaces for every shipped
locale (en, pseudo, ja, ko, pt-pt, zh-cn, and zh-hk/zh-tw generated with
`pnpm run i18n:zh-hant`), with no em dashes.

## Failure and recovery

- **Override read fails during admission:** sorted folds are skipped, ordinary
  Auto-merge still applies, and a structured warning is logged.
- **Override read fails during a status read:** the status reports
  Auto-sort-merge as unavailable, entries still reconcile, and the pill is
  disabled.
- **The compaction pass hits a CAS conflict or storage error:** the whole
  policy write rolls back. The client gets the error, refetches, and shows the
  prior queue and switch state.
- **Concurrent drain, Send Now, edit, or reorder:** these are ordered by the
  session lock. They win before or after the pass and never see a partial
  pass.
- **Stale identity:** session-not-found is returned and nothing changes.

## Security

Mutations reuse the existing queue authorization: any user with session access
can change the session switch, and only admins can change the global setting.
The browser sends only the identity triplet and a boolean. Sender identity
comes only from persisted `sender_task_id` metadata.

## Observability

Admission sorted folds log at debug level with `session_id`, `source_id`,
`target_id`, and `crossed` (the number of entries skipped). Compaction passes
log at info level with `session_id`, `compacted_count`, and `trigger`
(`auto_sort_merge` or `auto_merge`). Override-read failures log warnings. No
metric is added: this is a user preference, not a retry or availability loop.

## Verification strategy

- Pure tests of `sortedFoldTarget` and `isFoldBarrier` cover agent and user
  rows being crossed, workflow/server/move-task and hidden reserved barriers,
  sender-less agents, user sources, and the latest-only rule (A1 model x, A2
  model y, B1, then A3 model x stays at the tail).
- Service and repository tests run on memory, SQLite, and PostgreSQL.
  PostgreSQL cases require `KANDEV_TEST_POSTGRES_DSN`; without it
  `testutil.PostgresDSNFromEnv` skips them, so verification must check that
  the variable is set and record skips as unverified. Policy-write versus
  admission/compaction races use two repositories on two connections, not only
  the process-local gate.
- A combined-snapshot test pauses resolution across a concurrent two-field
  settings PATCH and asserts the pair is wholly old or wholly new. They
  cover admission A1, B1, A2 + B2; incompatible and edit-leased targets;
  compaction of A1, leased U1, A2 equal to admission (A1+A2, U1 unchanged);
  both session switches using the real settings loader and queue repository on
  one production-shaped single-connection SQLite writer pool;
  full-capacity sorted folds; staged-attachment `queue_full`; override-read
  failure; the policy-changed retry; compaction triggers from both switches;
  no compaction on global change or ON-to-ON writes; reserved-row
  preservation; and rollback.
- Settings tests cover the default OFF, a legacy value without the field, the
  revision bump, and startup hydration.
- Handler tests cover the action, the triplet, and the status fields.
- Frontend unit tests cover the API, hook, slice ordering, header pill
  dependency, and settings dependency.
- Playwright on desktop and `mobile-chrome` drives child-task messages through
  the mock agent's `message_task_kandev` directive. The phone scenario taps
  the pill, observes compaction and a later sorted admission, checks the
  disabled-OFF dependency and preference restoration, and reloads.

## Related decisions

- [Inherit Queue Auto-merge Until Overridden](../../../decisions/2026-09-04-inherit-queue-auto-merge-until-overridden.md)
- [Keep Queue Auto-run Server Owned](../../../decisions/2026-08-16-server-owned-queue-auto-run.md)
