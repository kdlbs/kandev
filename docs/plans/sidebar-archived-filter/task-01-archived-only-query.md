---
id: "01-archived-only-query"
title: "Add archived-only task query"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/sidebar-archived-filter.md"
---

# Task 01: Add archived-only task query

Extend the existing workspace task-list contract with an efficient paginated
archived-only mode while preserving every current default and compatibility
mode.

## Acceptance

- `only_archived=true` returns only archived, non-ephemeral tasks and an
  archived-only pre-pagination total; default and `include_archived=true`
  behavior remain unchanged.
- Search, workflow/repository filters, sorting, authorization, PR-number
  augmentation, and pagination apply the same archive mode; archived-only wins
  when both archive flags are supplied.
- Repository, service, and HTTP regressions cover active/all/archived-only
  modes, and every affected repository mock compiles with the updated contract.

## Verification

```bash
cd apps/backend && go test ./internal/task/repository ./internal/task/service ./internal/task/handlers -run 'ArchiveModes|OnlyArchived|PRMatchRespectsArchive'
cd apps/backend && go test ./internal/backendapp ./internal/orchestrator/executor ./internal/plugins ./internal/office/... ./internal/task/...
```

## Files likely touched

- `apps/backend/internal/task/handlers/task_http_handlers.go`
- `apps/backend/internal/task/handlers/task_http_handlers_test.go`
- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/task/repository/sqlite/task.go`
- `apps/backend/internal/task/repository/archive_repository_test.go`
- `apps/backend/internal/task/service/service_tasks.go`
- `apps/backend/internal/task/service/service_pr_search_test.go`
- `apps/backend/internal/task/handlers/process_handlers_test.go`
- `apps/backend/internal/task/handlers/quick_chat_list_handlers_test.go`
- `apps/backend/internal/task/service/handoff_workspace_test.go`
- Other compile-only `TaskRepository` mocks identified by `rg -n "ListTasksByWorkspace" apps/backend`

## Dependencies

None.

## Parallelism

`parallel-safe` with Task 02: this task owns backend files; Task 02 owns the
saved-view/filter frontend files.

## Inputs

- Spec **API surface**, **Failure modes**, and archive query scenarios.
- Plan **Archived-only workspace task query**.
- Existing `ListTasksByWorkspace` active/all behavior and
  `service_pr_search_test.go` archive filtering pattern.

## Risks

- The count query, row query, and PR-number augmentation must use the same mode
  or pagination totals will disagree with returned rows.
- Signature changes reach test doubles outside `internal/task`; compile-only
  updates must not alter their behavior.

## Output contract

Report the request precedence, SQL predicates, compatibility behavior, exact
files changed, commands/results, blockers, and update this task plus
`plan.md` status/results in the same conversation.

## Results

- Added the additive `ListTasksByWorkspaceWithArchiveMode` repository/service
  contract and `only_archived=true` HTTP query handling. Existing callers keep
  the active/all compatibility method.
- Archive predicates now apply consistently to non-search and search count/page
  queries; `onlyArchived` takes precedence over `includeArchived` and excludes
  ephemeral rows through the existing defaults.
- Added repository coverage for archived-only rows, totals, search, and
  ephemeral exclusion, plus PR-number augmentation coverage for archived-only
  mode.
- `cd apps/backend && go test ./internal/task/repository ./internal/task/service ./internal/task/handlers -run 'ArchiveModes|OnlyArchived|PRMatchRespectsArchive'` — passed (3 tests).
- `cd apps/backend && go test ./internal/backendapp ./internal/orchestrator/executor ./internal/plugins ./internal/office/... ./internal/task/...` — passed.
