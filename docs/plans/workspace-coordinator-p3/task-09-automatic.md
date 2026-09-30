---
id: "09-automatic"
title: "The first automatic class: create_task"
status: built
wave: 3
depends_on:
  - "01-flag-schema-settings"
  - "04-wake-recorder-backstop"
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-AUTOMATIC-001
  - REQ-COORDINATOR-AUTOMATIC-002
  - REQ-COORDINATOR-AUTOMATIC-003
  - REQ-COORDINATOR-AUTOMATIC-004
  - REQ-COORDINATOR-INTEGRATION-004
  - REQ-COORDINATOR-INTEGRATION-007
acceptance_criteria:
  - AC-COORDINATOR-AUTOMATIC-001.1
  - AC-COORDINATOR-AUTOMATIC-001.2
  - AC-COORDINATOR-AUTOMATIC-002.1
  - AC-COORDINATOR-AUTOMATIC-002.2
  - AC-COORDINATOR-AUTOMATIC-002.3
  - AC-COORDINATOR-AUTOMATIC-002.4
  - AC-COORDINATOR-AUTOMATIC-003.1
  - AC-COORDINATOR-AUTOMATIC-003.2
  - AC-COORDINATOR-AUTOMATIC-003.3
  - AC-COORDINATOR-AUTOMATIC-003.4
  - AC-COORDINATOR-AUTOMATIC-004.1
  - AC-COORDINATOR-AUTOMATIC-004.2
  - AC-COORDINATOR-AUTOMATIC-004.3
  - AC-COORDINATOR-INTEGRATION-004.3
  - AC-COORDINATOR-INTEGRATION-007.2
system_design:
  - ../../specs/coordinator/system-design/automatic.md
  - ../../specs/coordinator/system-design/integration.md
---

# Task 09: The First Automatic Class, create_task (WP-11)

## Summary

Lets a manager raise `create_task` to `automatic` for one coordinator once its
own log earns it, approves such proposals automatically through the phase 1
approve service, and lowers the class on an undo of an automatic create.
Starts only when phase 2 is merged and the ADR records the 30-day log review
confirming `create_task`.

## In scope

- `internal/coordinator/phase2.go`: adapters for `ActionSettings` and
  `DecisionLog` over phase 2's code, using the mapping and the additions
  phase 2 lacks: the `coordinator_class_changes` writes in `SaveSettings`'
  locked transaction and `Service.LowerClass`, the decided-row filter over
  `coordinator_activity` (authorization not `automatic`), `UndoneTaskIDs`
  from the approved rows' `undone_at`, and the post-commit `OnUndo` call in
  `markUndone` (`undo.go`)
  ([Phase 2 interfaces consumed](../../specs/coordinator/system-design/automatic.md#phase-2-interfaces-consumed)).
- `policy.go` and `settings.go`: `Validate(p, phase3)` accepts `automatic` for
  `create_task` only while phase 3 is effective and keeps
  `automatic_not_available` for every other action; one change-hook call in
  `SaveSettings` inside `withCoordinatorLock`.
- The approve path writes the `approved` and `failed` rows of a claim with
  `claimed_automatically = 1` with authorization `automatic` and actor the
  raising manager, and its copy in six locales
  ([Log rows](../../specs/coordinator/system-design/integration.md#log-rows)).
- `internal/coordinator/automatic.go`: the closed `raisableClasses`, the
  change hook (409 naming the first unmet condition; other classes are
  refused by `Validate` with 400 naming the class), `Eligibility`, the class review store and routes, the automatic
  branch in `propose_task_kandev`: the raiser check and
  `prepareApproval` before the lock, then one `withCoordinatorLock` section
  (the lock `SaveSettings` and `LowerClass` take, no separate automatic lock)
  holding the tx-handle setting re-read, the 10-per-24h count on
  `automatic_at` whatever the outcome, and the claim through the new
  `Store.ClaimProposalTx` and `ClaimProposalRawTx` (`coordinatorExec`
  parameter and `automaticAt`; `ClaimProposal` and `ClaimProposalRaw` become
  calls of them with `s.db` and nil), `NewestClassChangeTx` and
  `CountAutomaticDecidedTx`, the approve service split into `prepareApproval`,
  `claimAndProceed` and `finishClaim`, and `Service.approveAutomatically`
  running `finishClaim` after the commit
  ([Automatic approval](../../specs/coordinator/system-design/automatic.md#automatic-approval));
  `decided_automatically`, `claimed_automatically` and `automatic_at` on the
  claim (`claimed_automatically` 0 on a manager's claim), the adapter's
  exclusion of log rows decided by the automatic path, the
  server-computed review window and row count, and `OnUndo` lowering with the
  backstop retry, filled into task 04's step 2.4 hook
  ([automatic](../../specs/coordinator/system-design/automatic.md)).
- `internal/coordinator/no_turn_start_test.go`: append the automatic
  approval row to `noTurnStartPaths`
  ([copilot](../../specs/coordinator/system-design/copilot.md#attended-only)).
- Web: in phase 2's May do section, the Automatic option (disabled with "Cannot
  be raised" for every other action; enabled for `create_task` only while
  eligible; the raise is choosing it and saving), the eligibility list,
  **Review the last 30 days**, **Mark as reviewed** and the raised record
  ([Screens](../../specs/coordinator/system-design/automatic.md#screens),
  [Settings layout](../../specs/coordinator/system-design/integration.md#settings-layout)).
- Copy in six locales.

## Out of scope

- Any other raisable class; improvement proposals (never automatic).

## ASCII UI preview

See [plan UI-06](plan.md#ascii-ui-previews).

```text
Create a card   (o) Requires approval  ( ) Automatic (disabled)
  Not met  90% approved without edits (85%)  ...
  [Review the last 30 days] [Mark as reviewed]   Automatic option disabled
Merge           Cannot be raised
```

## Acceptance

- Setting any class other than `create_task` to `automatic` is 400 naming it,
  also with phase 3 effective; `create_task` is 409 naming the first unmet of the five conditions, each
  tested at its boundary (29 and 30 days, 19 and 20 rows, 89% and 90%, one
  undo, 7 days); a log read error refuses; lowering is never checked; review
  and settings writes are refused to readers and a coordinator principal.
- A raised coordinator's valid proposal is approved automatically with every
  phase 1 approve guarantee and no agent start, returns `approved` with the
  task id, logs a row with authorization `automatic` and the raising manager as actor
  (a failed attempt's row too); the eleventh in
  24 hours stays `pending` with the limit note, also when earlier automatic
  approvals ended `failed`; a failed approval is left `failed` for a manager
  and keeps its `automatic_at` through stale-claim recovery.
- An automatic approval written inside a propose call while an unattended turn
  is open carries its `unattended_turn_id` (from task 05's
  `currentUnattendedTurn`, not from the actor); the same approval outside a
  turn, or while a manager's request is open, carries none
  (`AC-COORDINATOR-INTEGRATION-004.1`).
- With the raiser deleted, disabled or no longer a manager, the proposal stays
  `pending` with the unavailable note, nothing is claimed or counted, and the
  class is lowered with its reason; a raiser check error leaves it `pending`
  without lowering.
- The Automatic option of every other action stays disabled with "Cannot be
  raised", and for `create_task` it is enabled only for a manager while
  eligible (`AC-COORDINATOR-INTEGRATION-007.2`). A raise and a lower each
  store a `coordinator_class_changes` row with the manager (or null and the
  reason for `LowerClass`) and bump `policy_revision`; the raised record
  reads the newest row.
- A manager's approval of a proposal whose automatic approval failed writes a
  `requires_approval` row and every other row keeps its authorization
  (`AC-COORDINATOR-INTEGRATION-004.3`); the raise's log reads exclude rows
  with authorization `automatic`.
- Automatic approvals are not decided rows: after a lower and re-raise, 20
  automatic approvals plus 5 manager decisions fail `volume`.
- A review POST with any body stores `window_end` = now, `window_start` =
  now minus 30 days and `row_count` from the log, ignoring client fields; a
  log read error is 503 and stores nothing.
- Undoing a task an automatic approval created lowers the class at once and
  logs the reason. A failed lower is retried by the backstop's step 2.4 hook
  on the next tick, also for a coordinator whose autonomy is off, and a
  retry against a setting already `requires_approval` writes nothing.
  Undoing a task
  created by a manager's approval of a proposal whose automatic approval had
  ended `failed` does not lower, and that manager approval counts as a
  decided row.
- The `noTurnStartPaths` row for the automatic approval, run through
  `TestCoordinatorConversationNoTurnStart`, asserts that an automatic
  approval sends no prompt and starts no agent on the conversation or the
  created task.
- Two processes proposing concurrently at 9 automatic approvals in 24 hours
  on PostgreSQL produce exactly one tenth automatic approval (`-race` and a
  two-connection PostgreSQL test); a lower committed between step 1's read
  and the lock leaves the proposal `pending` and unclaimed. The claim joins
  the caller's transaction: a forced error after `ClaimProposalTx` inside the
  section, a failed commit and a cancelled context each leave the proposal
  `pending`, unstamped and uncounted and return the unavailable note; a
  proposal a manager rejected before the lock (`matched` false) writes nothing
  and returns its current status; `ClaimProposal`'s existing tests pass
  unchanged and a manager's claim writes `claimed_automatically = 0` and never
  touches `decided_automatically` or `automatic_at`.

## Verification

```bash
cd apps/backend && go test ./internal/coordinator/... -run 'Automatic|Eligibility|ClassReview|Phase2|NoTurnStart' -count=1
cd apps/backend && go test ./internal/coordinator/... -run 'Automatic' -race -count=1
cd apps/web && pnpm test -- app/settings/workspace app/coordinator
cd apps/web && pnpm run typecheck && pnpm run i18n:check
cd apps/web && pnpm e2e:run tests/coordinator/automatic.spec.ts
```

## Risks

- The e2e test needs 30 days of log rows: seed them through phase 2's test
  fixture, never by weakening the history condition.
