---
status: current
system: tasks
requirements:
  - REQ-TASKS-TASK-TERMINALS-001
  - REQ-TASKS-TASK-TERMINALS-002
---

# Task terminal persistence and ownership design

## Purpose and boundaries

This design defines persistence-backend parity for ordinary task-terminal
records. The existing terminal service and WebSocket handlers continue to own
validation, PTY lifecycle, authorization, and client responses. The repository
continues to own the `user_terminals` table and its task-scoped lifecycle.

This design also covers ordinary terminal selection and attachment across
shared environments. The tasks system owns this contract because terminal
records retain task ownership throughout their lifecycle. Desktop and phone
consume that ownership through shared domain state.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-TASK-TERMINALS-001` | [Portable query execution](#portable-query-execution), [Test contract](#test-contract) |
| `REQ-TASKS-TASK-TERMINALS-002` | [Terminal ownership](#terminal-ownership), [Browser result scopes](#browser-result-scopes), [Panel reconciliation](#panel-reconciliation) |

## Components and responsibilities

- `apps/backend/internal/terminal/repository` persists ordinary terminal
  descriptors and initializes `user_terminals`.
- `apps/backend/internal/terminal/service` preserves the existing ordinary,
  fixed, and script-terminal boundaries and delegates descriptor persistence to
  the repository.
- `apps/backend/internal/agent/handlers` exposes the existing `user_shell.*`
  WebSocket actions and propagates repository failures.
- `internal/gateway/websocket/terminal_handler.go` resolves authorized launch
  metadata before either local or remote PTY attachment.
- The session runtime slice owns browser terminal results. `useUserShells`
  coordinates reads. Desktop panels and mobile hooks consume those results.

## Terminal ownership

An ordinary row identifies its task and environment. A runtime execution
identifies an available transport. `ExecutionStore.GetByTaskEnvironmentID` can
return any execution in a shared environment, including a sibling task's execution.

The shell URL remains `/terminal/environment/:environmentId?terminalId=...`.
The gateway must not use that arbitrary execution's `TaskID` as terminal
ownership authority. It resolves the owner from persisted terminal metadata and
rejects a missing row, empty owner, or different environment.

A new internal terminal-service lookup returns only the owner for a validated
environment and terminal pair. It does not consume a launch command. The
gateway then calls `CheckTaskEnvironmentAccess` with that owner and environment.
Only after authorization does it call `ClaimShellLaunchInfo` with the resolved
owner. Existing environment execution checks still precede this path.

Local and remote attachment share `resolveOrdinaryTerminalLaunch`. Neither path
derives ownership from a client label, command, or incidental execution.
Explicit task mutations retain `requireOwnership` and reject cross-task calls.
One-shot consumption remains atomic and follows authorization.

`wsUserShellList` filters ordinary items by the requested environment.
`httpListTaskTerminals` applies that filter when an environment is supplied.
The task-only HTTP list keeps its existing task-wide behavior. Repository
`ListByTask` and service `List` remain task-wide APIs.

## Browser result scopes

`lib/state/user-shell-scope.ts` defines a typed scope containing
`environmentId` and nullable `taskId`. Tuple encoding distinguishes a taskless
read from every task read. There is no environment-only ordinary fallback.

The runtime slice separates two resource namespaces:

- `userShells.byScope` owns ordinary rows, dismissal tombstones, and loading
  and loaded state for each task/environment tuple.
- `userShells.byEnvironmentId` and its environment dismissal map contain only
  fixed, script, and supported unmanaged legacy terminals.

`setUserShells` partitions successful lists into these namespaces. A taskless
list cannot populate an ordinary scope. Old environment-only ordinary rows
cannot prove ownership and must be discarded rather than adopted.

`useUserShells` combines current ordinary rows with shared unmanaged rows
through stable selectors and memoization. Ordinary selectors and mutations
receive the same typed scope. Callbacks capture that scope at admission,
including MCP authentication terminal creation. A late completion can update its
captured cache but cannot bind a panel in a different active scope.

The read coordinator needs a publication identity distinct from request
identity. Shell publication uses the task/environment tuple. Restoration and
workspace generations remain part of request validity. Commit and diff grouping
remain unchanged. Same-tuple readers share one request. Different tasks cannot
replace each other's writer or settlement flags.

A successful current read supplies selection authority through its coordinator
snapshot. A failed first read settles initialization without a successful
snapshot. A failed refresh can preserve known rows from its own scope. It
cannot authorize creation or import rows from a taskless or sibling scope.

Environment cleanup removes all its ordinary scopes after the final session
reference disappears. Session-to-environment migration preserves known task
identity and invalidates ambiguous ordinary entries. It never promotes taskless
data into an owned scope.

## Panel reconciliation

The default initializer waits for a successful list in its current scope.
It validates `terminal-default` even when its ID is no longer `shell-default`.
A valid open ordinary selection remains selected. Otherwise, the initializer
selects the lowest-sequence open ordinary row in that scope. Parked, foreign,
and unknown rows are ineligible.

If no eligible row exists after a successful list, one initializer creates an
ordinary terminal. Creation ownership is per app store and scope. Concurrent
desktop or mobile consumers join that operation. Failure releases the guard.
Navigation retires panel publication without cancelling an admitted create.

Before applying parameters, the initializer rechecks the active scope and
panel identity. A late result enters only its original cache. It cannot
rewrite a reused panel or create a panel omitted by the effective layout.
Invalid extra ordinary panels are removed from the current layout without
calling park or stop on their terminal IDs.

Desktop `TerminalPanel` gates ordinary attachment until the current scope
validates the ID. It rejects conflicting task/environment stamps. Mobile
terminal construction resets on task changes even when the environment stays
the same. Saved active IDs remain selectable only within the owned list.
Fixed, script, dev-server, and agent paths retain their existing gates.

Desktop retains its terminal tabs and reopen menu. Phone retains the existing
full-height pane, `MobilePillButton`, `MobilePickerSheet`, and key bar. This
repair changes state selection without changing layout or touch targets.

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

Missing or unauthorized launch metadata retains the existing non-disclosing
terminal error. Transient retries and missing-environment close handling retain
the [target lifecycle contract](terminal-target-lifecycle.md). This correction
does not add another retry loop.

## Persistence

The `user_terminals` schema, unique task-sequence constraint, state values, and
delete behavior remain unchanged. Query rebinding and task selection require no
database migration. Browser caches remain transient. Persisted layouts keep
their format, and restored terminal references receive ownership validation.

## Test contract

An environment-gated PostgreSQL repository test uses
`internal/testutil.OpenIsolatedPostgres` and exercises the complete descriptor
lifecycle: create, get, filtered and unfiltered list, rename, state changes,
single delete, and task-wide delete. The existing SQLite tests continue to pin
sequence allocation and lifecycle behavior. Existing browser terminal tests
cover the unchanged WebSocket and web flow. The PostgreSQL test covers the only
driver-specific boundary changed by this repair.

Ownership coverage uses a mixed inventory: two tasks in one environment,
valid open rows, parked rows, and a foreign or missing ID. Gateway tests
exercise a correct terminal whose transport execution belongs to the other
task. Denial tests assert authorization before one-shot consumption. Deferred
hook tests cover navigation, concurrent readers, taskless reads, and late
callbacks. Desktop and phone E2E tests prove real shell output after task
switches and restoration of a contaminated default panel.

## Related decisions

No new architecture decision record is required. Ordinary terminal records
already own task identity. This correction applies that existing authority
without changing permissions or wire contracts.

## Implementation plans

- [Task terminal ownership repair](../../../plans/task-terminal-ownership/plan.md)
- [PostgreSQL query parity](../../../plans/postgres-task-terminal-queries/plan.md)
