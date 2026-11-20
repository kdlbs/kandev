---
id: "03-exclusion-setup-warnings-backend"
title: "Predict exclusion halts and return setup warnings"
status: done
wave: 2
depends_on:
  - "01-dispatch-author-exclusion"
plan: "plan.md"
requirements:
  - REQ-OFFICE-REVIEW-AUTHOR-EXCL-005
acceptance_criteria:
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-005.1
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-005.2
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-005.3
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-005.4
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-005.5
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-005.6
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-005.8
system_design:
  - ../../specs/office/system-design/review-author-provider-exclusion.md
---

# Task 03: Predict exclusion halts and return setup warnings

## Summary

Backend half of system design §7 and §8: the pure setup evaluator, the stored
declaration query, the routing GET/PUT warning fields, import preview and
result warnings through an optional workflow-service port, and workflow sync
warning strings.

## Scope

1. `routing.EvaluateExclusionSetup` with the closed code set and deterministic
   ordering. Move `seatRolePools` to `office/models.SeatPoolAgentRoles` and
   switch the seat caster to it.
2. `ListAuthorExclusionDeclarations(ctx, workspaceID)` in the Office repository,
   and `routing.Provider.ExclusionSetup` / `StoredExclusionSetup`.
3. `RoutingConfigResponse.author_exclusion_warnings`. The PUT response gains the same field,
   computed after the save.
4. `service.ImportExclusionAdvisor` port with `SetImportExclusionAdvisor`.
   Add `Warnings` to `ImportProfilePreview` and `ImportResult`, and append
   sync warnings in `sync_apply.go`. Wire the adapter in `internal/backendapp`.

## Exclusions

Web UI (Task 04). No save, import, or sync is ever blocked. No provider-health
prediction and no per-agent override form warnings.

## Implementation acceptance

- Evaluator table tests cover: routing nil or disabled → `routing_disabled`;
  enabled with order `[A]` → `single_provider` naming A; order `[A, B]` with
  a reviewer-pool agent pinned to `[C]` → `agent_single_provider` naming C; stopped,
  pending, and out-of-pool agents ignored; no declarations → no warnings.
- Routing GET/PUT, import preview, import, and sync return the warnings for a
  declaring document or workspace and none otherwise. Each flow still
  succeeds. A failing advisor yields one `check_failed` warning and the flow
  still succeeds.
- Seat-caster tests pass unchanged after the pool move.

## Verification

```bash
cd apps/backend
go test -count=1 ./internal/office/routing/... ./internal/office/models/... \
  ./internal/office/engine_adapters/... ./internal/office/dashboard/... \
  ./internal/office/repository/sqlite/... ./internal/office/service/... \
  ./internal/workflow/service/... ./internal/workflow/handlers/... \
  ./internal/workflowsync/... ./internal/backendapp/...
cd ../..
bash scripts/lint-go-changed <changed-go-files>
```

## Likely files

- `apps/backend/internal/office/routing/author_exclusion_setup.go` (+ test)
- `apps/backend/internal/office/models/` (seat pool helper, + test)
- `apps/backend/internal/office/engine_adapters/seat_caster.go`
- `apps/backend/internal/office/repository/sqlite/author_provider.go` (+ test)
- `apps/backend/internal/office/service/` (`AuthorExclusionSetup`)
- `apps/backend/internal/office/dashboard/handler_routing.go`, `routing_dto.go` (+ tests)
- `apps/backend/internal/workflow/service/service.go`, `import_profile_selection.go`,
  `sync_apply.go`, new `import_exclusion_advisor.go` (+ tests)
- `apps/backend/internal/backendapp/services.go` (adapter wiring)

## Risks

- The PUT response shape changes from `{ok: true}`. Keep `ok` so existing
  callers are unaffected.
- `RoutingConfigResponse` gains a storage read on every GET. It reads one
  workspace's workflow steps, which is acceptable for a settings page.

## Results

Implemented with TDD.

- `routing.EvaluateExclusionSetup` (table tests cover routing nil or
  disabled, a single workspace provider, an empty pool, pool agents that all
  override, a pinned pool agent, inactive and out-of-pool agents, the approver
  pool, and deterministic sorting).
- `routing.Provider.ExclusionSetup` and `StoredExclusionSetup`, and
  `ListAuthorExclusionDeclarations` (SQLite test, and the PostgreSQL twin
  passes against a disposable local server).
- The seat pool moved to `office/models.IsSeatPoolEligible`. The seat-caster
  tests pass unchanged.
- Routing GET and PUT return `author_exclusion_warnings`. A failed check is a
  single `check_failed` warning with a 200 response.
- Workflow import preview and import (raw and with bindings) return
  `warnings`, through `service.ImportExclusionAdvisor`, which `backendapp`
  wires to `routing.Provider`. Workflow sync appends text warnings. A failing
  advisor yields `check_failed`, and the flow still succeeds. With no
  declarations, the advisor is not called.
- All verification packages pass, and `scripts/lint-go-changed` reports
  0 issues.
