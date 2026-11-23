---
id: "01-validate-link-repository-id"
title: "Report invalid repository IDs during MCP link"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001
  - REQ-INTEGRATIONS-TASK-CHANGE-LINK-MCP-003
acceptance_criteria:
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001.1
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-003.3
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-003.4
system_design:
  - ../../specs/integrations/system-design/task-change-link-mcp.md
---

# Task 01 - Report invalid repository IDs during MCP link

## Summary

Expose a field-specific validation message when an authorized `link` request
supplies a repository ID absent from the target task. Clarify where agents can
find the Kandev repository ID.

## Scope

1. Add `TestManageTaskChangeRequestLinkRepositoryIDValidation` in
   `task_change_link_mcp_test.go`. Use `dispatchTaskChangeManagement` and the
   existing SQLite task fixture and fake GitHub provider. Check a slug and an
   unattached ID for the validation error. Use an attached ID to verify that
   link succeeds for the same PR.
2. In `taskRepository`, return a dedicated error when the attachment scan finds
   no match. Define the error at the coordinator's interface in
   `internal/mcp/handlers/task_change_link.go`. Use the existing error text
   `repository not found for task` because the legacy handler forwards
   `err.Error()` directly. Keep the new error specific to the attachment scan.
3. In `handleManageTaskChangeRequest`, use `errors.Is` on a `link` error from
   the coordinator to identify the missing attachment. Return
   `ws.ErrorCodeValidation` with the message
   `repository_id must be the Kandev ID of a repository attached to the target task`
   and nil details. The MCP handler already presents plain errors as text.
   Store failures and provider failures use the generic mutation response.
4. Name `repository_id` as a Kandev repository ID in the tool description.
   Check the description in the existing catalog test. In the public reference, identify
   `repositories[].repository_id` from `list_tasks_kandev` and explain the
   attachment requirement for a new link. `unlink` uses the stored association's
   repository ID.

## Acceptance

- The advertised description identifies `repository_id` as a Kandev ID. A slug
  and an existing but unattached ID return the field-specific validation error
  before any provider attempt or association write.
- An attached Kandev ID creates the expected association and returns the
  canonical link. Exercise the tool with the same provider and number as the
  rejected calls.
- A provider failure uses the sanitized mutation envelope. The existing stale
  unlink and replacement compensation tests pass.

## Verification

Run from the repository root. Run the new integration test alone before the
correction to confirm the error assertion fails for both invalid inputs.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/backendapp -run '^TestManageTaskChangeRequestLinkRepositoryIDValidation$' -count=1 -v)
```

After the correction, run the regression with the existing management tests:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/backendapp ./internal/mcp/handlers ./internal/mcp/server -run '^(TestManageTaskChangeRequest|TestTaskChangeCoordinator|TestTaskChangeLinkMCPSingleUser|TestTaskChangeManagementDispatches|TestTaskChangeRequestProtocolCatalogAndCallAcrossProtocolEras)' -count=1)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
make -C apps/backend build
```

## Files likely touched

- `apps/backend/internal/backendapp/task_change_link_coordinator.go`
- `apps/backend/internal/backendapp/task_change_link_coordinator_test.go`
- `apps/backend/internal/backendapp/task_change_link_mcp_test.go`
- `apps/backend/internal/mcp/handlers/task_change_link.go`
- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/task_change_request_tools_test.go`
- `docs/public/automation-and-mcp.md`

## Dependencies

None.

## Risks

Treat the Kandev repository ID as opaque. Validate the attachment after task
authorization and before provider dispatch. Use the verified repository record
to derive the provider URL.

## Parallelism

`sequential`

## Inputs

- [Association management requirements](../../specs/integrations/requirements/task-change-link-mcp.md#req-integrations-task-change-link-mcp-003-manage-associations-with-explicit-intent).
- [Failure and recovery design](../../specs/integrations/system-design/task-change-link-mcp.md#failure-and-recovery).
- `apps/backend/internal/backendapp/task_change_link_coordinator_test.go` for
  `dispatchTaskChangeManagement`, the SQLite fixtures, and fake providers.

## Results

Implemented the dedicated unattached-repository error and mapped it to a
field-specific validation response for `link`. Updated the tool description and
public MCP reference with the Kandev repository ID source and attachment rule.
The review follow-up consolidated sentinel classification for both handler
paths and moved the repository ID guidance before the JSON example introduction.

The regression failed before the fix for both a provider slug and an existing
unattached repository ID, then passed with the attached ID control and sanitized
provider-failure case.

Verification passed:

- Work-order management tests in `internal/backendapp`, `internal/mcp/handlers`,
  and `internal/mcp/server`.
- `node --test scripts/validate-public-docs.test.mjs` passed: 63 tests.
- `node scripts/validate-public-docs.mjs` passed: 56 published pages.
- `python3 scripts/list-docs.py validate` passed, validating 461 decisions and
  2,093 specifications.
- `python3 scripts/lint-spec-files.py --all` passed.
- Changed-file Go lint, `make -C apps/backend build`, and `git diff --check`.

The public MCP reference also documents when the management tool is available
and how to restore it before removing a stale association.
