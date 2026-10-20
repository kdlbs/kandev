---
created: 2026-10-19
status: implemented
requirements:
  - REQ-TASKS-STEP-TRANSITION-READ-001
system_design:
  - ../../specs/tasks/system-design/task-step-transition-read-api.md
legacy_specs: []
---

# Implementation Plan: Task Step Transition Read API

## Overview

Expose a task's recorded step transitions with their attribution over
`GET /api/v1/tasks/:id/step-transitions`. One work order covers the
repository query, service method, handler, tests, and docs.

## Scope

### In scope

- [REQ-TASKS-STEP-TRANSITION-READ-001](../../specs/tasks/requirements/task-step-transition-read-api.md),
  AC `.1` through `.5`.
- Repository read, service authorization and paging, HTTP handler and route.
- Public task docs.

### Out of scope

- Attribution on behalf of a person, engine-enforced step exit policies,
  WebSocket, MCP, and plugin variants.

## Verification

- Go repository tests: oldest-first order with equal timestamps, attribution
  and step-name mapping, removed step, paging, empty task, Postgres parity.
- Go handler tests: owner read with field mapping and no ledger writes, cursor
  paging, foreign workspace, foreign organization, unknown task with and
  without authentication, unscoped callers, invalid limit and cursor.

## Work orders

- [Task 01: Task step transition read API](task-01-task-step-transition-read-api.md)
