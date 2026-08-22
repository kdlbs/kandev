---
id: "06-list-tasks-pending-action"
title: "list_tasks_kandev reports pending actions"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/external-question-answering/spec.md"
---

# Task 06: `list_tasks_kandev` reports pending actions

One call should find every blocked task in a workflow. The HTTP task list already returns this
projection; the MCP list does not.

- **Acceptance:**
  1. Each task in `list_tasks_kandev` carries `task_pending_action` and
     `primary_session_pending_action`, computed with the same rules as the HTTP list: only
     input-capable sessions count, and `permission` outranks `clarification` (T1).
  2. A task with no blocked session carries null for both fields, not an empty string (T2).
  3. The projection lives in one place, so the HTTP and MCP surfaces cannot drift.

- **Verification:**
  ```
  cd apps/backend && \
    gofmt -l internal/mcp/handlers internal/task && \
    go test -race ./internal/mcp/handlers/... ./internal/task/... && \
    go build ./...
  ```

- **Files likely touched:**
  - `apps/backend/internal/task/dto/` (extract the shared projection currently living as
    `isInputCapableSession` / `taskPendingActionPtr` / `pendingActionPtr` in
    `internal/task/handlers/task_http_handlers.go:378-417`)
  - `apps/backend/internal/task/handlers/task_http_handlers.go` (call the extracted helpers)
  - `apps/backend/internal/mcp/handlers/handlers.go:553` (`handleListTasks` — batch
    `taskSvc.BatchGetSessionsForTasks` + `taskSvc.GetPendingActionsForSessions` and stamp both
    fields alongside the existing `enrichTasksWithPRs`)
  - `apps/backend/internal/mcp/handlers/list_tasks_pending_action_test.go` (new)

- **Dependencies:** None.
- **Parallelism:** `parallel-safe` with tasks 01 and 08 — disjoint files, no shared schema,
  migration, or generated contract. Not parallel with task 05, which also edits
  `internal/mcp/handlers/handlers.go`.

- **Inputs:**
  - Spec T1, T2.
  - Plan § *Backend → `list_tasks_kandev` enrichment*.
  - `dto.TaskDTO.TaskPendingAction` / `.PrimarySessionPendingAction` already exist
    (`internal/task/dto/dto.go:168-169`) and are `*string` with no `omitempty`, which is what T2
    requires — do not add `omitempty`.
  - `BatchGetSessionsForTasks` and `GetPrimarySessionInfoForTasks` are the documented batch helpers
    (`task/service/service_sessions.go:204,218`); per `apps/backend/AGENTS.md` they take IDs derived
    from an already-authorized list, which `ListTasks` provides. Do not hand them caller-supplied
    IDs.

- **Output contract:** summary, files changed, tests run with counts, blockers, risks, and the
  task/plan status update in the same conversation.

## Results

Implemented in commit `cde150450`. Extracted the shared pending-action projection
(`isInputCapableSession` / `taskPendingActionPtr` / `pendingActionPtr`) out of
`task_http_handlers.go` into `internal/task/dto`, and `handleListTasks`
(`internal/mcp/handlers/handlers.go`) now batches `BatchGetSessionsForTasks` +
`GetPendingActionsForSessions` and stamps `TaskPendingAction` / `PrimarySessionPendingAction`
alongside the existing PR enrichment, using the same rules as the HTTP list (permission outranks
clarification, input-capable sessions only, null rather than empty string when nothing is blocked).

Verified in this session's final gauntlet (Wave 10): `go test ./internal/mcp/handlers/...
./internal/task/...` passes, including `list_tasks_pending_action_test.go` (both fields populated for
a blocked session, both null for an unblocked one). `gofmt -l` reports no files; `go build ./...`
succeeds.

No external side effects. The projection now lives in one place (`internal/task/dto`), so the HTTP
and MCP surfaces cannot drift.
</content>
