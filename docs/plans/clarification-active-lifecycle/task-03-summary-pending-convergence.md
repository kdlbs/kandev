---
id: "03-summary-pending-convergence"
title: "Summary pending convergence"
status: completed
wave: 3
depends_on: ["02-current-turn-backend-authority"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/clarification-active-lifecycle.md"
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
- PR review follow-up preserves the last valid summary snapshot when a task disappears during a
  rejected-CAS reload, avoiding a nil overwrite and spurious missing-summary rebuild. CAS retries also
  reload session state before re-deriving pending ownership.
- `cd apps/backend && go test ./internal/task/statussummary ./internal/task/service ./internal/backendapp`
  passed. The focused task-handler reconciliation test also passed.
- Full `go test ./internal/task/service -count=1` passed after the follow-up (291.044s).
- Final review remediation wires the live projector back into the task service as a synchronous
  acknowledgement boundary. Restored clarification publication now returns an error until durable
  pending-summary convergence succeeds, with focused service and handler regressions.
- Latest review remediation surfaces bounded summary CAS exhaustion instead of returning a false
  acknowledgement. Atomic clarification bundle methods now belong to the compile-time message repository
  contract rather than an optional runtime assertion. The focused CAS race check and full status-summary,
  task-service, and backend-app suites passed.
- Late Claude review publishes every committed restored clarification row even when synchronous summary
  acknowledgement is missing or fails, while preserving that error as the caller result. SQLite's
  detached guard now treats string `"true"` and `"1"` like boolean/numeric truthy metadata.
- Full SQLite repository and task-service suites passed in 49s and 129s; changed-code Go lint reported
  zero issues.
- Later Codex review rehydrates keyed Git observations whenever the loader is configured, including a
  restart whose persisted aggregate is nil. The nil-baseline multi-repository regression and full
  status-summary suite passed; changed-code Go lint reported zero issues.
- Final review follow-up rehydrates keyed pull-request observations under the same nil-aggregate rule
  during both restart restore and rejected-CAS rebase, preserving unchanged sibling pull requests when
  the next source event updates one key.
- The focused restart and rejected-CAS regressions and full status-summary suite passed; changed-code Go
  lint reported zero issues.
- Final Codex review moves configured session, Git, and pull-request rehydration ahead of missing-row
  creation, so the first keyed event cannot omit unchanged siblings. The focused missing-row regression
  and full status-summary suite passed.
- Fresh Codex review propagates existing-summary repair failures, deletes the failed stale row from the
  response map, and makes task-list and boot assembly use that partial authoritative result. Focused
  coverage plus the full task-service, handler, and backend-app suites passed.
- Delayed Claude review adds a structured warning at compare-and-set exhaustion with the task ID,
  configured attempt count, and last observed revision. The existing exhaustion regression now asserts
  that observability contract.
- Exact-head Codex review aligns exhaustion with the feature spec: a summary still disagreeing with the
  authoritative pending action after all retries is explicitly invalidated, so task-list and boot
  consumers clear cached stale state and use the coarse pending-action fallback. Delayed Codex follow-up
  carries that invalidation through the Go DTO and shared desktop/mobile snapshot merge without erasing
  a newer WebSocket revision received while the read was in flight.
- Task service, DTO, handler, backend composition, 31 focused web tests, frontend typecheck/lint, and
  changed-code Go lint pass after remediation.
- Latest Claude follow-up turns a corrupt persisted projection epoch's otherwise opaque `sql.ErrNoRows`
  into an actionable canonical-positive-integer diagnostic, with a focused regression.
