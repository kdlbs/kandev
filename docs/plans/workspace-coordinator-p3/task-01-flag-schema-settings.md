---
id: "01-flag-schema-settings"
title: "Phase 3 flag, schema and autonomy settings"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-WAKE-004
  - REQ-COORDINATOR-SPEND-001
  - REQ-COORDINATOR-INTEGRATION-001
acceptance_criteria:
  - AC-COORDINATOR-WAKE-004.1
  - AC-COORDINATOR-WAKE-004.3
  - AC-COORDINATOR-WAKE-004.4
  - AC-COORDINATOR-SPEND-001.1
  - AC-COORDINATOR-SPEND-001.2
  - AC-COORDINATOR-SPEND-001.3
  - AC-COORDINATOR-INTEGRATION-001.1
system_design:
  - ../../specs/coordinator/system-design/wake.md
  - ../../specs/coordinator/system-design/spend.md
  - ../../specs/coordinator/system-design/integration.md
---

# Task 01: Phase 3 Flag, Schema And Autonomy Settings (WP-11)

## Summary

Lands the shared interface every later work order builds on: the
`features.coordinatorPhase3` toggle, every phase 3 table and column, the
autonomy and ceiling PATCH fields with their interlock, the supersede on
autonomy off, and one named registration function per later work order.
Starts only after G3 is met.

## In scope

- `internal/runtimeflags/registry.go` and root `profiles.yaml`: the flag, per
  `/runtime-feature-flags` (prod and dev `"false"`, e2e `"true"`, restart
  required), with the registry/profile/frontend completeness tests.
- `internal/backendapp/coordinator.go`: compute "phase 3 effective" once, as
  `features.coordinator`, `features.coordinatorPhase2` and
  `features.coordinatorPhase3` all on
  ([integration](../../specs/coordinator/system-design/integration.md#effective-condition));
  empty named registration functions `registerCoordinatorContainment`,
  `...Spend`, `...Wake`, `...Delivery`, `...Relay`, `...Reply`,
  `...Automatic`, `...Improvements`.
- `internal/coordinator/store.go`: `coordinators.autonomy_enabled`,
  `coordinators.cost_ceiling_subcents`; `coordinator_proposals.reply_text`, `reply_delivered_at`, `reply_delivery_claimed_at`,
  `in_reply_to`, `decided_automatically`, `claimed_automatically`,
  `automatic_at`; tables `coordinator_wakes`,
  `coordinator_unattended_turns` (with the partial unique index,
  `session_turn_id`, `start_ceiling_subcents` and `stop_requested_at`), `coordinator_unattended_denials`,
  `coordinator_class_reviews`, `coordinator_class_changes`,
  `coordinator_pending_changes`; phase 2's `coordinator_proposals.kind`
  column is reused, not added; the phase 2 additions
  `coordinator_activity.unattended_turn_id` (nullable, no foreign key), the
  activity outcome `returned`, the authorization `automatic` and the
  activity-only class `improvement`
  ([integration Log rows](../../specs/coordinator/system-design/integration.md#log-rows));
  deletion with
  the coordinator and on `workspace.deleted`; retention in the startup pass
  ([wake Store](../../specs/coordinator/system-design/wake.md#store)).
- PATCH fields `autonomy_enabled` and `cost_ceiling_usd` with the integer
  parser ([spend Ceiling](../../specs/coordinator/system-design/spend.md#ceiling)),
  the interlock, no `config_revision` change, and the supersede on autonomy
  off in the same transaction, which first takes the per-coordinator
  [wake lock](../../specs/coordinator/system-design/wake.md#wake-lock)
  (`WithWakeLock(ctx, coordinatorID, fn)`, the helper tasks 04 and 05 reuse); GET and list carry both fields; the guard
  refuses the fields from a coordinator principal.
- Typed client fields in `apps/web/lib/api/domains/coordinator-api.ts`, and an
  empty Autonomy section in the coordinator settings page rendered only while
  the flag is on.

## Out of scope

- Any subscriber, ticker or route beyond PATCH and GET (tasks 02 to 10).
- The Autonomy section's controls (task 06).

## Acceptance

- With the flag on, PATCH sets and clears the ceiling and toggles autonomy;
  autonomy on without a ceiling, and clearing the ceiling while autonomy is
  on, are 400 naming `cost_ceiling`; invalid amounts (a JSON number, `0`,
  `0.001`, `10000.01`, `1e3`) are 400; `config_revision` and
  `conversation_task_id` are unchanged; readers get 403 and a coordinator
  principal is refused.
- Turning autonomy off marks every pending wake `superseded` in the same
  transaction and leaves an open turn row untouched. `WithWakeLock`
  serialises two concurrent callers for one coordinator and not for two
  coordinators (SQLite, and PostgreSQL under `KANDEV_TEST_POSTGRES_DSN` with
  `-race`). The delivery half of `AC-COORDINATOR-WAKE-004.3`, and its
  clause that an open turn keeps its ceiling stop, recovery and settle, are
  tested in task 05.
- With any of the three flags off (`features.coordinator`,
  `features.coordinatorPhase2`, `features.coordinatorPhase3`), the new PATCH
  fields are ignored, every phase 3 route is 404, the Autonomy section is
  hidden, and stored rows survive an off/on cycle
  (`AC-COORDINATOR-INTEGRATION-001.1`). The upgrade conformance test adds the
  activity column and the class change table to a phase 2 database and
  replays the migration.

## Verification

```bash
cd apps/backend && go test ./internal/coordinator/... -run 'Store|Patch|Phase3|Ceiling|Autonomy' -count=1
cd apps/backend && go test ./internal/runtimeflags/... ./internal/profiles/... -count=1
make -C apps/backend lint
cd apps/web && pnpm run typecheck && pnpm test -- lib/api/domains/coordinator-api hooks/domains/settings/use-coordinator
```

## Risks

- Postgres partial index syntax differs from SQLite: covered by the store's
  upgrade conformance on both dialects.
