---
id: coordinator-automatic-design
title: The first automatic action class design
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-29
last_updated: 2026-09-29
requirements:
  - REQ-COORDINATOR-INTEGRATION-004
  - REQ-COORDINATOR-INTEGRATION-007
  - REQ-COORDINATOR-AUTOMATIC-001
  - REQ-COORDINATOR-AUTOMATIC-002
  - REQ-COORDINATOR-AUTOMATIC-003
  - REQ-COORDINATOR-AUTOMATIC-004
---

# The first automatic action class System Design

## Purpose and boundaries

Phase 2 stores D17's per-coordinator action settings, writes the "What it
did" log, and offers undo. This design adds four things on top of them: the
rule that only `create_task` may be `automatic`, the eligibility computation
and class reviews that gate the raise, the automatic approval inside
`propose_task_kandev`, and the lowering on undo. It reads phase 2's settings
and log through the narrow interfaces below and does not redefine them. When
phase 2's names differ, the adapters in `internal/coordinator/phase2.go`
change; this contract does not.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-COORDINATOR-AUTOMATIC-001` | [Raisable classes](#raisable-classes) |
| `REQ-COORDINATOR-AUTOMATIC-002` | [Eligibility](#eligibility), [Class reviews](#class-reviews), [Raise gate](#raise-gate), [Screens](#screens) |
| `REQ-COORDINATOR-AUTOMATIC-003` | [Automatic approval](#automatic-approval) |
| `REQ-COORDINATOR-AUTOMATIC-004` | [Lowering](#lowering) |

## Phase 2 interfaces consumed

Phase 3 code depends on two narrow interfaces, implemented by adapters in
`internal/coordinator/phase2.go` over what phase 2 built:

```go
type ActionSettings interface {
    Setting(ctx context.Context, coordinatorID, class string) (Setting, error) // {Value, ChangedBy, ChangedAt}
    Lower(ctx context.Context, coordinatorID, class, reason string) error       // system write, logged
}

type DecisionLog interface {
    EarliestDecision(ctx context.Context, coordinatorID, class string) (time.Time, bool, error)
    Decisions(ctx context.Context, coordinatorID, class string, since, before time.Time) ([]Decision, error) // {ProposalID, Outcome, DecidedAt}
    UndoneTaskIDs(ctx context.Context, coordinatorID string, since, before time.Time) ([]string, error)
}
```

Phase 2 as built stores less than these interfaces name, so the adapters map
what exists and this design adds the rest, each as a small additive change
owned by task 09, except that the `coordinator_class_changes` table and its
index are created by task 01 in `store_phase2_schema.go`
([integration](integration.md#phase-2-touch-points)) and task 09 writes to it:

| Interface need | Phase 2 as built | Phase 3 provides |
| --- | --- | --- |
| `Setting.Value` | `policyFor(c)` (`policy_json`, `policy_revision`) | read as is |
| `Setting.ChangedBy`, `ChangedAt` | not stored per setting | table `coordinator_class_changes` below; the adapter reads the newest row for the class whose `to_value` equals the current value |
| Change validator | none | one call site in `SaveSettings`, inside `withCoordinatorLock`, and `Validate(p, phase3)` |
| `Lower` (system write) | none | `Service.LowerClass` below |
| Decided rows | `coordinator_activity` rows (outcome, authorization, `edited`, `created_at`) | the filter below |
| `approved_with_edits` | `edited` flag on the row | derived from it |
| `decided_at` | not on the proposal | the activity row's `created_at` |
| `UndoneTaskIDs` | `undone_at` and `target_task_id` on the approved row, set by `markUndone` in `undo.go` | the query below |
| `OnUndo` | none | a post-commit call added in `markUndone` |
| Locked reads and claim for the automatic section | `lockedCoordinatorRow` reads the row on a handle; `ClaimProposal` runs on `s.db` | `NewestClassChangeTx`, `CountAutomaticDecidedTx` and `ClaimProposalTx` ([Automatic approval](#automatic-approval)) |
| `returned` outcome, `automatic` authorization | not in the closed sets | added by [integration](integration.md#log-rows) |

`coordinator_class_changes` (append only, deleted with the coordinator and on
`workspace.deleted`, never pruned before the coordinator):

| Column | Type | Notes |
| --- | --- | --- |
| `id` | text primary key | UUID |
| `coordinator_id` | text not null | indexed with `class, changed_at desc` |
| `class` | text not null | one of the six actions |
| `from_value`, `to_value` | text not null | `requires_approval` or `automatic` |
| `changed_by` | text null | user id; null is the system (`LowerClass`) |
| `reason` | text null | set for a system change |
| `changed_at` | timestamp not null | |

`SaveSettings` inserts one row per class whose value changed, in the same
locked transaction as the policy write. `Service.LowerClass(ctx, coordinatorID,
class, reason)` opens `withCoordinatorLock`, returns nil when the class does
not read `automatic`, otherwise writes it `requires_approval`, bumps
`policy_revision`, inserts a row with null `changed_by` and the reason,
commits, and publishes `coordinator.updated` as a settings save does.

**Decided rows.** `Decisions` returns the `coordinator_activity` rows of the
coordinator with class `create_task`, outcome in (`approved`, `rejected`,
`returned`), authorization not `automatic`, and `created_at` in `[since,
before)`. `Outcome` is `approved_with_edits` when the row's outcome is
`approved` and `edited` is set, otherwise the row's outcome; `DecidedAt` is
`created_at`. A `failed` row is not a decision. An automatic approval, which
has authorization `automatic`, therefore counts toward none of `history_30d`,
`volume` and `unedited_rate`, before or after a lower and re-raise. A
manager's later decision on the same proposal (for example **Try again** on an
automatic approval that ended `failed`) is its own `requires_approval` row and
counts like any other. `EarliestDecision` is the minimum `created_at` under the
same filter. `UndoneTaskIDs` returns the `target_task_id` of this
coordinator's `create_task` `approved` rows (any authorization) whose
`undone_at` is in the window. Activity retention (400 days) covers the 30-day
window and the 24-hour lowering retry.

**`OnUndo`.** `markUndone` calls the coordinator's registered function after
its transaction commits, with the coordinator id and the undone task id. A
panic or error there is logged and never fails the undo; the backstop retry
of [Lowering](#lowering) covers it.

## Raisable classes

`internal/coordinator/automatic.go` holds `raisableClasses =
{"create_task"}`. Phase 2's `Validate(p)` refuses `automatic` for every action
with 400 `automatic_not_available` naming it; it becomes `Validate(p, phase3
bool)` and accepts `automatic` only for a class in `raisableClasses` while
phase 3 is effective, so every other class, including classes phase 2 or later
phases add, is refused with 400 naming the class
(`AC-COORDINATOR-AUTOMATIC-001.1`), because the set is a closed allowlist, not
a denylist.

## Eligibility

`Eligibility(ctx, coordinatorID, now) (Result, error)` for `create_task`
returns five conditions in this order, each `{Name, Met, Value}`:

| Name | Met when | Value |
| --- | --- | --- |
| `history_30d` | `EarliestDecision` exists and is at or before `now - 30d` | the earliest decision time |
| `volume` | `Decisions(now - 30d, now)` has at least 20 rows | the row count |
| `unedited_rate` | at least 90% of those rows are `approved` (integer math: `approved * 10 >= total * 9`) | the percentage, floored |
| `no_undo` | `UndoneTaskIDs(now - 30d, now)` contains no task created from this coordinator's proposals | the undone count |
| `reviewed_7d` | a class review for (coordinator, `create_task`) has `reviewed_at >= now - 7d` | the newest review time |

`Eligible` is true only when all five are met. Any interface error returns the
error, and every caller treats an error as not eligible
(`AC-COORDINATOR-AUTOMATIC-002.4`).

## Class reviews

`coordinator_class_reviews` in the coordinator store:

| Column | Type | Notes |
| --- | --- | --- |
| `id` | text primary key | UUID |
| `coordinator_id` | text not null | indexed with `class, reviewed_at` |
| `class` | text not null | `create_task` |
| `reviewed_by` | text not null | user id |
| `reviewed_at` | timestamp not null | |
| `window_start`, `window_end` | timestamp not null | the evidence window reviewed |
| `row_count` | integer not null | decided rows in that window at review time |

Deleted with the coordinator and on `workspace.deleted`.

Routes under `/api/v1/workspaces/:id/coordinators/:cid/`, phase 3 only:

| Route | Scope | Result |
| --- | --- | --- |
| `GET classes/create_task/eligibility` | `workspace.read` | `{eligible, conditions: [...], setting}` |
| `POST classes/create_task/reviews` | `workspace.manage` | 201 with the review; any other class is 400 naming `class` |

The review POST takes no fields: the body is empty or `{}`, and any field in
it is ignored. The server sets `reviewed_by` from the caller, `reviewed_at`
and `window_end` to now, `window_start` to now minus 30 days (the evidence
window, half-open `[window_start, window_end)`), and `row_count` to the
number of decided rows `Decisions` returns for that window. A log read error
returns 503 naming `decision_log` and stores nothing.

The coordinator guard refuses both write routes and the settings write for a
coordinator principal (`AC-COORDINATOR-AUTOMATIC-002.2`,
`AC-COORDINATOR-AUTOMATIC-004.3`).

## Raise gate

The change hook called once by `SaveSettings` inside `withCoordinatorLock`, for
`class == "create_task"` and `to == "automatic"` where the stored value was
not already `automatic`, runs `Eligibility` and returns a 409 error naming the
first unmet condition when not eligible; the settings write rolls back and
stores nothing. A save that leaves an already automatic class unchanged is not
checked. Lowering (`to == "requires_approval"`) is never
checked (`AC-COORDINATOR-AUTOMATIC-004.1`).

## Automatic approval

`coordinator_proposals` gains `decided_automatically integer not null default
0` and `automatic_at timestamp null`, both set once by the automatic claim's
`UPDATE` and never rewritten by stale-claim recovery or a later manager
approval, and `claimed_automatically integer not null default 0`, written by
every approve claim's `UPDATE` (1 by the automatic claim, 0 by a manager's
claim) and kept by stale-claim recovery, which re-uses the claim it
recovers. `decided_automatically` answers "was this proposal ever approved
automatically" (the 24-hour count); `claimed_automatically` answers "was the
current attempt, and so any task it created, automatic".

**The transaction-aware claim.** Phase 2 as built has `Store.ClaimProposal` and
`ClaimProposalRaw`, which execute through the store's `s.db`, so they cannot
join a caller's transaction. Phase 3 adds, in the manner of `CompleteProposalTx`
and `InsertProposalWith`, `Store.ClaimProposalTx(ctx, exec coordinatorExec, id,
token, finalSpec, decidedBy, now, automaticAt *time.Time)` and the matching
`ClaimProposalRawTx`. `ClaimProposal` and `ClaimProposalRaw` keep their
signatures and become one-line calls of the `Tx` forms with `s.db` and a nil
`automaticAt`, so every existing caller is unchanged and a manager's claim
still writes `claimed_automatically = 0` and never touches
`decided_automatically` or `automatic_at`. A non-nil `automaticAt` makes the
same `UPDATE` also set `decided_automatically = 1`, `claimed_automatically = 1`
and `automatic_at`. The conditional `WHERE id = ? AND status IN ('pending',
'failed')` is unchanged and its `matched` result is returned as before.

The approve service is split at the seam its manager path already has, with no
change of behaviour for that path: `prepareApproval(ctx, workspaceID,
proposal, edits) (ProposalSpec, error)` is `approvePending`'s
`buildCandidateSpec` and `validateProposalSpecFor` and writes nothing;
`finishClaim(ctx, workspaceID, coordinatorID, proposal, token, spec)` is
`claimAndProceed`'s tail (publish `coordinator.updated`, log,
`completeClaimedApproval`). `claimAndProceed` becomes `prepared spec ->
ClaimProposal -> finishClaim`. The automatic path adds
`Service.approveAutomatically(ctx, workspaceID, coordinatorID, proposal,
raiser)`, which takes the claim through `ClaimProposalTx` inside step 3 below
and runs `finishClaim` after that transaction commits.

In `propose_task_kandev`'s handler, after the phase 1 validation and insert
of the `pending` row:

1. Read the `create_task` setting and its `ChangedBy` through
   `ActionSettings.Setting`. Not `automatic` (or an error): return the phase 1
   result. This read is a cheap filter; step 3 decides.
2. Resolve the raiser from that read: its `ChangedBy` must be an active user
   who holds `workspace.manage` on the coordinator's workspace, checked
   through the same authorisation the approve route uses (with auth disabled,
   the synthetic admin passes as it does on every route). This runs before the
   lock, because it reads outside the coordinator tables. When the user is
   deleted, disabled, or no longer a manager, or the check errors, the
   proposal stays `pending`, the tool returns `{proposal_id, status:
   "pending", note: "automatic approval unavailable; a manager will decide"}`,
   and, except on a check error, the coordinator calls `LowerClass` with the
   reason "raising manager no longer a manager". Nothing is claimed and
   nothing counts toward the 10. Then run `prepareApproval` with no edits; an
   error there leaves the proposal `pending`, is logged at warn and returns
   the same unavailable note.
3. Call `Store.withCoordinatorLock(ctx, coordinatorID, fn)`, the per-coordinator
   lock `SaveSettings`, `LowerClass` and `InsertProposalWith` already take
   (`BEGIN IMMEDIATE` on SQLite, `SELECT ... FOR UPDATE` on the coordinator row
   on PostgreSQL), so a lower and this section exclude each other on both
   dialects and no separate automatic lock exists. `fn` uses only its
   `coordinatorExec` handle, as the helper's contract requires:
   1. Re-read the `create_task` value from the locked coordinator row
      (`lockedCoordinatorRow`) and its `ChangedBy` through the store's
      `NewestClassChangeTx`. When the value is no longer `automatic`, or
      `ChangedBy` differs from the raiser step 2 checked, write nothing and
      return the phase 1 result, because a lower or a re-raise that committed
      before the lock wins (`AC-COORDINATOR-AUTOMATIC-004.1`). A lower that
      commits after the lock takes effect for the next proposal; this one was
      decided under the setting it read.
   2. Count proposals with `decided_automatically = 1 AND automatic_at >=
      now - 24h` (`CountAutomaticDecidedTx`), whatever their status: an
      automatic approval that ended `failed` counts toward the 10, because the
      limit bounds automatic attempts. At 10 or more, write nothing and
      return `{proposal_id, status: "pending", note: "automatic limit
      reached; a manager will decide"}`
      (`AC-COORDINATOR-AUTOMATIC-003.2`).
   3. Claim with `ClaimProposalTx`, no edits, the raiser as the deciding user
      and `automaticAt = now`.
4. Outcomes of the section. Committed with `matched`: continue to step 5, with
   the lock already released. `matched` false (a manager approved or rejected
   the proposal between its insert and the lock): nothing was written, and the
   tool returns `{proposal_id, status}` with the row's re-read status, counting
   nothing. An error from any statement in `fn`, a failed commit, a cancelled
   context or a panic rolls the whole section back through the helper's
   deferred rollback: the proposal stays `pending`, nothing is counted,
   nothing is claimed, the error is logged at error, and the tool returns the
   unavailable note of step 2. A coordinator that no longer exists is
   `ErrNotFound`, answered as phase 1 answers it.
5. After the commit, run `finishClaim`: every phase 1 guarantee holds: the
   frozen spec, the pre-create eligible-step check, the idempotent create by
   external id, and no agent start (D15). The task is created under the
   raising manager's identity, outside the transaction. Return
   `{proposal_id, status, task_id}` with the resulting status: `approved`, or
   `failed` with the error, which leaves a normal failed card for a manager
   (`AC-COORDINATOR-AUTOMATIC-003.3`). A crash after the commit leaves the row
   `approving` with `claimed_automatically = 1`, which stale-claim recovery
   re-uses.

The approve path writes the `approved` row, or the `failed` row of a failed
attempt, with authorization `automatic` and actor the raising manager when the
claimed proposal has `claimed_automatically = 1`, so the row reads "Approved
automatically, raised by <manager>" (`AC-COORDINATOR-AUTOMATIC-003.4`,
[Log rows](integration.md#log-rows)). An improvement
proposal never reaches step 1: the automatic path is only in the
`propose_task_kandev` handler.

## Lowering

The coordinator registers the `OnUndo` function. When the undone task is the
`task_id` of one of the coordinator's proposals and the decision that created
that task was automatic (the proposal's `claimed_automatically = 1`; a
manager's approval of a proposal whose automatic approval ended `failed`
sets it to 0, so the task it creates is a manager's), it calls `LowerClass(ctx,
coordinatorID, "create_task", "undo of an automatic create")`. A failed lower
is retried by the backstop tick of [wake](wake.md#backstop), which visits every
coordinator with a proposal with `claimed_automatically = 1` whatever its
`autonomy_enabled` reads, because raising `create_task` does not require
autonomy. For each such coordinator whose `create_task` setting reads
`automatic`, it intersects `UndoneTaskIDs` over the last 24 hours with the
`task_id`s of its proposals with `claimed_automatically = 1`. A non-empty
intersection calls the same `Lower`. A setting that already reads
`requires_approval` is not written again, so the retry is idempotent. A
read error skips this coordinator's retry for the tick.

## Screens

In phase 2's permission settings for one coordinator (UI-06), on the May do
radio group described in [integration](integration.md#settings-layout):

- Every class other than `create_task` shows the Automatic option disabled
  with "Cannot be raised".
- `create_task` shows the five conditions as Met or Not met with their values,
  **Review the last 30 days** (opens phase 2's log filtered to this
  coordinator and class), and **Mark as reviewed** once the log view has been
  opened in this settings session. Its Automatic option is enabled only when
  eligible, and choosing it then saving with the page's Save is the raise;
  choosing Requires approval and saving is the lower. After a raise: "Raised
  to automatic by <you> at <time>" from the class change history.
- Copy goes through `t()` in six locales.

## Security

- The raise is a manager's settings write; the coordinator has no tool for
  settings, and the guard refuses the settings, review and eligibility-write
  routes for a coordinator principal.
- The automatic path runs only inside the coordinator's own propose call and
  uses the manager's recorded identity, so it grants the coordinator nothing a
  manager has not granted.

## Observability

`coordinator_automatic_approval_total{outcome}` (`approved`, `failed`,
`limited`, `raiser_invalid`) and `coordinator_automatic_lowered_total{reason}` counters, and an
info log for each raise refusal with the unmet condition.

## Related decisions

- [Coordinator phase 3: autonomy](../../../decisions/2026-09-29-coordinator-phase-3-autonomy.md)
- [Workspace coordinator in core](../../../decisions/2026-09-26-workspace-coordinator.md)
- [Plugin coordination platform](../../../decisions/2026-09-25-plugin-coordination-platform.md)
