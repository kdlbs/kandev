# Task 05 — Operator API: Coordinator Grant Handlers

## Owner

Backend

## Predecessors

01 (Persistence), 02 (Authority)

## Description

Add Gin route group under `/api/v1` for managing coordinator grants:

- `GET|POST /api/v1/workspaces/:id/coordinator-grants`
- `DELETE /api/v1/coordinator-grants/:grantId`
- `GET /api/v1/tasks/:id/coordinator-grants`
- `GET /api/v1/workspaces/:id/coordinator-audit?limit=&task_id=`

Each call runs `authorizeWorkspaceID` first. POST rejects invalid task IDs,
unknown capabilities, and workflows outside the workspace.

## Verification

- `coordinator_grant_handlers_test.go` — grant/list/revoke/audit
- Cross-workspace returns 404
- Unauthenticated rejection
- Invalid capability/scope rejected
