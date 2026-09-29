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
  (NULL and missing actions read as the phase-1 policy and `denied`),
  `Allows`, `Validate`. `WatchSet` load. `Service.Policy(ctx, coordinatorID)`
  (the phase-3 read).
- Coordinator GET and list carry `policy`, `policy_revision` and `watches`
  while `phase2` is on (`AC-COORDINATOR-PERMISSIONS-001.1`, `003.1`).
- Proposals carry `kind` (existing rows `create_task`), `target_task_id`,
  `standing_order_ids`, `starts_agent`, `outcome_json`
  (`AC-COORDINATOR-PROPOSAL-KINDS-001.6`). The phase-1 list filter
  `kind = 'create_task'` while `phase2` is off, applied also to `GET
  proposals/:pid`, approve and reject, so a non-create id is 404 and nothing
  is claimed or run (`AC-COORDINATOR-COORDINATORS-007.3`).
- `resetConversation(tx, coordinatorID)` in the service: clear
  `conversation_task_id`, increment `config_revision`, archive after commit
  ([permissions design](../../specs/coordinator/system-design/permissions.md#conversation-reset)).
- `internal/coordinator/activity.go`, the one log writer every later work
  order calls: `Record(tx, row)` in the caller's transaction, a failed
  insert failing the caller (`AC-COORDINATOR-ACTIVITY-LOG-001.6`), and
  `RecordRefusal(...)` with 60-second coalescing under the per-coordinator
  lock and the `unknown` class (`001.3`); rows are only ever marked undone
  or counted (`001.4`)
  ([design](../../specs/coordinator/system-design/activity-log.md#refusals)).
- Store methods (no routes) for activity insert, standing-order reads
  (`ActiveStandingOrders`) and goal reads, so later work orders share one
  store surface.
- Typed client `lib/api/domains/coordinator-api.ts`: types and functions for
  settings, activity, summary, undo, standing orders, goal and setup routes
  as the designs define them; with `phase2` off they are never called.

## Out of scope

- Routes other than the GET and list additions (tasks 02, 03, 05, 07, 12).
- Guard, registration and auto-approval changes (task 02); the guard's
  `RecordRefusal` call is task 02's.
- Any screen.

## Acceptance

- With `phase2` off, every phase-1 test passes unchanged, and stored phase-2
  data survives a flag off and on cycle.
- With `phase2` off, `GET`, approve and reject of a stored resume, message
  or move proposal return 404; its row, status and claim are unchanged and
  no executor runs.
- A phase-1 coordinator reads as the phase-1 policy and watching `all`.
- A phase-1 proposal reads as `create_task`.

## Verification

```bash
make -C apps/backend test PKG=./internal/coordinator/...
make -C apps/backend test PKG=./internal/persistence/...
make -C apps/backend test PKG=./internal/runtimeflags/...
cd apps/web && pnpm test -- lib/api/domains/coordinator-api.test.ts
cd apps/web && pnpm run typecheck
```

Tests: store conformance and upgrade on both dialects; `ParsePolicy` with
NULL, a partial map and garbage; the flag-off list filter hides a stored
`resume` proposal and `open_proposals` excludes it
(`AC-COORDINATOR-COORDINATORS-007.3`); the registry completeness test for
the new flag (`007.1`); a fault-injected row insert fails the caller's
transaction (`001.6`); 10 concurrent refusals give one row with count 10
and an unnamed action records as `unknown` (`001.3`); the store exposes no
update other than the undo marker and the count (`001.4`).

## Likely files

- `apps/backend/internal/runtimeflags/registry.go`, `profiles.yaml`
- `apps/backend/internal/coordinator/store.go`, `policy.go`, `watches.go`,
  `service.go`, `types.go`
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
