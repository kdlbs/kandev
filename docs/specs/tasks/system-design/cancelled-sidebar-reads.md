---
status: draft
system: tasks
requirements:
  - REQ-TASKS-CANCELLED-SIDEBAR-READS-001
created: 2026-10-05
owners:
  - kandev
---

# Canceled sidebar read system design

## Purpose and boundaries

This design changes cancellation handling in task read paths.
It preserves task mutations, workspace authorization, successful DTOs, and active-request failure handling.
No rendered UI changes are required.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-TASKS-CANCELLED-SIDEBAR-READS-001 | Cancellation boundary; Enrichment; Verification |

## Cancellation boundary

`TaskHandlers.httpQuerySidebarTasks` currently returns 500 after a canceled `sidebarTaskPageResponse` call.
Handle request cancellation before error logging at query and enrichment boundaries.
Use the request context and `errors.Is(err, context.Canceled)`.
A nested cancellation alone does not prove the HTTP caller abandoned the request.
Require the request context to identify cancellation before reducing severity.

Use the existing 499 convention from `internal/common/httpmw/logging.go`.
If no response started, finish the request with status 499 and no JSON error body.
If the response started, stop work without another header or body write.
Do not convert deadline expiration into client cancellation.

## Enrichment

The shared task-list enrichment path resides in `task_http_handlers.go`.
Check the request context before each optional query and after a canceled query returns.
Propagate cancellation instead of assembling fallback fields or starting the next read.
Preserve fallback behavior for genuine failures on active requests.

`messagequeue.Service.CountPendingByTaskIDs` must return its original error chain.
When its supplied context is canceled, omit its failed-count error entry.
Other database errors retain the existing error entry.
Do not change the count query, queue state, or pending-count semantics.

## Verification

Use handler/router tests with deterministic cancellation during query and during each enrichment stage.
Assert 499, no cancellation warnings/errors, no later optional query, and no state mutation.
Include wrapped cancellation, active-context failures, deadlines, and a successful successor request.
These HTTP integration tests exercise the public read endpoint without browser layout changes.
