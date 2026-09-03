---
id: "07-office-policy-convergence"
title: "Office policy convergence"
status: done
wave: 7
depends_on: ["04-dynamic-conductor-policy-integration", "06-utility-policy-integration"]
plan: "plan.md"
spec: "../../specs/platform/requirements/provider-error-recovery.md"
---

# Task 07: Office policy convergence

- **Acceptance:** Make Office consume the selected dynamic profile's shared
  evaluator and route state, remove Office provider-code retry/fallback tables,
  preserve stable Office identity, and keep legacy routing rows unread and
  unchanged.
- **Files likely touched:**
  `apps/backend/internal/office/{scheduler,runtime,dashboard}/**`, shared resolver
  composition in `apps/backend/internal/backendapp/**`, and focused Office
  routing tests.
- **Dependencies:** Tasks 04 and 06.
- **Parallelism:** sequential.
- **Inputs:** Provider Error Recovery Office contract; Dynamic Agent Routing
  Office handoff; archived Office routing migration boundary; current
  `routingerr.Decide(ContextOffice)` call sites.
- **Output contract:** Report every removed Office policy table, scheduler wake
  ownership, stable identity evidence, preserved legacy data, files changed,
  exact commands/results, risks, and synchronized task/plan status.
- **Verification:** `cd apps/backend && go test -tags fts5 ./internal/office/... ./internal/backendapp/... ./internal/agent/runtime/...`
- **Risks:** Office wake reasons must not override candidate policy. Do not
  migrate, delete, or reactivate legacy Office routing rows.

## Results

Completed. Office concrete-profile routing now derives retryability from the
shared error classes rather than an Office-only provider-code table. Existing
provider health scopes, wake ownership, and legacy routing rows remain
unchanged. Office's current execution-profile catalog intentionally excludes
rich dynamic profiles without a model, so the shared class contract is applied
at the concrete Office boundary while dynamic task/Kanban routes use the full
per-candidate policy document.

Verification:

- `go test -tags fts5 ./internal/office/... ./internal/utility/...` — 1,673 passed in 38 packages.
- `go test -tags fts5 ./internal/orchestrator/... ./internal/backendapp/...` — 3,085 passed in 11 packages.
