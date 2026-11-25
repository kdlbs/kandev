---
created: 2026-10-26
status: implemented
requirements:
  - REQ-TASKS-MCP-PRIORITY-001
system_design:
  - ../../specs/tasks/system-design/mcp-task-priority.md
legacy_specs: []
---

# Implementation plan: MCP task priority

## Overview

[Issue 4696](https://github.com/kdlbs/kandev/issues/4696) requests priority at
creation and during later task updates. Extend the existing create and generic
update tools, then expose generic update to external clients and prove the
complete transport path. Execute the two work orders sequentially.

The two sequential work orders implement the contract and prove it through the
existing task service. Production changes, permanent tests, and public docs are
included in this workspace diff.

## Confirmed root cause and evidence

Investigation used commit `b4e9cf5ede3545ca6a8e58165e580ad7c4988afa`.
The issue was open, unassigned, and contained no comments or image attachments.
After diagnosis, assignment to `carlosflorencio` succeeded and was read back
from GitHub. The issue remains open.

- `registerCreateTaskTool` advertises no priority. Closed argument validation
  rejects a call with `priority: "high"` before backend dispatch.
- Task mode already has `update_task_kandev`, beyond the issue's state-only
  example. Its schema also omits priority and rejects the same argument.
- `server.createTaskHandler`, `handlers.mcpCreateTaskRequest`, and the service
  request mapping all omit priority. Direct backend creation silently ignores
  it, and the service applies `medium`.
- The generic update bridge and backend request mapping omit priority too.
  Direct backend update leaves the stored priority unchanged.
- External mode exposes creation and configuration task tools but lacks the
  generic update tool. Task service validation, narrow priority updates, DTOs,
  listing, and task event publication already exist.

Temporary diagnostic tests exercised the real service and SQLite. They were
removed after the run. No live instance or developer data was changed.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/mcp/server ./internal/mcp/handlers -run '^TestRepro4696' -count=1 -v)
```

Observed failures:

| Diagnostic | Expected | Observed |
| --- | --- | --- |
| `TestRepro4696PriorityToolCalls/create_task_kandev` | Accept `high` | Unknown argument `priority` |
| `TestRepro4696PriorityToolCalls/update_task_kandev` | Accept `high` | Unknown argument `priority` |
| `TestRepro4696PriorityThroughTaskService/create` | Store `high` | Response and SQLite contain `medium` |
| `TestRepro4696PriorityThroughTaskService/update` | Store `low` | SQLite still contains `medium` |

The minimal server reproduction submits `{"title":"Priority repro","priority":"high"}`
to create, or `{"task_id":"task-current","priority":"high"}` to generic update.
Backend reproduction supplies valid workspace, workflow, profile, and
`start_agent:false`, then reads the created task from the service.

## Requirements and assumptions

The [task requirement](../../specs/tasks/requirements/mcp-task-priority.md)
owns this automation lifecycle. This is a missing API capability, not a
violation of the existing UI priority acceptance criteria.

The issue settles the enum, creation default, backward compatibility, and need
for later updates. Reusing the generic update tool is a local design choice.
No material product question remains unresolved.

The [priority visibility requirement](../../specs/tasks/requirements/task-priority-visibility.md)
explicitly excludes MCP writes. Its UI consumers remain applicable without
changing their layout. The external umbrella requirement links to the new
task-owned contract instead of defining another priority vocabulary.

## Scope

### In scope

- Optional priority on existing create and generic update tools.
- Backend validation and complete request forwarding.
- Creation default, update omission, field preservation, retry, and event proof.
- External registration of the existing generic update tool.
- Transport and authorization coverage, and public MCP documentation.

### Out of scope

- Office CLI changes, assignment, or new Office-session tools.
- New state semantics, bulk writes, permissions, flags, or schema migrations.
- UI controls, frontend code, translations, or scheduling changes.
- Issue comments, commits, pushes, or PR creation.

## Technical approach

Task 01 adds schema metadata and forwarding in
`internal/mcp/server/server.go` and `handlers.go`. It adds decoded fields in
`internal/mcp/handlers/create_task_mode.go` and `handlers.go`, then passes
them into the existing service requests. Update omission remains nil.
Validate priority before any deferred-prompt write. Preserve compact responses.

Task 02 extracts `registerUpdateTaskTool` and adds an external-only profile
group in `server.go`. Keep configuration and Office catalogs unchanged.
Use the shared update action and existing authority rather than add an endpoint.

| Surface | Intended result | Evidence | Unsupported surface behavior |
| --- | --- | --- | --- |
| Task/title-pending | Create and update priority | Schema, forwarding, service tests | Existing scope rejection |
| Automation | Same shared fields | Catalog and guarded target tests | Foreign and self-target denial |
| External | Create and generic update priority | Real HTTP/dispatcher/SQLite journey | Foreign target denied |
| Configuration | Existing state-only operations | Catalog and state regression tests | Generic create/update absent |
| Office/coordinator | Existing tools | Catalog exclusions | Generic create/update absent |

## Tests

The table lists regression coverage for this package. Completed results are
recorded below.

| Acceptance criteria | Planned tests and files |
| --- | --- |
| 001.1, 001.2, 001.5 | `server/task_priority_test.go`: `TestMCPTaskPrioritySchema`, `TestMCPTaskPriorityArguments`, `TestMCPTaskPriorityForwarding` |
| 001.1, 001.2, 001.4, 001.6, 001.7 | `handlers/task_priority_test.go`: `TestHandleCreateTaskPriority`, `TestHandleUpdateTaskPriority`, `TestMCPTaskPriorityDeduplication` |
| 001.5 | Same handler file: `TestMCPTaskPriorityValidationPrecedesMutation`; `handlers/deferred_launch_prompt_test.go`: `TestHandleUpdateTask_InvalidStateDoesNotChangeLaunchPrompt`, `TestHandleUpdateTask_OverlongTitleDoesNotChangeLaunchPrompt` |
| 001.3, 001.8, 001.9 | `server/task_priority_test.go`: `TestMCPTaskPriorityCatalog`; `handlers/task_priority_test.go`: `TestMCPTaskPriorityAutomationScope` |
| 001.1, 001.3, 001.6, 001.7, 001.8 | `server/task_priority_integration_test.go`: `TestExternalMCPTaskPriorityJourney`, `TestExternalMCPTaskPriorityAuthorization`, `TestExternalMCPViewerCannotChangeDeferredLaunchPrompt` |
| 001.8 | `service/service_deferred_launch_test.go`: `TestUpdateDeferredLaunchPromptRejectsForeignExecutor` |

All numeric entries abbreviate `AC-TASKS-MCP-PRIORITY-`.
Existing priority service/repository tests remain the storage baseline.

## E2E tests

Use the external Streamable HTTP journey in Task 02 as API end-to-end proof.
It calls the actual catalog, schemas, dispatcher, handlers, service, and SQLite.
Create `high`, list it, update to `critical` without state, list it again, and
retry the original creation without reverting priority. Verify task events and
unchanged task/session fields. Exercise default creation and denied ownership.

No Playwright test is required because the package changes no rendered UI.
Existing UI priority behavior is supplied by the unchanged DTO and event path.

## Work orders

- [x] [Task 01: Carry priority through existing task tools](task-01-priority-fields.md)
- [x] [Task 02: Complete external MCP priority lifecycle](task-02-external-lifecycle.md)

## Verification results

- Diagnostic command: expected failure in both packages, confirming the schema
  and persistence gaps. Temporary tests were removed.
- `python3 scripts/list-docs.py validate`: passed, 403 decisions and 1742
  specifications validated.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `.github/scripts/pr-docs.cjs` pure `validateCoverage` preflight: `covered`,
  both work orders and their requirement/design references accepted. The design
  run included the planned backend source path to exercise coverage rather than
  take the documentation-only exemption. The Task 02 command uses actual paths.
- New-artifact relative links: all resolve.
- `git diff --check -- docs/plans/mcp-task-priority docs/specs`: passed.
- The design package was handed off uncommitted before implementation. The
  implementation and remediation are recorded in this tracked plan package.
- Task 01 and Task 02 focused MCP tests passed. The real Streamable HTTP tests
  passed for create, list, priority-only update, omitted priority, retry,
  event propagation, and authorized Kanban and Office writes.
- `go test -trimpath -tags fts5 ./internal/mcp/server -count=1`: passed.
- Focused MCP tests under `-race`: passed in both server and handlers packages.
- The full server/handlers race command found two stale external catalog
  assertions, which were corrected. The handler package then reached its
  10-minute timeout during existing SQLite schema initialization in
  `TestHandleCreateTask_SubtaskBaseBranchOverride_ExplicitReposWin`, while
  unrelated test processes were active. It produced no race report. The
  focused priority race tests passed afterward.
- `make -C apps/backend build`: passed for the backend and helper binaries.
- Changed-file Go lint: `0 issues`; `git diff --check`: passed.
- Public docs tests and validation passed. The spec catalog validated 403
  decisions and 1742 specifications; all 36 spec-linter tests and the full
  spec lint passed.
- The Task 02 PR documentation preflight returned `covered`; both work orders
  and their requirement/design references were accepted.
- PR review regressions prove deferred-prompt writes require task-write and
  executor authority, and invalid title/state values do not mutate a pending
  prompt. The public guide links to its owning MCP priority contract, and the
  visibility requirement no longer says the MCP contract is unchanged.
- Follow-up focused tests passed for MCP handlers, external MCP/SQLite
  authorization, and the task service's deferred-prompt cases. Backend build
  and changed-file Go lint passed after the review fixes.
- A non-race full handler package run timed out at ten minutes during SQLite
  schema initialization in `TestHandleMessageTask_CancelledPrimaryFallsBackToTheLiveSession`.
  No assertion failure occurred; focused changed-handler tests passed.

## Risks

- A default on update would silently reset priority. Only creation has a default.
- Schema-only changes would still lose priority in either mapping layer.
- Invalid combined updates could alter a deferred prompt before rejection.
- Reusing generic update adds one external tool and requires catalog assertions
  to change. It also exposes that tool's existing metadata fields to external
  callers under the same task-service authorization.
- Replaying creation must never revert a later priority update.
- Office CLI still rejects its priority flag. This package covers external MCP
  access to authorized tasks and does not repair that separate runtime surface.
