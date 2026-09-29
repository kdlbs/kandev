---
id: "01-flag-schema-settings"
title: "Phase 3 flag, schema and autonomy settings"
status: done
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
- `internal/coordinator/store.go` (types and queries) and the schema file below: `coordinators.autonomy_enabled`,
  `coordinators.cost_ceiling_subcents`; `coordinator_proposals.reply_text`, `reply_delivered_at`, `reply_delivery_claimed_at`,
  `in_reply_to`, `decided_automatically`, `claimed_automatically`,
  `automatic_at`; tables `coordinator_wakes`,
  `coordinator_unattended_turns` (with the partial unique index,
  `session_turn_id`, `start_ceiling_subcents` and `stop_requested_at`), `coordinator_unattended_denials`,
  `coordinator_class_reviews`, `coordinator_pending_changes`; phase 2's
  `coordinator_proposals.kind` column is reused, not added; the activity
  outcome `returned`, the authorization `automatic` and the activity-only
  class `improvement`
  ([integration Log rows](../../specs/coordinator/system-design/integration.md#log-rows));
  deletion with
  the coordinator and on `workspace.deleted`; retention in the startup pass
  ([wake Store](../../specs/coordinator/system-design/wake.md#store)). Phase 2
  as built keeps `createTablesSQL` to the three phase 1 tables and adds every
  later column by `ALTER TABLE`, so each phase 3 column on an existing table
  is one more entry of `phase2ColumnMigrations` and each new table and index
  one more statement of `phase2TablesSQL` and `phase2IndexesSQL`, all in the
  file named next, never in `createTablesSQL`.
- `internal/coordinator/store_phase2_schema.go`, the file phase 2's additive
  schema lives in: the column `coordinator_activity.unattended_turn_id`
  (nullable, no foreign key) as one `phase2ColumnMigrations` entry,
  `{"coordinator_activity.unattended_turn_id", "ALTER TABLE coordinator_activity
  ADD COLUMN unattended_turn_id TEXT"}`; the table `coordinator_class_changes`
  ([automatic](../../specs/coordinator/system-design/automatic.md#phase-2-interfaces-consumed)
  for its columns) as `CREATE TABLE IF NOT EXISTS` in `phase2TablesSQL`; and
  in `phase2IndexesSQL` `CREATE INDEX IF NOT EXISTS
  idx_coordinator_class_changes_class ON coordinator_class_changes
  (coordinator_id, class, changed_at DESC)`. `migratePhase2` already runs
  columns, then tables, then indexes, each replayable, so no migration
  function is added.
- PATCH fields `autonomy_enabled` and `cost_ceiling_usd` with the integer
  parser ([spend Ceiling](../../specs/coordinator/system-design/spend.md#ceiling)),
  the interlock, no `config_revision` change, and the supersede on autonomy
  off in the same transaction, which first takes the per-coordinator
  [wake lock](../../specs/coordinator/system-design/wake.md#wake-lock)
  (`WithWakeLock(ctx, coordinatorID, fn)`, the helper tasks 04 and 05 reuse); GET and list carry both fields; the guard
  refuses the fields from a coordinator principal.
- Contract details that would otherwise be invented, all specified in the
  designs: the PATCH field rules, post-commit `Kick` and `autonomy_changed`
  (a nil-safe `Service` field `kick func(ctx, coordinatorID string) error` with `SetKick`, no interface of its own)
  ([integration](../../specs/coordinator/system-design/integration.md#autonomy-patch-and-deletion));
  the `WithWakeLock` contract, including the shared `takeWakeLock` step the
  autonomy-off PATCH uses, its lock order and missing-row result
  ([wake](../../specs/coordinator/system-design/wake.md#wake-lock)); deletion
  of every phase 3 table by coordinator, with denials through their turn, in
  `DeleteCoordinator` and `DeleteWorkspaceState`, and
  `Store.PruneWakeState`, wired as the first step of the hook `registerCoordinatorWake` returns; the eight functions all take the signature of `registerCoordinatorSubscribers`; the activity DTO's `unattended_turn_id` (omitted while phase 3 is not effective)
  ([integration](../../specs/coordinator/system-design/integration.md#autonomy-patch-and-deletion)); the columns
  of `coordinator_unattended_denials`
  ([containment](../../specs/coordinator/system-design/containment.md#unattended-permissions));
  the eight registration functions' signatures and call gate
  ([integration](../../specs/coordinator/system-design/integration.md#effective-condition)).
  `coordinator_class_changes` is created here and only written by task 09.
- `internal/persistence/requiredstores/catalog.go`: the `coordinator` entry's
  `RequiredTables` gains the phase 3 tables (`coordinator_wakes`,
  `coordinator_unattended_turns`, `coordinator_unattended_denials`,
  `coordinator_class_reviews`, `coordinator_pending_changes`,
  `coordinator_class_changes`).
- Typed client fields in `apps/web/lib/api/domains/coordinator-api.ts` (both
  optional, present only while phase 3 is effective), one hook computing
  "effective" from the three feature reads, and the Autonomy entry of the
  coordinator page's Sections row with an empty body
  ([integration](../../specs/coordinator/system-design/integration.md#settings-layout)),
  its `sectionAutonomy` and `sectionAutonomyHelp` copy in all six locales,
  shown only while phase 3 is effective.

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
  transaction (also when the row already read off) and leaves an open turn row
  untouched. `WithWakeLock` serialises two concurrent callers for one
  coordinator on SQLite and on PostgreSQL; on PostgreSQL only, it does not
  serialise callers for two different coordinators (the SQLite writer
  connection serialises every write, so no independence is asserted there).
  Both run under
  `KANDEV_TEST_POSTGRES_DSN` with `-race`. A missing coordinator returns
  `ErrNotFound` and writes nothing; `fn` returning an error rolls back. The delivery half of `AC-COORDINATOR-WAKE-004.3`, and its
  clause that an open turn keeps its ceiling stop, recovery and settle, are
  tested in task 05.
- `autonomy_enabled` other than `true` or `false` (including `null`) is 400
  naming `autonomy_enabled`; an absent key leaves the column unchanged;
  a changed autonomy or ceiling publishes `autonomy_changed` and calls `Kick`
  once after commit, an unchanged value publishes nothing and calls nothing,
  and a `Kick` that returns an error or panics does not change the 200. Malformed
  JSON is 400 before the scope check (as in phase 2); a wrongly typed
  `cost_ceiling_usd` never fails binding. The activity DTO carries
  `unattended_turn_id` only while phase 3 is effective.
- `PruneWakeState` deletes delivered/superseded wakes older than 30 days and
  turns finished more than 90 days ago (denials first) and keeps pending wakes
  and open turns; a second run at the same `now` deletes nothing; a prune
  error is logged at warn and startup continues. While phase 3 is not effective GET and list carry neither
  key and an invalid `cost_ceiling_usd` returns 200 and stores nothing.
- Deleting a coordinator, and deleting its workspace, removes its rows from
  every phase 3 table, denials first, with the flag on or off.
- With any of the three flags off (`features.coordinator`,
  `features.coordinatorPhase2`, `features.coordinatorPhase3`), the new PATCH
  fields are ignored, the eight registration functions are not called, the
  Autonomy section is hidden, and stored rows survive an off/on cycle
  (`AC-COORDINATOR-INTEGRATION-001.1`). The upgrade conformance test, on SQLite and on
  PostgreSQL, starts from a phase 2 database, applies every phase 3 column,
  table and index (including the partial unique index on open unattended
  turns, which it proves by a second open insert failing), replays the
  migration with no error, and checks the `RequiredTables` entry.
  Observation of "no wake, no backstop, no turn, every phase 3 route 404"
  belongs to the tasks that add those things
  ([integration](../../specs/coordinator/system-design/integration.md#effective-condition)).

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

## Build results

Files changed: the flag (`config.go`, `runtimeflags/registry.go`, `profiles.yaml`, web `features/types.ts`), the six phase 3 tables and columns (`store_phase2_schema.go`, `requiredstores/catalog.go`), settings and wake code under `internal/coordinator/` (`autonomy_settings.go`, `store_wake.go`, `store.go`, `service.go`, `dto.go`, `handlers.go`, `activity*.go`, `events.go`, `models.go`, `store_workspace_delete.go`), wiring in `backendapp/coordinator.go` and `services.go`, and the web hook, Autonomy section entry and six locales.

Conductor rulings applied: R3-1 (`WithPhase3` and `Phase3Enabled`, wired beside `WithPhase2`; `phase3Effective` is the single AND of the three flags), R3-2 (eight package-level `registerCoordinator*` vars, appended only when phase 3 is effective), R3-3 (`Service.PruneWakeState` returns `(turns, wakes)` and is called by the wake hook only through the Service; a prune failure is logged and does not block startup), R3-4, R3-5, R3-6.

R3-4 correction for wake.md: the wake lock takes `pg_advisory_xact_lock` then the row `FOR UPDATE` on Postgres. On SQLite `lockCoordinatorRow` keeps its existing behavior (no `FOR UPDATE`; `BEGIN IMMEDIATE` provides the serialisation), so wake.md's SQLite statement is wrong. To port to #4044.

Deviation: `coordinator_activity.unattended_turn_id` is applied through a new `phase2LateColumnMigrations` step (after `phase2TablesSQL`, before `phase2IndexesSQL`) instead of the column list, because on a phase 1 database the table does not exist until the tables step.

Column definitions come from the specs; where silent they match phase 2's nearest column.

E2E decision: none added. The only UI change is an empty Autonomy entry behind a default-off flag; it is covered by the editor-page and hook Vitest tests.

Receipts:
- `go test -race ./internal/coordinator/...` (SQLite) and the Postgres legs (`KANDEV_TEST_POSTGRES_DSN`, throwaway instance): pass.
- `go test -race ./internal/backendapp/... ./internal/persistence/... ./internal/runtimeflags/... ./internal/profiles/... ./internal/mcp/...`: pass, including `storeconformance` and the Postgres boot test. Two backendapp startup-conflict tests fail only when `TMPDIR` is not the resolved `/private/var/...` path (macOS symlink) and pass with it.
- `go run ./cmd/sqlguard ./internal` and `golangci-lint` on the touched packages: clean.
- `internal/common/config` `TestConfigSourceLoadsStableYAMLFields` (`launcher.healthTimeoutMs = 600000`) fails identically on the merge-base; pre-existing, not touched here.
- `pnpm run typecheck`, Vitest for components/coordinators and the new hook, `pnpm run i18n:check` and `i18n:ratchet`: pass.
