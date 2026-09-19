---
id: "05-operator-api"
title: "Operator API: coordinator grant handlers"
status: done
wave: 5
depends_on:
  - "01-persistence"
  - "02-authority"
plan: "plan.md"
requirements:
  - REQ-TASKS-COORDINATOR-AUTHORITY-001
acceptance_criteria:
  - AC-TASKS-COORDINATOR-AUTHORITY-001.1
  - AC-TASKS-COORDINATOR-AUTHORITY-001.2
  - AC-TASKS-COORDINATOR-AUTHORITY-001.3
  - AC-TASKS-COORDINATOR-AUTHORITY-001.4
system_design:
  - ../../specs/tasks/system-design/coordinator-task-authority.md
---

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
