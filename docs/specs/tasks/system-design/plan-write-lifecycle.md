---
status: current
system: tasks
requirements:
  - REQ-TASKS-DOCUMENTS-001
created: 2026-08-28
owners:
  - kandev
---
# Task plan write lifecycle System Design

## Purpose and boundaries

This design defines the missing-task boundary for backward-compatible task-plan
writes. It does not implement the broader task-documents migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-DOCUMENTS-001` | [Transactional write](#transactional-write) and [Error contract](#error-contract) |

## Components and responsibilities

- `internal/task/repository/sqlite.Repository` owns the transaction and database
  error classification for SQLite and PostgreSQL.
- `internal/task/service.PlanService` owns operational logging and preserves
  typed repository errors.
- `internal/task/planws` owns the shared WebSocket error contract for browser
  handlers and MCP handlers.

## Transactional write

`WritePlanRevision` writes the plan head before it writes or merges a revision.
Both plan tables reference the task row with foreign keys.

If the head write reports a foreign-key violation, the repository returns the
shared `ErrTaskNotFound` sentinel. The transaction then rolls back. No plan head
or revision remains.

The repository uses `internal/db.IsForeignKeyViolation` for both supported
database dialects. It does not inspect a raw database error outside that helper.

## Error contract

The plan service passes `ErrTaskNotFound` to its callers. It records the
expected rejection at debug level. Other write errors remain error-level
entries.

`planws.CreateError` and `planws.UpdateError` map `ErrTaskNotFound` to the
existing `not_found` WebSocket code. The response does not include a database
constraint message.

The browser and MCP surfaces use the same `planws` mapping. No request or
success payload changes.

## Failure and recovery

A concurrent task deletion can occur after access validation and before the
plan transaction. The foreign-key classification closes that race without a
separate task existence query.

An unrelated database error keeps its wrapped diagnostic context. The service
records it at error level and the wire contract returns `internal_error`.

## Test strategy

SQLite and PostgreSQL repository tests cover the sentinel and rollback. Service
tests cover log severity. Shared contract tests cover browser and MCP mappings.
