---
id: "01-shared-interface"
title: "Phase 2 shared interface"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-COORDINATORS-007
  - REQ-COORDINATOR-PERMISSIONS-001
  - REQ-COORDINATOR-PERMISSIONS-003
  - REQ-COORDINATOR-PROPOSAL-KINDS-001
  - REQ-COORDINATOR-ACTIVITY-LOG-001
acceptance_criteria:
  - AC-COORDINATOR-COORDINATORS-007.1
  - AC-COORDINATOR-COORDINATORS-007.3
  - AC-COORDINATOR-PERMISSIONS-001.1
  - AC-COORDINATOR-PERMISSIONS-003.1
  - AC-COORDINATOR-PROPOSAL-KINDS-001.6
  - AC-COORDINATOR-ACTIVITY-LOG-001.3
  - AC-COORDINATOR-ACTIVITY-LOG-001.4
  - AC-COORDINATOR-ACTIVITY-LOG-001.6
system_design:
  - ../../specs/coordinator/system-design/coordinators.md
  - ../../specs/coordinator/system-design/shared-interface.md
  - ../../specs/coordinator/system-design/permissions.md
  - ../../specs/coordinator/system-design/activity-log.md
  - ../../specs/coordinator/system-design/standing-orders.md
  - ../../specs/coordinator/system-design/goals.md
  - ../../specs/coordinator/system-design/proposal-kinds.md
---

# Task 01: Phase 2 Shared Interface (WP-6/7)

## Summary

Land the contract every other phase-2 work order builds against: the
`features.coordinatorPhase2` flag, every new table and column, the Go types
and route shapes, the policy value with its phase-1 default, the proposal
`kind`, `resetConversation`, and the typed web client. No enforcement and no
screen.

## In scope

- Flag: registry entry, `profiles.yaml` (`prod`/`dev` false, `e2e` true),
  restart-required, client `features.coordinatorPhase2`, and the single
  `phase2 := coordinator && coordinatorPhase2` value passed to the
  coordinator service and the MCP and executor wiring
  ([coordinators design](../../specs/coordinator/system-design/coordinators.md#phase-2)).
  Follow `/runtime-feature-flags`.
- Store: `coordinators.policy_json`, `policy_revision`, `watch_scope`;
  `coordinator_watches`; `coordinator_activity`;
  `coordinator_standing_orders` with its `last_applied_at` column (starts
  null, only ever raised by task 05's `MarkApplied` helper); `coordinator_goals`
  with its active partial unique index; proposal columns `kind`, `target_task_id`,
  `standing_order_ids`, `starts_agent`, `outcome_json` and
  `coordinator_proposals_open_target`. Deletion of the new rows in the
  coordinator delete and workspace-deletion transactions. Upgrade tests from
  the phase-1 schema on SQLite and PostgreSQL.
- `policy.go`: `Policy`, `Setting`, `Action`, `PhaseOnePolicy`, `ParsePolicy`
  (NULL reads as the phase-1 policy, a missing action as `denied`, per the
  result table),
  `Allows`, `Validate`. `WatchSet` load. `Service.Policy(ctx, coordinatorID)`
  (the phase-3 read).
- Coordinator GET and list carry `policy`, `policy_revision` and `watches`
  while `phase2` is on (`AC-COORDINATOR-PERMISSIONS-001.1`, `003.1`).
- Proposals carry `kind` (existing rows `create_task`), `target_task_id`,
  `standing_order_ids`, `starts_agent` and `outcome` (the parsed
  `outcome_json`; `AC-COORDINATOR-PROPOSAL-KINDS-001.6`). The phase-1 list filter
  `kind = 'create_task'` while `phase2` is off, applied also to `GET
  proposals/:pid`, approve and reject, so a non-create id is 404 and nothing
  is claimed or run (`AC-COORDINATOR-COORDINATORS-007.3`).
- `resetConversation(tx, coordinatorID)` in the service: clear
  `conversation_task_id`, increment `config_revision` (always), archive after commit
  ([permissions design](../../specs/coordinator/system-design/permissions.md#conversation-reset)).
- `internal/coordinator/activity.go`, the one log writer every later work
  order calls: `Record(tx, row)` in the caller's transaction, a failed
  insert failing the caller (`AC-COORDINATOR-ACTIVITY-LOG-001.6`), and
  `RecordRefusal(...)` with 60-second coalescing under the per-coordinator
  lock and the `unknown` class (`001.3`); rows are only ever marked undone
  or counted (`001.4`)
  ([design](../../specs/coordinator/system-design/activity-log.md#refusals)).
- Store methods (no routes) for activity insert, `MarkUndone`, standing-order
  reads (`ActiveStandingOrders`) and goal reads (`ActiveGoal`, `LastMetGoal`),
  so later work orders share one store surface. The exact Go signatures,
  the `coordinatorExec` transaction handle, `PolicyView`, `WatchSet`, the
  `NewService` option and the wire field names are fixed in the
  [shared interface](../../specs/coordinator/system-design/shared-interface.md#shared-interface)
  section; implement them as written.
- The `phase2` kind predicate on every store read a flag-off phase-1 path
  makes (list, by-id, decision-route read, startup pass, stale-claim sweep,
  `get_coordinator_item_kandev` by-id read), and `CountOpenProposals` (reader pool) and
  `CountOpenProposalsTx` with `phase2` (`InsertProposal` passes it to the Tx form) so the cap and
  `open_proposals` agree. Task 04 extends the sweep
  for the other kinds and keeps the parameter.
- Deletion order: children before the coordinator, with the per-coordinator
  lock first on PostgreSQL, and `coordinator_activity(workspace_id)`.
  Workspace deletion removes every phase-2 table by its own `workspace_id`
  (after locking the workspace's coordinators in id order on PostgreSQL); stall
  rows are deleted only by `workspace_id`.
- `Store.withCoordinatorLock` (created here) and `QueryContext` on
  `coordinatorExec`; the `CompleteProposalTx`, `FailProposalTx`,
  `RejectProposalTx` and `InsertProposalWith` variants (the phase-1 methods
  wrap them unchanged); with `phase2` on the service runs those writes and
  `Record` inside `withCoordinatorLock`, and a deleted coordinator goes
  through `settleWriteRace` (complete, fail) or `claimRaceResult` (reject);
  `InsertProposalWith` takes a `pre` dedupe hook that runs before the cap
  count and can short-circuit
  ([design](../../specs/coordinator/system-design/shared-interface.md#transaction-bound-proposal-writes)).
- `ActivityRow`, validation (`ErrInvalidActivity`), id and timestamp
  assignment, 1,000-rune truncation and the `coordinator_activity_rows_total`
  counter, all in `InsertActivity`.
- Typed client `lib/api/domains/coordinator-api.ts`: types and functions for
  settings, activity, summary, undo, standing orders, goal and setup routes
  as the designs define them; with `phase2` off they are never called.

## Out of scope

- Routes other than the GET and list additions (tasks 02, 03, 05, 07, 12).
- Guard, registration and auto-approval changes (task 02); the guard's
  `RecordRefusal` call is task 02's.
- Any screen.

## Acceptance

- With `phase2` off, every phase-1 test passes with no change other than the
  added trailing `phase2=false` argument at its direct `CountOpenProposals`,
  `InsertProposal`, `ListProposals`, `GetProposal`, `ListApprovingClaimedBefore`
  and `ReclaimStale` call sites (`CompleteProposal`, `FailProposal` and
  `RejectProposal` are unchanged), and
  stored phase-2 data survives a flag off and on cycle.
- With `phase2` off, `GET`, approve and reject of a stored resume, message
  or move proposal return 404; its row, status and claim are unchanged and
  no executor runs.
- A phase-1 coordinator reads as the phase-1 policy and watching `all`.
- A phase-1 proposal reads as `create_task`.

## Verification

```bash
cd apps/backend && go test ./internal/coordinator/... ./internal/persistence/... ./internal/runtimeflags/... ./internal/mcp/...
cd apps/web && pnpm exec vitest run lib/api/domains/coordinator-api.test.ts
cd apps/web && pnpm run typecheck
```

Tests: store conformance and upgrade on both dialects; `ParsePolicy` with
NULL, a partial map and garbage; the flag-off list filter hides a stored
`resume` proposal and `open_proposals` excludes it
(`AC-COORDINATOR-COORDINATORS-007.3`); the registry completeness test for
the new flag (`007.1`); a fault-injected row insert fails the caller's
transaction (`001.6`); 10 concurrent refusals give one row with count 10
and an unnamed action records as `unknown` (`001.3`); a scan of the
package sources finds every `UPDATE coordinator_activity` setting only the
undo-marker or count columns (`001.4`).

Added tests: `ParsePolicy` on every row of its result table; `Allows` on an
unknown action; `Service.Policy` with `phase2` on and off (revision 0 when off), for a
missing coordinator and for an unreadable stored policy (full all-`denied`
view, nil error); `LoadWatchSet` order, empty `selected`, missing coordinator and
a failed query; `resetConversation` clears, increments and returns the old
task id, the after-commit archive runs once and never with an empty id, a
rolled-back transaction changes nothing, and it matches a config-changing PATCH on `conversation_task_id`,
`config_revision` and `updated_at` (the PATCH's own config column excluded), and
a PATCH without a config-field change does not increment `config_revision`; coordinator GET and list carry `policy`, `policy_revision` and
`watches` with `phase2` on and none with it off, and read an unreadable stored
policy as all `denied`; the flag-off cap and `open_proposals` cases and the
flag-off sweep, startup-pass and by-id cases of the design; the four-case
flag dependency and its client derivation; 25-vs-26 open counts on both
sides; a refusal coordinator deleted mid-write writes nothing; `RecordRefusal` with
an empty reason code or an out-of-set class and `InsertActivity` with an
invalid enum write nothing and return `ErrInvalidActivity`; a coordinator GET
or list whose watch or policy read fails is 500; unknown `watch_scope` reads
as `selected`; `ActiveGoal` and `LastMetGoal` for a missing coordinator return
`nil, nil`; refusal
cutoff at 59 and 61 seconds; a 1,001-rune detail stored as 1,000; delete
ordering and concurrent `Record` against `DeleteCoordinator` on both
dialects; an unknown stored kind (phase2 on) lists raw, approve and reject are 500 with
nothing claimed or written and the row still counts toward the cap; a PUT carrying the all-`denied` view over an
unreadable stored policy still writes and increments the revision; a
`withCoordinatorLock` `fn` statement commits and no source passes `s.db.` inside
it; complete, fail, reject and propose write their activity row atomically with
the status write and a `Record` fault rolls the status back; a deleted
coordinator under complete yields the `settleWriteRace` 404 and under reject the
`claimRaceResult` 409; `pre` returning an existing proposal at 25 open rows returns it
with no insert and no cap error; workspace deletion
removes orphans a phase-1 binary left in every phase-2 table; a phase-1 statement set run against the phase-2 schema; the upgrade
test from both starting schemas of the design; `ActiveStandingOrders` and goal
reads with none, several and a failed query; the client test covers only the
routes whose bodies the designs define.

## Likely files

- `apps/backend/internal/runtimeflags/registry.go`, `profiles.yaml`
- `apps/backend/internal/coordinator/store.go`, `policy.go`, `watches.go`,
  `activity.go`, `toolprofile.go`, `service.go`, `models.go`, `dto.go`
- `apps/backend/internal/persistence/requiredstores/catalog.go` and upgrade
  testdata
- `apps/web/lib/api/domains/coordinator-api.ts`, `apps/web/lib/types/`

## Dependencies

- Phase 1 merged (the coordinator package and store exist). Built ahead of
  that merge, it starts from the phase-1 integration branch after phase-1
  [task 12](../workspace-coordinator/task-12-review-follow-ups.md) has
  passed review: that task adds `config_revision`, which
  `resetConversation` increments, a migration this one follows, and the
  stale-claim sweep task 04 extends.

## Risks

- The partial unique indexes must be written in a form both dialects accept;
  the upgrade test runs both.
