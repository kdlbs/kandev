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
acceptance_criteria:
  - AC-COORDINATOR-WAKE-004.1
  - AC-COORDINATOR-WAKE-004.3
  - AC-COORDINATOR-WAKE-004.4
  - AC-COORDINATOR-SPEND-001.1
  - AC-COORDINATOR-SPEND-001.2
  - AC-COORDINATOR-SPEND-001.3
system_design:
  - ../../specs/coordinator/system-design/wake.md
  - ../../specs/coordinator/system-design/spend.md
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
- `internal/backendapp/coordinator.go`: compute "phase 3 effective" once;
  empty named registration functions `registerCoordinatorContainment`,
  `...Spend`, `...Wake`, `...Delivery`, `...Relay`, `...Reply`,
  `...Automatic`, `...Improvements`.
- `internal/coordinator/store.go`: `coordinators.autonomy_enabled`,
  `coordinators.cost_ceiling_subcents`; `coordinator_proposals.kind`,
  `reply_text`, `reply_delivered_at`, `in_reply_to`,
  `decided_automatically`; tables `coordinator_wakes`,
  `coordinator_unattended_turns` (with the partial unique index),
  `coordinator_class_reviews`, `coordinator_pending_changes`; deletion with
  the coordinator and on `workspace.deleted`; retention in the startup pass
  ([wake Store](../../specs/coordinator/system-design/wake.md#store)).
- PATCH fields `autonomy_enabled` and `cost_ceiling_usd` with the integer
  parser ([spend Ceiling](../../specs/coordinator/system-design/spend.md#ceiling)),
  the interlock, no `config_revision` change, and the supersede on autonomy
  off in the same transaction; GET and list carry both fields; the guard
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
  transaction and leaves an open turn row untouched.
- With either flag off, the new PATCH fields are ignored, every phase 3 route
  is 404, the Autonomy section is hidden, and stored rows survive an off/on
  cycle.

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
