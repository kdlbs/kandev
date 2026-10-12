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

`TaskHandlers.httpQuerySidebarTasks` checks request cancellation after body decoding and before returning malformed-input responses, then at query and enrichment boundaries.
A canceled body read must reach the existing 499 convention before it can return 400.
For a completed read, classify the response with the request context.
Use `errors.Is(err, context.Canceled)` to decide whether the returned error is itself cancellation.
A nested cancellation alone does not prove the HTTP caller abandoned the request.
Require the request context to identify cancellation before reducing severity.
If the request context is canceled but the repository returned an unrelated error, retain its error log and still return 499.

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
Omit its failed-count error entry only when both its supplied context and repository error chain identify `context.Canceled`.
Deadlines and unrelated database errors retain their existing diagnostic entry, including when an unrelated error races with request cancellation.
Do not change the count query, queue state, or pending-count semantics.

Status-summary reconciliation observes cancellation between optional reads and per-task operations.
Once a summary compare-and-update commits, publish the matching update event even if request cancellation follows the successful write.

## Verification

Use handler/router tests with deterministic cancellation during body decoding, query, and each enrichment stage.
Assert 499, no cancellation warnings/errors, no later optional query, and no state mutation.
Include wrapped cancellation, unrelated repository errors concurrent with cancellation, active-context failures, deadlines, committed summary writes, and a successful successor request.
These HTTP integration tests exercise the public read endpoint without browser layout changes.
