---
status: current
system: tasks
requirements:
  - REQ-TASKS-TASK-TERMINALS-001
---

# Task Terminal Persistence System Design

## Purpose and boundaries

This design defines persistence-backend parity for ordinary task-terminal
records. The existing terminal service and WebSocket handlers continue to own
validation, PTY lifecycle, authorization, and client responses. The repository
continues to own the `user_terminals` table and its task-scoped lifecycle.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-TASK-TERMINALS-001` | [Portable query execution](#portable-query-execution), [Test contract](#test-contract) |

## Components and responsibilities

- `apps/backend/internal/terminal/repository` persists ordinary terminal
  descriptors and initializes `user_terminals`.
- `apps/backend/internal/terminal/service` preserves the existing ordinary,
  fixed, and script-terminal boundaries and delegates descriptor persistence to
  the repository.
- `apps/backend/internal/agent/handlers` exposes the existing `user_shell.*`
  WebSocket actions and propagates repository failures.

## Portable query execution

The terminal repository keeps `?` as the source placeholder syntax. Before a
parameterized statement reaches the database driver, the repository passes it
through the `sqlx.DB` associated with that operation:

- Writer operations use `r.db.Rebind`.
- Read operations use `r.ro.Rebind`.
- Schema statements contain no parameters and execute unchanged.

This follows the established repository convention. SQLite retains `?`
placeholders, while PostgreSQL receives numbered placeholders. Query text,
arguments, ordering, filters, and lifecycle semantics otherwise remain
unchanged.

## Failure and recovery

Database execution errors continue to carry the current repository operation
context to the terminal service and WebSocket caller. The repair does not add a
retry or convert a failed persistence operation into an in-memory terminal.

## Persistence

The `user_terminals` schema, unique task-sequence constraint, state values, and
delete behavior remain unchanged. This repair changes only how parameterized
statements are prepared for the selected driver and requires no migration.

## Test contract

An environment-gated PostgreSQL repository test uses
`internal/testutil.OpenIsolatedPostgres` and exercises the complete descriptor
lifecycle: create, get, filtered and unfiltered list, rename, state changes,
single delete, and task-wide delete. The existing SQLite tests continue to pin
sequence allocation and lifecycle behavior. Existing browser terminal tests
cover the unchanged WebSocket and web flow. The PostgreSQL test covers the only
driver-specific boundary changed by this repair.

## Related decisions

No architecture decision record is required. Query rebinding is an established
repository convention, and this repair introduces no new ownership or
persistence boundary.
