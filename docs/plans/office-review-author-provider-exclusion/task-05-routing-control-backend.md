---
id: "05-routing-control-backend"
title: "Read and write the setting from the routing settings API"
status: done
wave: 4
depends_on:
  - "03-exclusion-setup-warnings-backend"
plan: "plan.md"
requirements:
  - REQ-OFFICE-REVIEW-AUTHOR-EXCL-005
  - REQ-OFFICE-REVIEW-AUTHOR-EXCL-006
acceptance_criteria:
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-005.9
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-006.1
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-006.2
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-006.3
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-006.4
system_design:
  - ../../specs/office/system-design/review-author-provider-exclusion.md
---

# Task 05: Read and write the setting from the routing settings API

## Summary

Backend half of system design §10: list every seat-ensuring step on the routing
GET, and add a batch write endpoint that sets or clears
`exclude_author_provider` through the workflow service and publishes step
updates. Rewrite the workflow sync warning text in the three-part shape.

## Scope

1. `wfmodels.ParticipantSeatRoles` and `wfmodels.SetExcludeAuthorProvider`.
2. Office repository `ListSeatSteps` and `routing.SeatStep`; routing GET adds
   `author_exclusion_steps`.
3. `workflow/service.SetSeatAuthorExclusion` (validate every change, then
   write), an Office dashboard port wired in `internal/backendapp` with a
   `stepevents.Publisher` (source `office-routing`), and
   `PUT /workspaces/:wsId/routing/author-exclusion`.
4. Sync warning text per AC-OFFICE-REVIEW-AUTHOR-EXCL-005.9.

## Exclusions

Web (Task 06). Workflow-editor controls (issue #5347).

## Implementation acceptance

- GET lists one row per seat action, hidden workflows included, in workflow,
  position, role order; the PUT flips only the targeted action's flag,
  publishes `workflow_step.updated`, and returns the new rows and warnings.
- A change for a foreign step, a missing role, or a read-only workflow writes
  nothing and answers 400, 404, or 409.

## Verification

```bash
cd apps/backend
go test -count=1 ./internal/workflow/models/... ./internal/workflow/service/... \
  ./internal/office/repository/sqlite/... ./internal/office/routing/... \
  ./internal/office/dashboard/... ./internal/backendapp/...
cd ../..
bash scripts/lint-go-changed <changed-go-files>
```

## Likely files

- `apps/backend/internal/workflow/models/author_exclusion.go` (+ test)
- `apps/backend/internal/workflow/service/seat_author_exclusion.go` (+ test)
- `apps/backend/internal/office/repository/sqlite/author_provider.go` (+ test)
- `apps/backend/internal/office/routing/provider.go`, `author_exclusion_setup.go`
- `apps/backend/internal/office/dashboard/handler_routing.go`, `routing_dto.go`, `handler.go` (+ tests)
- `apps/backend/internal/backendapp/adapters_author_exclusion.go`, `main.go`
- `apps/backend/internal/workflow/service/import_exclusion_advisor.go` (sync text)

## Risks

- The built-in Office workflow is hidden; the read must not filter hidden
  workflows.
- Step updates must go through the workflow service so authorization,
  mutability, and step events stay identical to the other mutation surfaces.

## Results

Implemented with TDD.

- `wfmodels.ParticipantSeatRoles` and `SetExcludeAuthorProvider` (copy, not
  mutate; other actions and keys kept).
- `ListSeatSteps` in the Office repository, with hidden workflows included
  (SQLite test plus a PostgreSQL twin, `TestPostgresListSeatSteps`, which
  skips without a DSN).
- `workflow/service.SetSeatAuthorExclusion` validates every change (visible,
  in the workspace, mutable, seat role present) before the first write.
- `PUT /workspaces/:wsId/routing/author-exclusion` through a dashboard port
  that `backendapp` wires to the workflow service and a `stepevents.Publisher`
  (source `office-routing`). Errors map to 400, 404, 409, or 500. The routing
  GET returns `author_exclusion_steps`.
- Sync warning text uses the three-part wording shared with the web.
- The verification packages pass, and `scripts/lint-go-changed` reports
  0 issues.
