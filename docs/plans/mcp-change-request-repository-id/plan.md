---
created: 2026-11-23
status: implemented
requirements:
  - REQ-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001
  - REQ-INTEGRATIONS-TASK-CHANGE-LINK-MCP-003
system_design:
  - ../../specs/integrations/system-design/task-change-link-mcp.md
legacy_specs: []
---

# Fix plan for repository ID validation in MCP links

## Overview

An agent calling `manage_task_change_request_kandev` needs the Kandev repository
ID for the target task. Fix the error shown when a `link` call supplies a
GitHub slug or another ID that matches none of that task's attachments. The
integration system owns this behavior because it defines the identity required
to associate a provider request with a task. [GitHub issue #5448](https://github.com/kdlbs/kandev/issues/5448).

The existing requirements cover canonical identity and explicit management.
Criterion `AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-003.4` adds catalog and error
requirements for this input. [Requirements](../../specs/integrations/requirements/task-change-link-mcp.md#req-integrations-task-change-link-mcp-003-manage-associations-with-explicit-intent)
and [system design](../../specs/integrations/system-design/task-change-link-mcp.md#failure-and-recovery).

## Evidence

A temporary Go test submitted MCP `tools/call` requests on main at
`a2ef38bbfcf`. It used the registered tool, the WebSocket dispatcher and
coordinator, a local SQLite database, and a fake GitHub service.

| `repository_id` | Observed result |
| --- | --- |
| `acme/api` | Tool error with generic `operation_error` and `state_known: false`. The provider recorded zero link attempts. |
| `unattached-repo` | Same error for a repository that exists in the workspace but is absent from the target task. |
| `repo-1` | Link succeeds and returns the attached repository ID and PR number. |

The MCP schema checks that the ID is a nonempty string. The coordinator's
`taskRepository` lookup then rejects an ID that matches no task attachment.
`taskChangeMutationFailure` maps this refusal to the generic mutation error.
The tool returns those structured details to the caller.

## Scope

The work order covers the neutral `link` action, its tool description, and its
help text in the public MCP reference. Existing acceptance criteria cover replacement
recovery and removal of stale associations.

## Technical approach

Return a dedicated error at the end of `taskRepository` when the attachment
scan finds no match. Use the handler's existing validation response for this
error on `link`. Reuse the lookup that runs after task authorization and
before provider dispatch. Compare repository IDs as opaque values.

Name the Kandev ID in the tool description. The MCP reference can point callers
to the task's `repositories[].repository_id` in `list_tasks_kandev`.

## Tests

| Criteria | Evidence |
| --- | --- |
| `AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-003.4`, `AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001.1` | `TestManageTaskChangeRequestLinkRepositoryIDValidation` reuses the dispatcher fixture with a slug, an unattached ID, and an attached ID. The first two cases fail because the response lacks a field-specific error. |
| `AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-003.4` | Add a tool description assertion to `TestManageTaskChangeRequestSchemaAndDispatch`. |
| `AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-003.3` | Existing backend tests cover stale unlink and replacement compensation. |

The dispatcher regression exercises the handler shared by both MCP protocol eras.
The [work order](task-01-validate-link-repository-id.md) lists the exact commands.

## Work orders

- [x] [Task 01 - Report invalid repository IDs during MCP link](task-01-validate-link-repository-id.md)

## Verification results

- The temporary MCP test reproduced the two error cases on main. The attached
  ID control linked successfully.
- The existing management tests in the work order's Go command passed in all
  three packages on main.
- `python3 scripts/list-docs.py validate` passed, validating 461 decisions and
  2,093 specifications. `python3 scripts/lint-spec-files.py --all` passed.
- The PR documentation coverage preflight passed for the fix package.

- `TestManageTaskChangeRequestLinkRepositoryIDValidation` failed before the
  fix because slug and unattached IDs returned the generic mutation message.
  It passes after the correction, including the attached-ID success control and
  sanitized provider-failure assertion.
- The work-order management test command passed in `internal/backendapp`,
  `internal/mcp/handlers`, and `internal/mcp/server`.
- Both public-doc validators passed: 63 tests and 56 published pages validated.
- `make -C apps/backend build`, changed-file Go lint, and `git diff --check`
  passed.

## Risks

- Mapping a repository read failure to validation would send callers toward a
  different ID when the repository service needs attention. Only the absence of
  an attachment match qualifies for the new error.
- Unlink uses a stored association, which may refer to a detached repository.
  Keep the error mapping specific to the `link` operation.
