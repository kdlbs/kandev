## WebSockets

**Format:** `{id, type, action, payload, timestamp}`.

Use subscription hooks only; the WS client auto-deduplicates.
**Task overview vs. session detail:** Shared task rows read `Task.statusSummary` and `task.status_summary.updated`; rich streams stay session-detail-only. Extend the bounded projection per the [spec](../../docs/specs/platform/requirements/bounded-task-status-delivery.md) and [ADR](../../docs/decisions/2026-08-01-separate-task-summary-session-stream-traffic.md).
**Branch-scoped task state:** For live worktree/session state plus `task_prs`, key by `(repository, checked-out branch)`, not task/repository alone. `branch_switched` invalidates prior status/commits; reject late results with a generation/identity guard and preserve siblings. Historical PRs affect Changes only when `repository_id` and normalized `head_branch` match; Review/PR history may still show them. Test single/multi-repo cases and desktop/mobile Changes behavior.
**HTTP/WS cache races:** When HTTP hydrates a cache also updated by WebSockets, resolve every freshness-sensitive displayed field from one accepted live/HTTP projection, not a mixture of sources; this includes workflow placement and step badge name/color as well as activity/status. Guard responses with per-scope revision and request/workspace generation; discard or refresh stale responses and cover a newer live move plus stale-live rejection. For React StrictMode or multiple hydration effects, keep the mocked response deferred, assert request count while it is pending, then resolve it for cleanup. Action-level pending markers spanning status refreshes clear from the initiating request's settle path, not unrelated status changes; scope them by repository/path for concurrent operations. `useEnsureTaskSession` re-fires `session.ensure` when an open task page sees zero sessions after a prior ensure, so deleting the last session from that page spawns a replacement; test zero-session states through the backend/API instead.

- **Wire timestamps:** Use the shared `parseTurnTimestamp` parser before formatting or comparing untrusted RFC3339 values; `Date.parse` accepts or normalizes values such as `"0"` and February 30. Test syntactically malformed and Date-normalized malformed inputs, then follow the consumer's omit-or-reject contract.

When changing task lifecycle WS handlers (`task.updated`, `task.deleted`, `task.state_changed`), check both kanban and Office surfaces. Archive/delete
events may need to update kanban caches, `tasks.activeTaskId` / session pin
state, recent/sidebar prefs, Office refetch triggers such as
`setOfficeRefetchTrigger("tasks")`, and route redirects for `/t/:id`,
`/tasks/:id`, and `/office/tasks/:id`. Add focused tests for every affected
surface.
