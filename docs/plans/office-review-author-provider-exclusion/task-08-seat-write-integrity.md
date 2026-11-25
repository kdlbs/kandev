---
id: "08-seat-write-integrity"
title: "Seat switch save integrity, parked-run release, and route history"
status: done
wave: 7
depends_on:
  - "07-grouped-warnings-save-bar"
plan: "plan.md"
requirements:
  - REQ-OFFICE-REVIEW-AUTHOR-EXCL-003
  - REQ-OFFICE-REVIEW-AUTHOR-EXCL-006
acceptance_criteria:
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-003.2
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-006.4
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-006.8
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-006.9
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-006.10
system_design:
  - ../../specs/office/system-design/review-author-provider-exclusion.md
---

# Task 08: Seat switch save integrity, parked-run release, and route history

## Summary

Fix the defects PR review found in the Review on a different provider
switches and the parked-run history. A save either writes every step or none,
and never overwrites a concurrent change. Turning a switch off releases the
reviews it parked. A run that parks after every other provider failed still
records the author skip. The routing page never shows another workspace's
switches.

## Scope

1. Workflow repository `UpdateStepEventsIfUnchanged`: one transaction,
   events and `updated_at` only, guarded by the read version;
   `models.ErrWorkflowStepChanged`.
2. `SetSeatAuthorExclusion`: all checks (including executor authorization and
   step validation) before the write, one guarded batch write, up to three
   attempts on a concurrent change; the dashboard maps a persistent conflict
   to 409.
3. `routing.Provider.ReleaseParkedRuns`, called by the PUT handler when a
   change turns the setting off.
4. `tryCandidates` writes the author skip before `exhaustedCandidates` parks.
5. `useWorkspaceRouting` hides data from another workspace and ignores late
   save responses.
6. Production comments carry no requirement IDs.

## Exclusions

Workflow-editor control (issue #5347). Releasing only the affected runs
rather than every routing-parked run in the workspace.

## Implementation acceptance

- A batch whose later step fails authorization or changed after the read
  writes nothing; two concurrent saves of different roles on one step both
  persist.
- Saving a switch off re-queues the workspace's routing-parked runs; saving
  it on does not.
- A seat run that parks after its only non-author candidate fails has a
  `skipped_author_provider` attempt for the author provider.

## Verification

```bash
cd apps/backend
go test -count=1 ./internal/workflow/repository/... ./internal/workflow/service/... \
  ./internal/office/routing/... ./internal/office/dashboard/... \
  ./internal/office/scheduler/... ./internal/backendapp/...
go test -race -count=30 -run ConcurrentRoleChanges ./internal/workflow/service/
cd ../..
bash scripts/lint-go-changed <changed-go-files>
cd apps/web
pnpm exec vitest run hooks/domains/office app/office/workspace/routing
```

## Likely files

- `apps/backend/internal/workflow/repository/step_events.go` (+ test)
- `apps/backend/internal/workflow/models/errors.go`
- `apps/backend/internal/workflow/service/seat_author_exclusion.go` (+ test)
- `apps/backend/internal/backendapp/adapters_author_exclusion.go` (+ test)
- `apps/backend/internal/office/dashboard/handler_routing_seat.go`, `service.go` (+ tests)
- `apps/backend/internal/office/routing/seat_steps.go` (+ test)
- `apps/backend/internal/office/scheduler/dispatch_routing.go` (+ test)
- `apps/web/hooks/domains/office/use-workspace-routing.ts` (+ test)

## Risks

- Releasing every routing-parked run in the workspace can re-dispatch runs
  the change did not affect; they re-park with the same verdict, as after a
  routing settings change.

## Results

Implemented with TDD. The new repository, service, routing, dashboard,
scheduler, and hook tests failed before the fix and pass after it. The
concurrency test passes 30 runs under `-race`.
