---
id: coordinator-automatic-classes-design
title: Raisable action classes and eligibility settings design
status: draft
system: coordinator
owners:
  - kandev
created: 2026-11-07
last_updated: 2026-11-07
requirements:
  - REQ-COORDINATOR-AUTOMATIC-005
  - REQ-COORDINATOR-AUTOMATIC-006
  - REQ-COORDINATOR-AUTOMATIC-007
  - REQ-COORDINATOR-AUTOMATIC-008
---

# Raisable action classes and eligibility settings System Design

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-COORDINATOR-AUTOMATIC-005` | [Amendment: classes](#amendment-2026-11-07-resume-and-start_agent-classes) |
| `REQ-COORDINATOR-AUTOMATIC-006` | [Eligibility settings](#eligibility-settings) |
| `REQ-COORDINATOR-AUTOMATIC-007` | [Generalised automatic approval](#generalised-automatic-approval) |
| `REQ-COORDINATOR-AUTOMATIC-008` | [Lowering without undo](#lowering-without-undo) |

Section references to [Phase 2 interfaces consumed](automatic.md#phase-2-interfaces-consumed),
[Lowering](automatic.md#lowering) and the original design live in
[automatic](automatic.md).

## Amendment 2026-11-07: resume and start_agent classes

[automatic](automatic.md) describes the original `create_task`-only design.
Where this amendment differs, it governs. `raisableClasses` becomes `{create_task, resume,
start_agent}`; `Validate(p, phase3)` keeps the closed allowlist. `GetEligibility`,
`RecordClassReview`, the `/classes/:class/...` routes (`classField` check),
`ClassError`, the raise-gate hook in `SaveSettings`, `LowerClass` and the
backstop `LoweringDuty` take the class as a parameter instead of
`ActionCreateTask`. `coordinator_class_reviews.class` accepts the three values.

**Decided rows by class.** `create_task` and `resume`: the filter of
[Phase 2 interfaces consumed](automatic.md#phase-2-interfaces-consumed) with `action_class`
bound to the class. `start_agent`: the same filter without the `action_class`
term, joined to `coordinator_proposals` on `proposal_id` with `starts_agent =
1`. Ordering for every list is `created_at` ascending with `id` ascending as the
tiebreak. Rows with a null `proposal_id` never count for `start_agent`.

**The setting read.** `ActionSettings.Setting` returns `ClassSetting{Value,
ChangedBy, ChangedAt}`; this change adds `ChangeID`, the `id` of the newest
`coordinator_class_changes` row for the class whose `to_value` equals the
current value (`NewestClassChangeTx`: `changed_at` descending, `id` descending). A value of `automatic` with no such row is treated as not
automatic: the phase 1 result is returned and the gap is logged at error. The
raiser check (step 3), the locked re-read (step 6) and the lowering guards
compare `ChangeID`, not the user.

**The log filter.** `ListActivityParams.Class = start_agent` (accepted by
`validActivityClass` for the list only) selects `coordinator_activity` rows
joined to `coordinator_proposals` on `proposal_id` with `starts_agent = 1`,
any outcome, ordered `created_at` descending then `id` descending, with the
list's existing cursor and limit (`AC-COORDINATOR-AUTOMATIC-005.5`). The
summary counts keep one entry per stored `action_class`.

**The needed set.** `neededClasses(proposal)` returns `{kindAction(kind)}` plus
`start_agent` when `starts_agent` is set, ordered `kindAction` first. A kind
whose class is not raisable (`move`, `message`, `improvement`) yields no
automatic path.

## Eligibility settings

`Policy` gains an optional `Eligibility` object serialised in `policy_json`
beside `actions` (`policyVersion` unchanged; an absent object means defaults,
so existing rows need no migration):

| JSON field | Default | Bounds |
| --- | --- | --- |
| `evidence_window_hours` | 720 | 24..2160 |
| `min_decisions` | 20 | 3..500 |
| `unedited_floor_percent` | 90 | 50..100 |
| `review_max_age_hours` | 168 | 1..720 |
| `daily_limit` | 10 | 1..100 |

`parsePolicyMember` owns the structural checks (a non-object member, unknown
fields, a missing field, a non-integer literal) because `Validate(p Policy,
...)` receives typed integers; `Validate` checks the bounds. Both report
`PolicyFieldError{Field: "eligibility.<field>"}` (a non-object member reports
`eligibility`), always with code `invalid_setting`. The first error wins in
this order: every `policy.actions` error first (their existing order), then a
non-object member, unknown fields in ascending name order, missing fields in
table order, non-integer values in table order, then out-of-bounds values in
table order. The error maps in the settings layer, which the settings layer maps
to a `SettingsError` with code `invalid_setting` and `Field:
"policy.eligibility.<field>"` through a new `eligibilityFieldErr` (the existing
`policyFieldErr` prefixes `policy.actions.` and is not used for it), 400. With phase 3 not effective, `parsePolicyMember` refuses a non-null `eligibility`
member with 400 naming `policy.eligibility` (`AC-COORDINATOR-AUTOMATIC-006.11`),
and the settings response still returns the effective object (the class routes, including the eligibility GET, stay 404 without phase 3). The
member is parsed with `presentMember`: absent or JSON null keeps the stored
object; a present object must name all five fields, each a JSON integer
literal (decoded from `json.Number` and checked with no fraction or exponent),
else 400 naming the field; the object replaces the stored one whole. `EligibilitySettings`
replaces the constants `evidenceWindow`, `reviewMaxAge`, `eligibilityMinRows`,
`eligibilityApproved` and `automaticDailyLimit`; `Defaults()` holds the old
values. The unedited test is `total > 0 && approved * 100 >= total * floor`; the `total > 0` guard of the constant-based check is kept, so `unedited_rate` is unmet with no decided rows. Every policy
writer carries the stored `Eligibility` into the policy it writes: the settings
save when the member is absent or null, and `LowerClass`, which today rebuilds
`Policy{Version, Actions}` and must copy `stored.Eligibility` into `next`.
`policiesEqual` compares the effective `Eligibility` (the stored object, or
`Defaults()` when absent) as well as `Actions`, so an unchanged object, and an
explicit defaults object over an absent one, bump no revision and write
nothing (`AC-COORDINATOR-AUTOMATIC-006.7`); "revision" is `policy_revision`. Class reviews
(`AC-COORDINATOR-AUTOMATIC-006.8`): `window_start` is `now` minus the stored
evidence window; the review condition compares `reviewed_at` with `now` minus
the stored review max age at evaluation time; `RecordClassReview` runs
under `withCoordinatorLock`, reads the stored settings, counts the decided rows
and inserts the review in that one section. The lock helper allows only
transaction-bound reads and writes, so this change adds `InsertClassReviewTx` and `CountDecidedRowsTx(ctx, exec,
coordinatorID, class, since, before)`, the `Tx` form of the decided-row count
for the three class filters (the `start_agent` one with the `starts_agent` join);
the reader-pool `DecidedRows` stays for eligibility reads outside the lock (`AC-COORDINATOR-AUTOMATIC-006.10`). A failure to take the lock, count or insert answers the existing 503 `DecisionLogUnavailableError` and records nothing (the section is one transaction). The review
never calls `LowerClass` or re-evaluates eligibility
(`AC-COORDINATOR-AUTOMATIC-005.4`). The review button and the log filter label
read the configured window. The write goes
through `SaveSettings` under `withCoordinatorLock` with the existing
`policy_revision` bump. The audit is a new table, `coordinator_eligibility_changes`
(`id`, `coordinator_id`, `changed_by`, `from_json`, `to_json`,
`policy_revision`, `changed_at`), created with the other coordinator tables by
`CREATE TABLE IF NOT EXISTS` (a new table, so no `ADD COLUMN` migration), one
row appended in the same transaction as the policy write when the effective
object changed (`changed_by` null when `decidingUserID` is empty; no
`coordinator_class_changes` row is written for it); no route reads it (`AC-COORDINATOR-AUTOMATIC-006.9`). The
conversation reset in `SaveSettings` (`resetConversation`) runs only when
actions, watches or projects changed, so an eligibility-only save leaves the
conversation alone. The settings DTO gains `eligibility` (effective values) beside `Actions`. The raise hook reads the settings from the
stored row before the save is applied (`AC-COORDINATOR-AUTOMATIC-006.3`). The
automatic path reads `daily_limit` from `lockedCoordinatorRow`, so a concurrent
settings save and an automatic claim exclude each other. `GET .../eligibility`
adds `settings`. Condition names are unchanged. Tightening never touches a
stored `automatic` class (`AC-COORDINATOR-AUTOMATIC-005.4`); no code path
derives a raise from a settings change.

## Generalised automatic approval

`TryAutomaticApproval` is called from `propose_task_kandev` as today and from
`propose_resume_kandev` (`proposeKindHandler` gains the same
`applyAutomaticApproval` call for kind `resume`; `move`, `message` and
`improvement` do not call it). Both handlers call it only for a proposal they
created: `ProposeKind` already returns `deduplicated` for an open target
proposal, and a deduplicated result returns to the caller unchanged with no
automatic attempt (`AC-COORDINATOR-AUTOMATIC-007.5`); `ProposeTask` has no
deduplication and always creates, so its handler always attempts. Order of the
section:

1. Phase 3 on, decision-tasks wired, and `neededClasses` non-empty and all
   raisable; otherwise return the phase 1 result. Read every needed class's
   `ClassSetting` (value, `ChangedBy`, `ChangeID`); any not `automatic` returns
   the phase 1 result.
2. Pause check (existing gate): pending with the paused note.
3. Raiser validation for each distinct `ChangedBy` as today. An invalid raiser
   lowers the class it raised (`raiserLoweredReason`) guarded by the
   `ChangeID` it checked, counts `raiser_invalid`, and leaves the proposal
   pending with the unavailable note. The claim's actor is the raiser of the
   class whose change row has the newest `changed_at`, class name ascending as
   the tiebreak.
4. If `neededClasses` contains `resume` or `start_agent`: `AdmitAction(ctx,
   coordinatorID)`, a new function that splits `Admit` after its spend check and runs the `Admit` checks in `AdmitReadOnly` mode and
   stops after the cost-ceiling check (autonomy on, containment, spend
   measured, ceiling not reached), skipping the conversation and cooldown
   checks because the call happens inside the coordinator's own turn. It
   starts no turn and writes and counts nothing. The held token is the
   admission `Reason`, except `read_error` when the admission `Detail` is
   `read_error` (a failed coordinator, autonomy or containment read). A spend
   read error is already `Reason = spend_unmeasured` with an empty `Detail`
   and keeps that token. A
   failure returns pending with `automatic action held (<token>); a manager
   will decide`, increments `held{reason}` with the same token, counts nothing
   toward the limit, and does not lower.
5. `prepareApproval` (create path) or `prepareKindApproval` (kind path); error
   leaves pending with the unavailable note.
6. Inside `withCoordinatorLock`: re-read each needed class from the locked row
   and its `ChangeID` from `NewestClassChangeTx`; any not `automatic`, with no
   change row, or with a `ChangeID` other than the one read in step 1 returns
   the phase 1 result. Re-read `paused_at` under the lock; paused leaves the
   proposal pending with the paused note. Count with `CountAutomaticDecidedTx`
   (all classes, 24h) against the locked row's `daily_limit`; at or over the
   limit, pending with the limit note. Then claim with `automaticAt` set; the
   automatic claim's `WHERE` is `id = ? AND status = 'pending' AND
   decided_automatically = 0`, so two concurrent callers on one proposal get
   exactly one decider, a failed or manager-touched proposal is never claimed
   automatically, and the loser returns the re-read status.
7. After commit, `finishClaim` (create path) or `executeKind` (kind path). For
   `resume` the kind's `Execute` runs under the raiser identity as for a
   manager's approval (including the cold-launch goroutine). The result is
   `{proposal_id, status}` plus `task_id` when the kind created one, and `note`
   on pending outcomes.

**The kind-path seam (`resume`).** A resume proposal is approved by
`approveKind` (`approve_kinds.go`) through `claimKind`, `executeKind`,
`completeKind`, `failKind` and `settleStaleKind`, not through `prepareApproval`
or `finishClaim`, so steps 5 and 7 name their kind-path equivalents:

- `claimKind` splits at its claim like the create path: `prepareKindApproval`
  (`ValidateEdits` and `recheckPolicy`, writes nothing) and the claim.
  `ClaimProposalRawTx(ctx, exec, id, token, finalJSON, decidedBy, now,
  automaticAt *time.Time)` already exists with the `automaticAt` parameter
  (nil for manager claims through `ClaimProposalRaw`, which stay unchanged). A
  non-nil `automaticAt` already sets `decided_automatically`,
  `claimed_automatically` and `automatic_at`; this change narrows its `WHERE`
  from `status IN ('pending','failed')` to the one in step 6 only when
  `automaticAt` is non-nil, for both the create path (`ClaimProposalTx`) and
  the kind path. With a nil `automaticAt` (every manager claim) the `WHERE` is
  unchanged, so a manager can still approve, with or without edits, a `failed`
  proposal. Step 6's locked section calls it, then
  `executeKind` runs after commit with `claim.ApprovedBy` set to the raiser.
- `completeKind`, `failKind`, `failedKindRow` and `settleStaleKind` stop
  hardcoding `AuthRequiresApproval`: they read the proposal row's
  `claimed_automatically` and write `AuthAutomatic` and the claim's
  `ApprovedBy` as actor when it is 1, and set `UnattendedTurnID` from the
  originating turn as the create path does (`automaticTurnFrom`). With it 0
  they behave as today; in particular `settleStaleKind` keeps its current
  empty actor for a manager's stale claim and uses the deciding user recorded
  on the proposal row only when `claimed_automatically = 1`.
- Failed rows record `edited`. `failProposalStore` (create) and
  `failedKindRow` (kind) set `Edited` from the claimed proposal exactly as the
  approved rows do (`specEdited` on the final against the raw spec for the
  create path, the claim's `TextEdited` for the kind path); `settleStaleKind`
  does the same from the stored final and raw specs. An automatic claim never
  edits, so its rows read `edited = false`. The log summary's unedited counts
  keep reading `edited` for `approved` rows only, so a failed row with
  `edited` set changes no eligibility count.
- The failure hook is one function, `afterAutomaticFailure(ctx, attempt)`,
  where `attempt` is the snapshot `{coordinatorID, proposal, neededClasses,
  automaticAt}` taken from the proposal row inside the settling transaction
  that wrote `failed`, only when that row had `claimed_automatically = 1`. It
  runs after that transaction commits and never re-reads the proposal, so a
  later manager retry cannot suppress it. It is called from every site that
  settles an automatic attempt `failed`: `finishClaim`'s failure branch,
  `failApproval` (the stale create re-claim through `reclaimStaleAndProceed`),
  `failKind` and `settleStaleKind`. It lowers per [Lowering without
  undo](#lowering-without-undo). An execute deadline and a stale `resume`
  settle both end `failed` with `outcome_unknown` and therefore lower; a stale
  `resume` claim is never executed again, while a stale automatic
  `create_task` claim keeps its existing recovery (re-use of the claim,
  idempotent create by external id).
- The create path stops refusing `starts_agent`: `TryAutomaticApproval` no
  longer returns the phase 1 result when `StartsAgent` is set, and
  `approveAutomatically` passes the proposal's `starts_agent` into
  `finishClaim` (so `EligibleStartingStep` and the auto-start metadata apply)
  instead of the hardcoded false. It is reached only when `start_agent` is
  among the needed classes that read `automatic`.

Step 3's raiser validation runs over every distinct `ChangedBy` without
stopping at the first invalid one, lowering each invalid raiser's class and
emitting one `raiser_invalid` count and one unavailable note per proposal; a
check error lowers nothing for that raiser. Step 6 compares, per needed class,
the `ChangeID` against the one read in step 1, so a lower followed by a
re-raise, even by the same manager, is detected. The admission read (step 4)
is made once before the lock and not re-read under it
(`AC-COORDINATOR-AUTOMATIC-007.2`); Pause is read at step 2 and again under the
lock at step 6, as [pause](pause.md#the-precondition) requires. Admission
reasons are the existing closed `Reason` set; `read_error` is added as a
separate value of the held counter's `reason` label, and the note shows the
token only.

`start_agent` proposals are `create_task` with `starts_agent` only, since
`move` is not raisable. `recheckPolicy` is unchanged: an `automatic` value
`Allows` the action as `requires_approval` does for approval purposes.

## Lowering without undo

`undoableClass` is unchanged: `resume` rows are never undoable, and a
`starts_agent` create stays undoable as a `create_task` row. `LowerClass` gains
a guard parameter, `LowerGuard{After *time.Time, RaiseID string}`. Inside the
coordinator lock, after the existing "not automatic, write nothing" check, it
reads the newest `coordinator_class_changes` row for the class with `to_value
= automatic`; when `After` is set it writes nothing unless that row's
`changed_at` is strictly before `After`, and when `RaiseID` is set it writes
nothing unless that row's `id` equals it. A manager's own lower through the
settings save is unguarded. The guard has no age limit; the 24-hour window
belongs to the backstop's evidence queries only (`AC-COORDINATOR-AUTOMATIC-008.5`). Each trigger passes its date as `After`: the
proposal's `automatic_at` for a failure, the decided row's `created_at` for an
override, the activity row's `undone_at` for an undo (not `created_at`: undo
updates an existing approved row); the raiser-invalid lowering passes the
`ChangeID` it validated as `RaiseID`.

The failure trigger is `afterAutomaticFailure` of [the kind-path
seam](#generalised-automatic-approval): for the snapshot's `neededClasses` it
lowers `resume` for a resume proposal and `start_agent` for a `starts_agent`
proposal, reason `automatic action failed`, leaving `create_task` as the
existing rules leave it. A failed lower is retried by `LoweringDuty`, which
gains per-class evidence, each limited to rows dated strictly after the
`changed_at` of the class's newest change to `automatic` (a date equal to it
counts as preceding, so a trigger that preceded a re-raise never lowers it)
and to the last 24 hours, while the setting still reads `automatic`: for
`resume`, a `failed` activity row of class `resume` with authorization
`automatic`, dated by its proposal's `automatic_at`; for `start_agent`, a
`failed` activity row with authorization `automatic` whose proposal (joined on
`proposal_id`) has `starts_agent = 1`, since those rows carry the kind's class
`create_task`, dated the same way; for the manager override, a `rejected` or `returned` decided row, or a decided
row with `edited` set and outcome `approved` or `failed`, of a proposal that
needed the class, dated by its `created_at`; for the undo intersection, `undone_at` via `UndoneTaskIDs`
intersected with `AutomaticClaimTaskIDs`, and for `start_agent` further joined
to proposals with `starts_agent = 1` (a `create_task` undo needs no such join). The backstop's visit set gains
`CoordinatorIDsWithRaisedClasses` (coordinators with any
`coordinator_class_changes` row whose `to_value = automatic`), added to the
autonomy, active-turn and automatic-claim sets of
[wake](wake.md#backstop), because a class can be raised, and a lowering fail,
on a coordinator whose autonomy is off. The retry of a raiser-invalid lowering
does not exist: the lowering is logged and counted on
`coordinator_automatic_lower_failed_total{trigger}` (see Observability), and the next
automatic attempt re-validates the raiser. Undoing an automatic `starts_agent`
create lowers `create_task` and `start_agent`, reason `undo of an automatic
create`; `Service.afterUndo` (the post-commit call of `markUndone`) reads the
proposal's `starts_agent` for this. The manager override trigger lives in the
reject, return and approve-with-edits paths of the approve service: when the
proposal needs `resume` or `start_agent` and that class reads `automatic`,
after the decision is settled it calls `LowerClass` (for reject and return, after the decision commits; for an approve, after the settling commit of the `approved` or `failed` row, never after the claim) with `manager overrode an
automatic class`; a lower error is logged and never fails the decision. The `ActionSettings.Lower` interface method changes with it (it takes the `LowerGuard` and returns `(bool, error)`), as do its test doubles. `LowerClass` returns
`(lowered bool, err error)`: `lowered` is true only when it wrote a class
change, so a guarded or already-lowered no-op is false. A true result is
named in the decision response as `lowered_class` so the page can say so
(`AC-COORDINATOR-AUTOMATIC-008.3`). A decision on a proposal that needs
neither class, or on one whose class does not read `automatic`, triggers
nothing.

`no_undo` for `resume` and `start_agent` counts, in the evidence window:
activity rows of the class (for `start_agent`, proposals with `starts_agent`)
with outcome `failed` and authorization `automatic`, plus
`coordinator_class_changes` rows for the class with `to_value =
requires_approval`, null `changed_by` and a reason other than `manager
overrode an automatic class` and `raising manager no longer a manager`.
Value: that count. Manager lowers (non-null `changed_by`) are not counted.

## Screens (amendment)

The permission settings gain, per coordinator, an "Automatic eligibility"
group with the five numeric fields, their bounds as helper text, and the
daily-limit copy "one limit per coordinator across all classes". The raise
list for `resume` and `start_agent` mirrors `create_task`, with
`history_30d` and `reviewed_7d` labelled from the settings. `resume` and
`start_agent` show a note that an automatic run waits for containment and the
cost ceiling. All copy goes through `t()` in the seven locales.

## Containment note

Today the V2 Development coordinator's autonomy admission fails
(`executor_isolated`, `auth_enabled`). An automatic `resume` or `start_agent`
is held until an isolated executor and `KANDEV_FEATURES_AUTH=true` exist. That
is infrastructure, tracked separately, and not changed here.

## Observability (amendment)

`coordinator_automatic_approval_total{outcome}` gains `held`.
`coordinator_automatic_lowered_total{reason}` keeps the reason string as its
label and gains the strings `automatic action failed` and `manager overrode an
automatic class`. A new `coordinator_automatic_held_total{reason}` counts
admission holds by the closed admission reason set plus `read_error`. A new
`coordinator_automatic_lower_failed_total{trigger}` counts lowerings that
failed, with `trigger` one of `raiser_invalid`, `automatic_failure`,
`manager_override`, `undo` and `settings`. No coordinator or proposal
id is a label.

## Related decisions

- [Coordinator phase 3: autonomy](../../../decisions/2026-09-29-coordinator-phase-3-autonomy.md)
- [Workspace coordinator in core](../../../decisions/2026-09-26-workspace-coordinator.md)
- [Plugin coordination platform](../../../decisions/2026-09-25-plugin-coordination-platform.md)