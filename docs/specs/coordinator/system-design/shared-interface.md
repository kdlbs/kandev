---
id: coordinator-shared-interface-design
title: Phase 2 shared interface design
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-29
last_updated: 2026-09-29
requirements:
  - REQ-COORDINATOR-COORDINATORS-007
  - REQ-COORDINATOR-PERMISSIONS-001
  - REQ-COORDINATOR-PERMISSIONS-003
  - REQ-COORDINATOR-PROPOSAL-KINDS-001
  - REQ-COORDINATOR-ACTIVITY-LOG-001
---

# Phase 2 shared interface System Design

## Shared interface

Owned by phase-2 [task 01](../../../plans/workspace-coordinator-p2/task-01-shared-interface.md).
Every later work order builds on these signatures. The package is
`internal/coordinator` unless a path is given.

### Transactions

`coordinatorExec` (the store's existing interface in `store.go`) is the
transaction handle. Task 01 widens its method set to `ExecContext`,
`QueryRowContext` and `QueryContext` (`*sql.Conn` and `*sqlx.Tx` both have
all three), so `LoadWatchSet` can read several rows on the locked
transaction. Task 01 creates the per-dialect lock helper
`Store.withCoordinatorLock(ctx, coordinatorID, fn func(tx coordinatorExec) error) error`,
built from the same two mechanisms `PatchCoordinator` uses inline today: a
SQLite `*sql.Conn` opened with `BEGIN IMMEDIATE`, or a PostgreSQL `*sqlx.Tx`
that first ran `SELECT id FROM coordinators WHERE id = ? FOR UPDATE`. It
commits when `fn` returns nil and rolls back (with a cancel-proof ROLLBACK
on SQLite) otherwise. The phase-1 PATCH paths stay inline and are not
migrated. Statements run on the handle are rendered with `s.db.Rebind`
(the store's own dialect rebinder, not a parameter). No second abstraction
exists. When the coordinator row is missing under the lock the helper
returns `ErrNotFound` and `fn` does not run.

Rule for `fn`: it uses only the handle it is given (and pure computation).
It never calls `s.db`, `s.ro`, another store method, or any service method
that opens its own statement or transaction, because the SQLite writer pool
holds one connection and such a call blocks until the context ends. Reads
`fn` needs (for example a goal baseline) are performed before the lock or
through the handle. A test calls `withCoordinatorLock` with an `fn` that
runs a statement on the handle and asserts it commits, and the source scan
in the task 01 tests forbids `s.db.` inside functions passed to it.

### Transaction-bound proposal writes

The phase-1 store methods keep their exact signatures and behaviour and
become thin wrappers over transaction-bound variants, so no phase-1 test
call site changes for them (`*sqlx.DB` satisfies `coordinatorExec`):

```go
func (s *Store) CompleteProposalTx(ctx context.Context, exec coordinatorExec, id, token, taskID string, now time.Time) (bool, error)
func (s *Store) FailProposalTx(ctx context.Context, exec coordinatorExec, id, token, errMsg string, now time.Time) (bool, error)
func (s *Store) RejectProposalTx(ctx context.Context, exec coordinatorExec, id, reason, decidedBy string, now time.Time) (bool, error)
func (s *Store) InsertProposalWith(ctx context.Context, p *Proposal, phase2 bool, inTx func(tx coordinatorExec) error) error
```

`CompleteProposal`, `FailProposal` and `RejectProposal` call their `Tx`
variant with `s.db`. `InsertProposal(ctx, p, phase2)` calls
`InsertProposalWith(ctx, p, phase2, nil)`; `InsertProposalWith` runs `inTx`
inside the insert's own locked transaction after the cap check and the row
insert, and an `inTx` error rolls the whole insert back.

Only the service switches. With `phase2` false the service calls the
phase-1 methods exactly as today, takes no extra lock and writes no
activity. With `phase2` true, the service runs each of the claim-fenced
completion, the failure write and reject inside
`withCoordinatorLock(ctx, coordinatorID, fn)`, where `fn` calls the `Tx`
variant and, only when it reports a matched row, `Record` on the same
handle; a `Record` error fails `fn` and rolls the status write back
(`AC-COORDINATOR-ACTIVITY-LOG-001.6`). Propose passes `inTx` that calls
`Record` for the `proposed` row. When `withCoordinatorLock` returns
`ErrNotFound` (the coordinator was deleted), the service treats it as a zero
matched row and goes through the existing `settleWriteRace` path (404 with
the existing info log, no activity written). A zero matched row inside `fn`
commits nothing and writes no activity.

### Open-proposal counting and the flag-off kind predicate

`Store.CountOpenProposals(ctx, coordinatorID string, phase2 bool)` (reader
pool, the phase-1 signature plus the trailing argument) and
`Store.CountOpenProposalsTx(ctx, exec coordinatorExec, coordinatorID string, phase2 bool)`
(the propose transaction) share one query and are the one count behind both
the propose-time cap of 25 and the `open_proposals` field. Non-transactional
callers (`service.go`, `stalls.go`) use the reader-pool form.
With `phase2` false it adds `AND kind = 'create_task'`, so stored resume,
message and move proposals neither count toward the cap nor appear in
`open_proposals`; with `phase2` true it counts every kind. The cap and the
field can therefore never disagree.

Task 01 owns the `kind` predicate on every read a flag-off phase-1 path
makes: `ListProposals`, `GetProposal`, the decision routes' row read,
`ListApprovingClaimedBefore` (startup pass), `ReclaimStale` (stale-claim
sweep) and the by-id read of `get_coordinator_item_kandev`. Each store
method takes `phase2 bool` as a trailing argument; the only change to a
phase-1 test is that added `false` argument at its direct store call sites.
`InsertProposal`, which runs the cap, takes the same trailing `phase2 bool`
and passes it to `CountOpenProposalsTx`; the service supplies its own value.
With it false, a non-create id is the phase-1 not-found result, nothing is
claimed, and no row changes. Task 04 extends the sweep for the other kinds
and keeps the parameter. Tests: 25 stored open
`resume` rows with `phase2` false do not block a `create_task` propose and
`open_proposals` is 0; 25 open `create_task` rows refuse the 26th; with
`phase2` true 24 create plus 1 resume rows count 25; a stored `approving`
resume row is neither reclaimed nor listed by the flag-off sweep and startup
pass; the by-id tool read of a resume id is not found.

### Signatures

```go
type Store struct{ /* existing */ }
func (s *Store) CountOpenProposals(ctx context.Context, coordinatorID string, phase2 bool) (int, error)
func (s *Store) CountOpenProposalsTx(ctx context.Context, exec coordinatorExec, coordinatorID string, phase2 bool) (int, error)
func (s *Store) LoadWatchSet(ctx context.Context, exec coordinatorExec, coordinatorID string) (WatchSet, error)
func (s *Store) ActiveStandingOrders(ctx context.Context, coordinatorID string) ([]StandingOrder, error)
func (s *Store) ActiveGoal(ctx context.Context, coordinatorID string) (*Goal, error)
func (s *Store) LastMetGoal(ctx context.Context, coordinatorID string) (*Goal, error)
func (s *Store) InsertActivity(ctx context.Context, exec coordinatorExec, row ActivityRow) error
func (s *Store) MarkUndone(ctx context.Context, exec coordinatorExec, rowID, undoneBy string, at time.Time) (bool, error)

func (s *Service) Policy(ctx context.Context, coordinatorID string) (PolicyView, error)
func (s *Service) Record(ctx context.Context, exec coordinatorExec, row ActivityRow) error
func (s *Service) RecordRefusal(ctx context.Context, coordinatorID, workspaceID string, actionClass Action, reasonCode string) error
func (s *Service) resetConversation(ctx context.Context, exec coordinatorExec, coordinatorID string) (archiveTaskID string, err error)
func (s *Service) archiveConversation(ctx context.Context, coordinatorID, taskID string)
func NewService(store *Store, validator *Validator, authorizer WorkspaceAuthorizer, log *logger.Logger, opts ...ServiceOption) *Service
func WithPhase2(on bool) ServiceOption
```

- `ActivityRow` is the struct of the `coordinator_activity` columns of
  [activity log](activity-log.md#store): `ID`, `CoordinatorID`,
  `WorkspaceID`, `ActionClass Action`, `Outcome`, `Authorization`,
  `TargetTaskID`, `ProposalID`, `ActorUserID`, `ReasonCode` (each a nullable
  string), `Detail`, `Edited`, `RefusalCount`, `UndoneAt`, `UndoneBy`,
  `UndoOfID`, `CreatedAt`, `UpdatedAt`, with the JSON names of the columns.
  `InsertActivity` assigns `ID` (a new UUID) when empty, `CreatedAt` and
  `UpdatedAt` (UTC now) when zero, and `RefusalCount` 1 when zero; it
  truncates `Detail` to 1,000 runes; and it returns a wrapped
  `ErrInvalidActivity` without writing when `ActionClass`, `Outcome` or
  `Authorization` is outside its set, or `CoordinatorID` or `WorkspaceID` is
  empty. `RecordRefusal` with an empty `reasonCode` or an `actionClass`
  outside the six actions and `unknown` returns `ErrInvalidActivity` and
  writes nothing. `InsertActivity` increments the
  `coordinator_activity_rows_total` counter named in
  [activity log](activity-log.md#observability) after a successful insert.
- `NewService` gains only a trailing variadic option, so every phase-1 call
  site and test compiles unchanged; `phase2` defaults to false.
- `Service.Record` and `Service.RecordRefusal` return nil and write nothing
  when `phase2` is false; the store methods are ungated. Every writer goes
  through the service.
- `RecordRefusal` lives in the `coordinator` package. The guard in
  `internal/mcp/handlers` reaches it through an interface
  `RefusalRecorder { RecordRefusal(ctx, coordinatorID, workspaceID string, actionClass Action, reasonCode string) error }`
  that `*Service` satisfies. Five parameters including `ctx`; the earlier
  four-argument form in the permissions design is retired.
- `resetConversation` runs inside the caller's locked transaction: sets
  `conversation_task_id` NULL, sets `config_revision = config_revision + 1`
  and `updated_at` to now, and returns the previous `conversation_task_id`
  (empty when none). It always increments. The caller
  invokes `archiveConversation(ctx, coordinatorID, taskID)` after commit;
  that function is a thin wrapper over the existing
  `archiveClearedConversationTask` (`conversation.go`), which is not changed,
  so the warn-and-startup-pass handling of an archive failure is inherited.
  It is never called with an empty id. The phase-1 PATCH keeps its inline
  clear at `store.go` and is deliberately not moved, so phase-1 tests are
  untouched. That PATCH increments `config_revision` only when it changed
  `context`, `agent_profile_id` or `executor_profile_id`
  ([coordinators](coordinators.md#routes)). A parity test asserts both paths
  leave the same `conversation_task_id` and advance `updated_at`, and change
  no other column; the only allowed difference is `config_revision`, which
  `resetConversation` increments unconditionally and PATCH increments only
  for a config-field change (both cases tested).
- `MarkUndone` runs `UPDATE coordinator_activity SET undone_at = ?, undone_by
  = ?, updated_at = ? WHERE id = ? AND undone_at IS NULL` and reports whether
  a row changed. Task 01 owns it; the undo route's work order calls it.

### Policy

`ParsePolicy(raw *string) (Policy, error)` is pure and never logs. Result
table (a returned error is `ErrPolicyUnreadable`, wrapped):

| Stored value | Policy | Error |
| --- | --- | --- |
| NULL | `PhaseOnePolicy()` | nil |
| empty or whitespace, invalid JSON, `actions` null or not an object | all six `denied` | yes |
| `version` other than 1 | all six `denied` | yes |
| valid, an action absent | that action `denied` | nil |
| valid, an unknown action key | key dropped | nil |
| valid, a setting that is null or outside the three | that action `denied` | nil |

`Allows` on an `Action` outside the six (including `ActionUnknown`) is
false. `ActionUnknown Action = "unknown"` is not in the ordered `AllActions`
list that `Validate` and the activity classes iterate. `ActionForTool(name
string) Action` in `toolprofile.go` maps a propose tool to its action and
returns `ActionUnknown` for every other name, read tools included; the
guard uses it to assign the `unknown` class.

The once-per-coordinator error log lives in `Service.policyFor`, which knows
the coordinator id: a `sync.Map` keyed `coordinatorID:policy_revision`, so a
save that changes the revision logs again. `ParsePolicy` callers other than
`Service` do not log.

`PolicyView` is the ADR "What phase 3 reads" shape:

```go
type PolicyView struct {
    CoordinatorID  string             `json:"coordinator_id"`
    WorkspaceID    string             `json:"workspace_id"`
    PolicyRevision int                `json:"policy_revision"`
    Actions        map[Action]Setting `json:"actions"` // always all six keys
    WatchScope     string             `json:"watch_scope"`
    WorkflowIDs    []string           `json:"workflow_ids"` // sorted ascending, never nil
}
```

`Service.Policy` returns the zero `PolicyView` and `ErrNotFound` for a
missing coordinator. With `phase2` false it returns `PhaseOnePolicy()`
actions, scope `all`, empty workflow ids and `PolicyRevision` 0 regardless of
stored data. With `phase2` true and an unreadable stored policy it returns
a full view (all six `denied`, the stored revision, the stored scope and
watches) and a nil error, after the once-per-revision error log of
`policyFor`; the caller enforces `denied`, and only a failed query returns an
error with the zero view. A coordinator GET or list of an unreadable stored
policy reports the same view. For a PUT that carries `policy`, an unreadable
stored policy always counts as differing from the request, so the write
happens even when the request equals the all-`denied` view the GET showed
(the policy is replaced and the revision increments); a PUT without `policy`
leaves it as it is.

### Watches

```go
type WatchSet struct {
    All         bool
    WorkflowIDs []string // sorted ascending; non-nil, empty when All
}
func (w WatchSet) Contains(workflowID string) bool // "" is never contained
```

`LoadWatchSet` reads `watch_scope`, then when `selected`
`SELECT workflow_id FROM coordinator_watches WHERE coordinator_id = ? ORDER
BY workflow_id ASC`. A missing coordinator is `ErrNotFound`. A `selected`
scope with zero rows is `WatchSet{All: false, WorkflowIDs: []}` and contains
nothing. A failed query returns the error, and a coordinator GET or list whose watch
or policy read fails responds 500; the guard treats it as refuse
with the phase-1 not-found error, logs at error, and writes no activity row.
The settings `workflow_ids` and `PolicyView.WorkflowIDs` use the same order.
A save with scope `all` deletes every watch row and inserts none.

### Standing-order and goal reads

- `ActiveStandingOrders`: `WHERE coordinator_id = ? AND retired_at IS NULL
  ORDER BY created_at ASC, id ASC`; an empty non-nil slice for none or for a
  missing coordinator; a query error is returned.
- `ActiveGoal`: the row with `status = 'active'`, or `nil, nil`. `LastMetGoal`:
  `status = 'met' ORDER BY met_at DESC, id DESC LIMIT 1`, or `nil, nil`.
  Both return an error on a failed query.
- Defaults: `coordinator_goals.status` default `'active'`, `baseline_json`
  default `'{}'`, `criteria_json` default `'[]'`; index `(coordinator_id,
  created_at, id)` beside the partial unique active index.

`ActiveGoal` and `LastMetGoal` return `nil, nil` for a coordinator that has
no matching goal or does not exist; callers that need the not-found result
read the coordinator first.

`Goal` JSON: `{id, coordinator_id, name, due_on, status, criteria:
[{id, text, done}], baseline, set_at, met_at, met_by, created_at,
updated_at}`, `due_on`, `met_at` and `met_by` null when unset, `criteria`
always an array. The store-level `StandingOrder` is the row shape `{id,
coordinator_id, text, created_by, created_at, retired_at, retired_by,
source_proposal_id, last_applied_at}` and is not serialised by any route.
The routes serialise the wire shape `{id, number, text, created_at,
created_by_name, retired_at, last_applied_at}` of
[standing orders](standing-orders.md#routes) for the list, create, retire and
restore responses; the typed client uses only that wire shape, and
`number` and `created_by_name` are filled by the route from the row order and
the user lookup. The guided setup
201 body is the coordinator DTO of `POST .../coordinators`; setup adds no
other body. The typed client covers only routes whose bodies the designs
define.

### Proposal wire fields

`ProposalDTO` gains `kind` (string), `target_task_id` (string or null),
`standing_order_ids` (string array, `[]` when none), `starts_agent`
(boolean) and `outcome` (the parsed `outcome_json` object, null when NULL).
`spec` stays `ProposalSpec` for `create_task`; for other kinds it is the
kind's JSON of [proposal kinds](proposal-kinds.md#store). The TypeScript
`Proposal` is a discriminated union on `kind` with one `spec` type per kind.

### Enumerations

No enumeration is a database CHECK constraint (SQLite cannot add one with
an additive `ALTER`, and both dialects must match). Policy settings and
actions, proposal `kind`, `status`, `outcome`, activity `action_class`,
`outcome`, `authorization`, and `watch_scope` are validated by the writer
before the row is written; nothing writes an unknown one. Readers of
policy fail closed as [Policy](#policy) tabulates. `LoadWatchSet` reads any
`watch_scope` other than `all` as `selected`, so an unknown scope watches
only its stored rows, none when there are none. A stored proposal `kind`
outside the four is listed only with `phase2` true, as a card whose `kind`
is the stored string and whose `spec` is the raw stored JSON; the TypeScript
union has no branch for it, so the web renders it as an unsupported card
with no action. With `phase2` true, `GET proposals/:pid` returns the same
raw card; approve returns 500 and writes nothing, and claims nothing (the
registry has no executor, per [proposal kinds](proposal-kinds.md#executors));
reject is allowed on a pending or failed row and marks it `rejected`, which
frees its cap slot. `CountOpenProposalsTx` counts it like any other open row
with `phase2` true. With `phase2` false it is invisible, like every
non-create kind. An unknown activity `action_class`, `outcome` or
`authorization` is returned to the list as stored. No code path writes
either.

### Deletion and locking

`DeleteCoordinator` deletes children before the parent, in one transaction:
watches, activity, standing orders, goals, proposals, then the coordinator.
On PostgreSQL it first takes the per-coordinator lock (`SELECT ... FOR
UPDATE` on the coordinator row). On SQLite the one-connection writer pool
serialises the same way.

The workspace-deletion transaction deletes every phase-2 table by its own
`workspace_id` (all of them carry the column: watches, activity, standing
orders, goals), then the phase-1 `coordinator_stalls` rows and
`coordinator_proposals` by `workspace_id` as in phase 1, then the
coordinators, child-first. It never uses a subquery on `coordinators`, so
rows orphaned by a phase-1 binary deleting a coordinator are removed too.
On PostgreSQL it first runs `SELECT id FROM coordinators WHERE workspace_id
= ? ORDER BY id FOR UPDATE`, taking every coordinator lock of the workspace
in id order before any delete, so a concurrent `withCoordinatorLock` writer
either commits before the delete and is removed with it, or blocks and then
finds no coordinator. `coordinator_stalls` has no `coordinator_id` (it is
keyed by `task_id`), so it is deleted only by `workspace_id`, never by
`DeleteCoordinator`. A test runs `Record` inside `withCoordinatorLock`
against workspace deletion on both dialects and asserts no row of any
phase-2 table survives for the workspace.

`Service.Record` only inserts inside the transaction it is handed; it does
not lock. Precondition: every caller of `Record` holds the per-coordinator
lock (`withCoordinatorLock` or the equivalent inline lock) for that
transaction, including the completion and failure writes of the claim-fenced
proposal transaction, which take it before their `UPDATE`. Under that
precondition a `Record` either commits before the delete and is removed with
it, or blocks and then finds the coordinator missing (`ErrNotFound`, nothing
written). `InsertActivity` itself does not check the coordinator row. The
concurrent test runs `Record` inside `withCoordinatorLock` against
`DeleteCoordinator` and asserts no activity row survives. Activity rows are found by
`workspace_id` through the index `coordinator_activity(workspace_id)`. Orphan
rows left by a phase-1 binary deleting a coordinator are unreachable (every
read is scoped by coordinator) and are removed by workspace deletion, and
activity orphans also by retention.

### Downgrade and upgrade

All new columns are nullable or defaulted and no constraint restricts a
`create_task` row, so a phase-1 binary against the phase-2 schema inserts and
reads proposals and coordinators without change. Downgrading while
non-`create_task` proposals are stored is unsupported: a phase-1 binary does
not know the kind column, so it lists and counts those rows as create cards
and cannot decide them correctly. Operators turn the flag off, not the
binary, to keep the rows dormant. The upgrade test builds the
phase-1 schema from hand-written DDL in the test (the `v0.93.0` fixture lists
`coordinator` as known-missing, so it cannot seed it), seeds a coordinator and
a proposal, and runs from two starting points: with `config_revision` and
without it; both end with identical schemas. Migration order: phase-1
migrations, then the phase-2 `ADD COLUMN`s, then `CREATE TABLE`s, then indexes
on the new columns (never in `createTablesSQL`). The PostgreSQL leg runs under
the same environment gate as the existing PostgreSQL store tests. A test runs
phase-1 statements (insert, list, decide) against the migrated schema.
