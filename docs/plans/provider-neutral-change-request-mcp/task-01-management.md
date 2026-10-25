---
id: "01-management"
title: "Shared management contract"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001
  - REQ-INTEGRATIONS-TASK-CHANGE-LINK-MCP-003
acceptance_criteria:
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001.1
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001.2
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001.3
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001.4
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001.5
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001.6
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-003.1
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-003.2
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-003.3
system_design:
  - ../../specs/integrations/system-design/task-change-link-mcp.md
---

# Task 01: Shared management contract

## Summary

Expose `manage_task_change_request_kandev` with strict operation branches and complete canonical identity.
Preserve the merged coordinator and its provider cleanup contracts.

## In scope

- Register the neutral tool and backend action; retain legacy registration until task 05.
- Require trusted principal and same-workspace target; reject forged internal caller identity.
- Preserve same-provider replacement, exact no-op, absent unlink, stale removal, and preexisting-new-link compensation ownership.
- Carry typed failure/readback state through WS and MCP errors without exposing raw provider errors.
- Extend both-provider failure tests: new-link failure, old-unlink failure, compensation success/failure, preexisting new link, and failed final listing.

## Out of scope

Provider-store redesign, new UI, cross-provider replacement, and GitLab outcome tracking.
Do not change unrelated provider algorithms or historical transcripts.

## Acceptance

- Strict schema rejects unknown keys, fractional numbers, partial old identity, and old fields on link/unlink before writes.
- Both providers retain persisted exact cleanup and committed deletion events after restart.
- Replacement returns original/rollback failures and known links without reporting uncertain state as success.

## Verification

Run from the repository root. Add failing tests first, then implement and run these checks.
Use existing package fixtures; keep all new tests inside this work order's listed suites.

```bash
(cd apps/backend && go test ./internal/mcp/server ./internal/mcp/handlers ./internal/backendapp -count=1)
(cd apps/backend && go test ./internal/github ./internal/gitlab -run 'TaskPRDetach|DetachTaskPR|UnlinkTaskMR|TaskChange|AssociateExisting' -count=1)
(cd apps/backend && go test -race ./internal/backendapp -run 'TaskChange|ChangeRequest' -count=1)
```

## Files likely touched

- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/handlers.go`
- `apps/backend/internal/mcp/server/task_change_request_tools_test.go (new)`
- `apps/backend/internal/mcp/handlers/task_change_link.go`
- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/mcp/handlers/task_change_request_test.go (new)`
- `apps/backend/internal/backendapp/task_change_link_coordinator.go`
- `apps/backend/internal/backendapp/task_change_link_coordinator_test.go`
- `apps/backend/pkg/websocket/actions.go`

## Dependencies

None. Read the merged baseline before changing code.

## Risks

Compensation must not remove a preexisting association. A committed write can outlive a failed readback.

## Parallelism

`sequential`. Shared schemas and registration files prevent parallel ownership.

## Inputs

- [Requirements](../../specs/integrations/requirements/task-change-link-mcp.md), frontmatter IDs.
- [System design](../../specs/integrations/system-design/task-change-link-mcp.md), relevant contract sections.
- [Plan](plan.md), baseline, test matrix, and agent requests.
- Existing adjacent tests named in the plan and `apps/backend/AGENTS.md`.
- `.agents/skills/tdd/SKILL.md` and its backend testing reference before implementation.

## Results

Implemented the neutral management action and tool with strict operation and
identity validation, trusted principal and workspace checks, and typed
replacement failure state with operation and rollback errors. The final
cutover removed the legacy MCP registrations while retaining the old backend
transport handlers for the documented transition release.

Validation passed:

- `go test ./internal/mcp/server ./internal/mcp/handlers ./internal/backendapp -count=1`
- `go test ./internal/github ./internal/gitlab -run 'TaskPRDetach|DetachTaskPR|UnlinkTaskMR|TaskChange|AssociateExisting' -count=1`
- `go test -race ./internal/backendapp -run 'TaskChange|ChangeRequest' -count=1`
- Review regression commands passed:
  - `go test ./internal/mcp/handlers -run 'TestManageTaskChangeRequest' -count=1`
  - `go test ./internal/mcp/server -run 'TestManageTaskChangeRequest|TestTaskChangeRequestTool' -count=1`
  - `go test ./internal/backendapp -run 'TestTaskChangeManagementDispatches|TestTaskChangeCoordinator' -count=1`
  These cover rich replacement old-identity forwarding, serialized rollback
  and unknown-state details, and WebSocket handler-to-coordinator success and
  compensation paths.
- Additional coordinator regressions cover exact cleanup when a legacy GitLab
  row is resolved from its stored host and project path, while unrelated
  unresolved rows remain visible without blocking the requested mutation.
