# Coordinator Task Authority Plan

## Objective

Deliver an explicit, revocable operator-granted capability system that allows a
"coordinator" task to orchestrate the board — stop/interrupt unrelated tasks,
attach workspace sources, and read documents/relations — without requiring a
parent/child topology. Default-off, flag-gated, audited, and fail-closed.

## Work Orders

| #   | Area          | Description                                                                   | Done |
| --- | ------------- | ----------------------------------------------------------------------------- | ---- |
| 01  | Persistence   | SQLite schema, interfaces, repository, dialect parity                         | ✓    |
| 02  | Authority     | `internal/coordinator/` central authority, capability check, flag gate, audit | ✓    |
| 03  | Call sites    | `stop_task.go`, `handlers.go`, `task_target_access.go`, `handoff_service.go`  | ✓    |
| 04  | Agent surface | MCP server descriptions, sysprompt injection                                  | ✓    |
| 05  | Operator API  | Gin handlers: grant CRUD, audit query                                         |      |
| 06  | Operator UI   | Settings tab: grants table, grant dialog, revoke, audit viewer                |      |
| 07  | Docs          | Public operator docs                                                          |      |
| 08  | Specs & Plan  | Requirements, system design, ADR, plan file                                   | ✓    |

## Dependencies

- Runtime flag `features.coordinatorTaskAuthority` must be OFF in all shipped profiles.
- Operator API and UI are independent of each other.
- Docs depend on having the full API surface to document.
