---
id: "03-summary-pending-convergence"
title: "Summary pending convergence"
status: completed
wave: 3
depends_on: ["02-current-turn-backend-authority"]
plan: "plan.md"
spec: "../../specs/clarification-active-lifecycle/spec.md"
---

# Task 03: Summary pending convergence

## Acceptance

- The production projector derives pending state from a bounded authoritative loader on restore and
  pending-sensitive events, including an ordinary message in a newer turn and deletion of that turn's
  last message; event order and message deletion cannot re-arm an older request.
- Boot/task-list hydration repairs an existing stale pending field with monotonic bounded CAS,
  preserves every unrelated summary field, returns the corrected row, and publishes a complete
  replacement on semantic change.
- Loader errors retain last known pending state; CAS rejection reloads and resynchronizes before
  retry/later events.

## Verification

```bash
cd apps/backend && go test ./internal/task/statussummary ./internal/task/service ./internal/backendapp
```

## Files likely touched

- `apps/backend/internal/task/statussummary/projector.go`
- `apps/backend/internal/task/statussummary/projector_events.go`
- `apps/backend/internal/task/statussummary/projector_helpers.go`
- `apps/backend/internal/task/statussummary/projector_test.go`
- `apps/backend/internal/task/service/service_status_summary_rebuild.go`
- `apps/backend/internal/task/service/service_status_summary_rebuild_test.go`
- `apps/backend/internal/task/handlers/task_http_handlers.go`
- Task HTTP status-summary test nearest the existing list hydration coverage
- `apps/backend/internal/backendapp/gateway.go`
- `apps/backend/internal/backendapp/boot_state.go`
- `apps/backend/internal/backendapp/status_summary_boot_test.go`

## Dependencies

Task 02.

## Parallelism

Sequential. It shares pending repository semantics with Task 02 and defines the task action consumed
by Task 04.

## Inputs

- Spec persistence, failure, and summary-repair scenarios.
- `statussummary.BuildFromAuthoritative`, `SummaryUpdated`, and existing queued-count CAS retry pattern.
- Task list/boot already batch-load sessions and `GetPendingActionsBySessionIDs`; reuse those bounded
  inputs.

## Risks

- Never rebuild unrelated fields of an existing summary from incomplete read inputs.
- A successful CAS followed by publication failure must not roll back the stored correction.
- Avoid self-subscription loops when publishing `task.status_summary.updated`.
- Do not add transcript reads or one query per task to normal list hydration.

## Output contract

Use TDD: prove stale restore, newer-turn clear, loader failure, existing-row repair, and CAS contention
fail first. Run the exact package command, report test counts/results and event behavior, reconcile
actual files, then update task/plan status.

## Results

- Added an authoritative pending-action loader to the live projector. Restore, message,
  clarification, permission, task, and session-state occurrences refresh bounded repository state;
  lookup failure retains the stored action, and CAS rejection reloads/retries.
- Replaced missing-only hydration with summary reconciliation. Existing rows update only
  `pending_action`, advance revision/time, preserve unrelated fields, and publish one complete
  `task.status_summary.updated` replacement. Boot and task-list assembly now invoke it.
- Added restore, ordinary-message, deletion, loader-error, CAS-contention, boot, and task-list
  regressions.
- `cd apps/backend && go test ./internal/task/statussummary ./internal/task/service ./internal/backendapp`
  passed. The focused task-handler reconciliation test also passed.
