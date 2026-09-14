---
id: "01-coordinator"
title: "Provider-neutral task change-request link coordinator"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001
acceptance_criteria:
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001.1
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001.2
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001.3
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001.4
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001.5
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001.6
system_design:
  - ../../specs/integrations/system-design/task-change-link-mcp.md
---

# Task 01: Provider-neutral task change-request link coordinator

## Summary

Add the backend MCP link contract for GitHub pull requests and GitLab merge
requests behind one provider-neutral coordinator: request validation,
workspace-scoped resolution, provider dispatch, and the resulting canonical
linked set returned to the caller.

## In scope

- `apps/backend/internal/mcp/handlers/task_change_link.go` and its tests.
- `apps/backend/internal/backendapp/task_change_link_coordinator.go` and its
  tests.
- Provider service entry points used by the coordinator in
  `apps/backend/internal/github` and `apps/backend/internal/gitlab`.
- GitLab unlink deletion-event publication consumed by
  `apps/backend/pkg/websocket/actions.go`.

## Out of scope

- Frontend rendering of the linked set beyond existing provider stores.
- Automation redesign or workflow moves.
- Direct database editing outside product migrations.

## Acceptance

- A bare pull-request or merge-request number is rejected before any store
  mutation, and fork versus canonical same-number identities stay distinct.
- A failed replacement removes the newly created association and reports both
  failures without leaving two active links.
- Unlinks emit the provider deletion event after persistence succeeds.

## Verification

```bash
cd apps/backend && go test ./internal/mcp/handlers ./internal/mcp/server ./internal/backendapp ./internal/github ./internal/gitlab ./internal/task/dto ./pkg/api/v1
```

## Results

Implemented and verified on branch `feature/manage-task-pr-and-m-e6i` at
`b1ada7b95dd471752799fbbe78ed6ce8b2794564`. Focused coordinator, handler,
server, GitHub, and GitLab suites passed, including race-enabled focused
identity, authorization, idempotency, rollback, detach, and projection tests.
