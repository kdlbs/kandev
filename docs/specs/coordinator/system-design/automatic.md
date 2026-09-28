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

```go
// Settings, owned by phase 2 (D17 storage).
type ActionSettings interface {
    Setting(ctx context.Context, coordinatorID, class string) (Setting, error) // {Value, ChangedBy, ChangedAt}
    RegisterChangeValidator(func(ctx context.Context, coordinatorID, class, from, to string) error)
    Lower(ctx context.Context, coordinatorID, class, reason string) error       // system write, logged
}

// Log, owned by phase 2 ("What it did").
type DecisionLog interface {
    EarliestDecision(ctx context.Context, coordinatorID, class string) (time.Time, bool, error)
    Decisions(ctx context.Context, coordinatorID, class string, since, before time.Time) ([]Decision, error) // {ProposalID, Outcome, DecidedAt}
    UndoneTaskIDs(ctx context.Context, coordinatorID string, since, before time.Time) ([]string, error)
    OnUndo(func(ctx context.Context, coordinatorID, taskID string))
}
```

`Outcome` is one of `approved`, `approved_with_edits`, `rejected`,
`returned`. A **decided row** is a manager's decision: the adapter keys the
filter on the log row itself, dropping every row whose decider is the
automatic path (the adapter records a decision with the proposal's
`claimed_automatically` at the time of that decision), so automatic approvals count toward none of
`history_30d`, `volume` and `unedited_rate`, before or after a lower and
re-raise. A manager's later decision on the same proposal (for example
**Try again** on an automatic approval that ended `failed`) is its own log
row with a manager decider and counts like any other.
`EarliestDecision` applies the same filter. When phase 2's log does not distinguish edits, the adapter
derives `approved_with_edits` from the proposal row: `final_spec_json`
differs from `spec_json` in title, description, workflow, step or repository.

## Raisable classes

`internal/coordinator/automatic.go` holds `raisableClasses =
{"create_task"}`. The validator registered with
`ActionSettings.RegisterChangeValidator` refuses `to == "automatic"` for any
other class with a 400 error naming the class
(`AC-COORDINATOR-AUTOMATIC-001.1`), including classes phase 2 or later phases
add, because the set is a closed allowlist, not a denylist.

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

The same registered validator, for `class == "create_task"` and `to ==
"automatic"`, runs `Eligibility` and returns a 409 error naming the first
unmet condition when not eligible. Phase 2's settings write surfaces it
unchanged and writes nothing. Lowering (`to == "requires_approval"`) is never
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
current attempt, and so any task it created, automatic". In `propose_task_kandev`'s handler, after the phase 1 validation and insert
of the `pending` row:

1. Read the `create_task` setting. Not `automatic` (or an error): return the
   phase 1 result. This read is a cheap filter; step 2 decides.
2. Open one transaction and take the coordinator's automatic lock in it:
   `pg_advisory_xact_lock(hashtextextended('coordinator_automatic:' ||
   coordinator_id, 0))` on PostgreSQL; on SQLite the store's single writer
   serialises the transaction. The lock is held until the claim commits, so
   steps 2 to 4's count, re-read and claim are one critical section across
   processes. Inside it, re-read the `create_task` setting and its
   `ChangedBy` through `ActionSettings.Setting`; when it is no longer
   `automatic`, commit nothing and return the phase 1 result, because a
   lower that committed before this read wins
   (`AC-COORDINATOR-AUTOMATIC-004.1`). A lower that commits after it takes
   effect for the next proposal; this one was decided under the setting it
   read. Then count proposals with
   `decided_automatically = 1 AND automatic_at >= now - 24h`, whatever their
   status: an automatic approval that ended `failed` counts toward the 10,
   because the limit bounds automatic attempts. At 10 or more,
   return `{proposal_id, status: "pending", note: "automatic limit reached; a
   manager will decide"}` (`AC-COORDINATOR-AUTOMATIC-003.2`).
3. Resolve the raiser from the setting re-read in step 2: its `ChangedBy`
   must be an active user who
   holds `workspace.manage` on the coordinator's workspace, checked through
   the same authorisation the approve route uses (with auth disabled, the
   synthetic admin passes as it does on every route). When the user is
   deleted, disabled, or no longer a manager, or the check errors, the
   proposal stays `pending`, the tool returns `{proposal_id, status:
   "pending", note: "automatic approval unavailable; a manager will decide"}`,
   and, except on a check error, the coordinator calls `ActionSettings.Lower`
   with the reason "raising manager no longer a manager". Nothing is claimed
   and nothing counts toward the 10.
4. Call the phase 1 approve service function (the one behind the approve
   route, below its HTTP authorisation) with no edits, the setting's
   `ChangedBy` as the deciding user and a flag that sets
   `decided_automatically = 1`, `claimed_automatically = 1` and
   `automatic_at` in the claim's `UPDATE`,
   which runs inside step 2's transaction; the lock is released when it
   commits, before the task create. Every phase 1
   guarantee holds: the claim, the frozen spec, the pre-create eligible-step
   check, the idempotent create by external id, and no agent start (D15).
   The task is created under the raising manager's identity.
5. Return `{proposal_id, status, task_id}` with the resulting status:
   `approved`, or `failed` with the error, which leaves a normal failed card
   for a manager (`AC-COORDINATOR-AUTOMATIC-003.3`).

Phase 2's log records the decision as it records any approval; the adapter
passes `claimed_automatically` and `decided_by` so the row reads "Automatic,
raised by <manager>" (`AC-COORDINATOR-AUTOMATIC-003.4`). An improvement
proposal never reaches step 1: the automatic path is only in the
`propose_task_kandev` handler.

## Lowering

The coordinator registers `DecisionLog.OnUndo`. When the undone task is the
`task_id` of one of the coordinator's proposals and the decision that created
that task was automatic (the proposal's `claimed_automatically = 1`; a
manager's approval of a proposal whose automatic approval ended `failed`
sets it to 0, so the task it creates is a manager's), it calls `ActionSettings.Lower(ctx,
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

In phase 2's permission settings for one coordinator (UI-06):

- Every class other than `create_task` shows "Cannot be raised" in place of
  a raise control.
- `create_task` shows the five conditions as Met or Not met with their values,
  **Review the last 30 days** (opens phase 2's log filtered to this
  coordinator and class), then **Mark as reviewed** once the log view has been
  opened in this settings session, and **Raise to automatic**, enabled only
  when eligible. After a raise: "Raised to automatic by <you> at <time>" with
  **Lower**.
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
