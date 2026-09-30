---
id: "01-conditional-session-response"
title: "Conditional session response"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SESSION-REFRESH-EFFICIENCY-001
acceptance_criteria:
  - AC-UI-SESSION-REFRESH-EFFICIENCY-001.1
  - AC-UI-SESSION-REFRESH-EFFICIENCY-001.2
  - AC-UI-SESSION-REFRESH-EFFICIENCY-001.3
system_design:
  - ../../specs/ui/system-design/session-refresh-efficiency.md
---

# Task 01: Conditional Session Response

## Summary

Add an ETag to the existing authorized full-session response. A matching
conditional request returns 304 with no body. All other callers retain the
current full response and runtime fields.

## In scope

- Evaluate conditional headers after session authorization and full DTO
  enrichment, including transient runtime projections.
- Keep the pending-action revision stable across unchanged full-session reads
  while preserving the pre-read ordering guard for delayed snapshots.
- Preserve the current JSON representation and private response semantics.
- Add handler tests for 200/304, changed runtime state, and access failure.
- Add service coverage for stable snapshot revisions and stale-read rejection.

## Out of scope

- A new session DTO, frontend polling changes, and environment status reads.

## Acceptance

- A response's ETag changes whenever any serialized session field changes,
  including runtime-only fields.
- A matching authorized validator yields 304 with no body. An absent or
  different validator yields the existing 200 body.
- An unauthorized or missing session never yields 304 by matching a prior tag.

## Verification

```bash
(cd apps/backend && go test ./internal/task/handlers -run 'TestHTTPGetTaskSessionConditional' -count=1)
```

## Files likely touched

- `apps/backend/internal/task/handlers/task_http_handlers.go`
- `apps/backend/internal/task/handlers/task_session_etag_test.go` (new)
- `apps/backend/internal/task/service/service_pending_action_projection.go`
- `apps/backend/internal/task/service/service_events.go`
- `apps/backend/internal/task/service/service_pending_action_snapshot_test.go` (new)

## Dependencies

None.

## Risks

- Hashing a persisted timestamp instead of the final DTO can miss live
  foreground, cancellation, or parked state.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/ui/requirements/session-refresh-efficiency.md)
- [System design](../../specs/ui/system-design/session-refresh-efficiency.md)
- `apps/backend/internal/task/handlers/foreground_activity_test.go`

## Results

Implemented the authorized full-session ETag/304 response and stable
pending-action snapshot revision. Handler coverage verifies changed runtime
fields, matching validators, no body on 304, and authorization before matching.
Service coverage verifies stable no-op revisions and rejection of an older
delayed snapshot. Verification passed:

```bash
(cd apps/backend && go test ./internal/task/handlers -run 'TestHTTPGetTaskSessionConditional' -count=1)
(cd apps/backend && go test ./internal/task/service -run 'TestPendingActionSnapshot|TestPublishMessageEvent' -count=1)
```

Follow-up code-review remediation separates completed-read watermarks, stable
snapshot revisions, and emitted workspace-event deduplication. Snapshot reads
no longer consume compact event delivery. Deferred ordering regressions cover
newer unchanged event and snapshot results completing before older changed
results, and an in-memory event-bus test covers a snapshot between a database
change and its message event. The full task-service test package and backend
build pass with this correction.
