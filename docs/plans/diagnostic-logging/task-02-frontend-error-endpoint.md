---
id: "02-frontend-error-endpoint"
title: "Frontend-error endpoint"
status: completed
wave: 2
depends_on: ["01-backend-log-sinks"]
plan: "plan.md"
spec: "../../specs/platform/diagnostic-logging.md"
---

# Task 02: Frontend-error endpoint

## Acceptance

- Authenticated install users can submit the exact bounded
  `POST /api/v1/system/logs/frontend-errors` contract and receive `204`.
- A valid request creates one fixed `frontend error toast` error entry with
  client data, including an optional bounded `task_id`, only in structured
  fields.
- Malformed, oversized, source-invalid, text-empty, and truncation cases follow
  the spec without weakening the existing log-download allow-list.
- Per-identity 60/minute burst-20 and process-wide 300/minute burst-100 token
  buckets return `429` plus `Retry-After`; stale identity buckets are removed.
- The endpoint lives in `internal/system/frontenderrors`, not the legacy
  `internal/system/logs` package removed by Task 09.

## Verification

```bash
cd apps/backend
go test ./internal/system/frontenderrors ./internal/system
```

## Files likely touched

- `apps/backend/internal/system/frontenderrors/handler.go`
- `apps/backend/internal/system/frontenderrors/handler_test.go`
- `apps/backend/internal/system/frontenderrors/limiter.go`
- `apps/backend/internal/system/frontenderrors/limiter_test.go`
- `apps/backend/internal/system/system.go`
- `apps/backend/internal/system/system_routes_test.go`

## Dependencies

- Task 01 provides the dual-sink logger and fixed file used by the endpoint.

## Parallelism

Parallel-safe with Task 03 after Task 01. The files are disjoint from the
launcher implementations and no shared schema or generated contract changes.

## Inputs

- Spec: API surface, Permissions, Failure modes, and report Scenarios.
- Plan: Backend / Frontend-error ingestion.
- Existing patterns: System route composition and `system/logs` handler tests.

## Risks

- Body limiting must occur before JSON decoding.
- Truncation must preserve valid UTF-8 and must not turn client data into a log
  message or level.
- `task_id` is an untrusted correlation field, not authorization or proof that
  the reporting user owns a task.
- Limiter identity keys come only from authenticated context; task IDs and
  browser fields never select a bucket.

## Output contract

Report the final request shape and limits, files changed, exact test
command/result, blockers or risks, and update this task plus `plan.md` status
in the same conversation.
