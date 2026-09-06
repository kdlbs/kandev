---
status: current
system: office
requirements:
  - REQ-OFFICE-COORDINATOR-AUTHORITY-001
  - REQ-OFFICE-COORDINATOR-AUTHORITY-002
  - REQ-OFFICE-COORDINATOR-AUTHORITY-003
  - REQ-OFFICE-COORDINATOR-AUTHORITY-004
  - REQ-OFFICE-COORDINATOR-AUTHORITY-005
created: 2026-09-06
owners:
  - kandev
---

# Taskless Coordinator Authority System Design Part 1

## Purpose and boundaries

This design records the authority boundary for a scheduled, unattended Office run and
the mechanism that enforces it. It owns the runtime capability vocabulary, the
task-scope derivation and the annotation predicate. The board-read endpoint's wire
contract — route, parameters, response, ordering, error mapping and CLI — is
[part 2](taskless-coordinator-authority-02.md). Neither part owns run launching, session
identity, or the workflow engine.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-COORDINATOR-AUTHORITY-001` | [Vocabulary change](#vocabulary-change), [part 2 Board read](taskless-coordinator-authority-02.md#board-read) |
| `REQ-OFFICE-COORDINATOR-AUTHORITY-002` | [part 2 Request and response contract](taskless-coordinator-authority-02.md#request-and-response-contract) |
| `REQ-OFFICE-COORDINATOR-AUTHORITY-003` | [Task-scope derivation](#task-scope-derivation), [Snapshot semantics](#snapshot-semantics) |
| `REQ-OFFICE-COORDINATOR-AUTHORITY-004` | [Three scope classes](#three-scope-classes) |
| `REQ-OFFICE-COORDINATOR-AUTHORITY-005` | [What stays denied](#what-stays-denied) |

## Prior art

**Leg 1, compiled wiki.** Searched: vault `/Users/henry/Documents/henry/wiki` (from
`~/.obsidian-wiki/config.henry`), QMD collection `wiki`. **The leg did not run.**
`/Users/henry/Documents` is unreadable from this execution environment (`ls: Operation
not permitted`, on the vault path and on the parent), and neither `obsidian-wiki` nor
`qmd` is on `PATH`, so the GraphRAG pre-pass, the QMD pass and the grep fallback were
all unavailable. A tool-availability failure, not an empty result: the vault may hold a
position on unattended-agent authority this spec has not consulted.

**Leg 2, `saas-kb`.** Searched: nothing. `search_fsm_docs` is not exposed in this
session (only `codex` and `kandev` MCP servers are present, and no tool-discovery
mechanism surfaced it). **The leg did not run.** No cross-vendor comparison informed
this contract.

**Leg 3, substitute, in-repo prior reasoning.** Both external legs being unavailable,
the frozen in-repo contracts were read instead, and they constrain this design.
`agents-03.md` froze that step-move and archive "fail before making an HTTP request ...
because no signed Office runtime capability exists", preserved by
[What stays denied](#what-stays-denied). `tasks-01.md` ("Permissions") froze
`AllowedTaskIDs` as the rule stopping "a worker on task T1 from mutating task T2 just
because both belong to its workspace", which is why `WildcardTaskScope` is refused.
`runtime-01.md` froze capabilities as taken at run-claim time and kept by an in-flight
JWT; the scope snapshot is that shape plus the reuse rule in
[Snapshot semantics](#snapshot-semantics). `office/AGENTS.md` ("Participant slate: one
builder") records the trap the step-decision exclusion avoids: a second answer to a
question that has exactly one.

**What we are doing differently.** Nothing departs from a documented position. The one
place this could have gone wider, `WildcardTaskScope` for taskless runs, is rejected
because `tasks-01.md` already decided against workspace-wide mutation.

## The read that already exists

`agentctl kandev tasks list` (`cmd/agentctl/kandev_tasks.go`) issues
`GET /api/v1/office/workspaces/$KANDEV_WORKSPACE_ID/tasks` — the dashboard route, not
`/runtime/*`. `AgentAuthMiddleware` accepts any valid agent JWT whose workspace claim
equals `:wsId`, and `buildEnvVars` (`internal/office/service/env_builder.go`) sets
`KANDEV_WORKSPACE_ID` on every run, taskless included. So the board read is reachable
today with no capability check, no `runtime.action` event, and a workspace taken from
the URL rather than the token claim.

## What already works taskless

`CanMutateTask` (`internal/office/runtime/context.go`) is consulted at exactly four
sites in `internal/office/runtime/actions.go`: `PostComment`, `UpdateTaskStatus`,
`CreateSubtask`, and `CreateTask` when `parent_task_id` is set. Every other runtime
action is workspace- or agent-scoped and already succeeds on a taskless run: root
`create_task`, `create_project`, `create_agent`, `modify_agents`, `spawn_agent_run`,
`request_approval`, `read_memory`, `write_memory`, `list_projects`, `list_skills`,
`delete_skills`.

## The authority boundary

One table, decided once. Every row answers "what may a scheduled coordinator do?"

| Action class | Keys | Scope for a taskless run | Why |
|---|---|---|---|
| Board read | `list_tasks` (new) | The run's workspace, from the token claim | Already reachable ungoverned via the dashboard route; bringing it inside the contract adds the gate and the audit trail without adding reach. |
| Annotation | `post_comment` | The run's workspace, **taskless runs only** | The routine's stated job, and unreachable any other way. A task-bound run keeps today's own-task-only reach. |
| State mutation | `update_task_status`, `create_subtask`, parented `create_task` | The runner set only | `tasks-01.md` already decided workspace membership must not authorize mutating another task. |
| Workspace-scoped | root `create_task`, `create_project`, `create_agent`, `modify_agents`, `spawn_agent_run`, `request_approval`, `read_memory`, `write_memory`, `list_projects`, `list_skills`, `delete_skills` | Unchanged | These never consulted `CanMutateTask` and already succeed on a taskless run. |
| Denied | step move, archive, step decision | None | Frozen by `agents-03.md`; step decision belongs to the session-bound MCP tool. |

`list_tasks` is granted to every role by default rather than gated on a permission: the
read is already reachable today, so defaulting it off would remove capability installs
already have while adding no safety.

### The parity argument, and why it is retired

Workspace-wide annotation for **every** run is rejected, and so is the parity argument for
it: that it grants no power beyond `spawn_agent_run`, which is already workspace-wide and
unattended-reachable. The observation is factually correct — `SpawnAgentRun` checks only
`target.WorkspaceID == runCtx.WorkspaceID`, with no task scoping at all — but it does not
follow that a new grant may match it. `spawn_agent_run`'s breadth is a known
runaway-containment gap, tracked separately for a causation cap and a per-workspace launch
budget; a capability that is already too permissive is an argument for narrowing that one,
never a licence for the next grant to copy it. Reasoning by parity with the loosest
existing capability ratchets the floor upward every time it is used.

The grant is therefore bounded by need instead. A scheduled coordinator whose job is to
surface blockers cannot do it without commenting on tasks it does not run; a task-bound
worker has no such need. So the widening is taskless-only, and the branch lives in the
enforcement predicate rather than implicitly in who holds `post_comment` — which is what
makes the boundary checkable rather than merely intended.

The other rejected alternative is `WithTaskScope(WildcardTaskScope)` for taskless runs.
It is the one-line fix, and it grants an unattended agent write authority over every task
in the workspace, the exact case `tasks-01.md`'s `AllowedTaskIDs` rule exists to prevent.

## Vocabulary change

`internal/office/runtime/capabilities.go` gains one key, `CapabilityListTasks =
"list_tasks"`, with a matching `CanListTasks` field on `Capabilities`, a `case` in
`Allows`, an entry in `AllowedKeys`'s stable-order list, and an unconditional `true` in
`FromAgent` beside `CanListProjects` and `CanListSkills`.

`Allows`'s `default: return false` is preserved: the vocabulary stays closed, and this
is a deliberate extension rather than an opening of it. No `shared.Permissions` key is
added — `list_tasks` is capability-only, so `AllPermissionKeys` and
`ValidateNoEscalation` are untouched.

`AllowedKeys` ordering is load-bearing: it is rendered into the prompt as
`AllowedActions` (`assembleAgentPrompt`). `list_tasks` is inserted next to the other
read keys so the prompt's action list stays grouped by kind.


## Task-scope derivation

`ContextBuilder.Build` currently does:

```go
taskID := payload["task_id"]
caps := FromAgent(agent).WithTaskScope(taskID)
```

Three changes.

**Tasklessness is decided once, by trimming.** `strings.TrimSpace(payload["task_id"])`
is computed before any branch, and its emptiness is the single definition of "taskless"
used by both this derivation and the annotation predicate. The trimmed value is also what
`RunContext.TaskID` carries (AC-003.3), so the `taskID == c.TaskID` short-circuit in
`CanMutateTask`, the `input_snapshot`, and AC-004.12's "its own bound task" all agree on
one spelling.

This is a real, if narrow, behavior change for a task-bound run, and AC-005.5 names it as
one of its two exceptions rather than claiming nothing moved: today a padded `" T1 "`
scopes to `[" T1 "]` and the run can mutate *nothing*, because no task id has that
spelling. After trimming it reaches `T1`, the task it was always meant to be bound to.
That is a fix, not a widening — but it is not "byte-for-byte today", so it is written
down. Without that, a whitespace-only
`task_id` is non-empty for the branch test and empty for `WithTaskScope`, which is the
contradiction AC-003.1 and AC-003.3 would otherwise encode.

**`WithTaskScope` filters.** It drops empty and whitespace-only entries before storing
them, so `WithTaskScope("")` yields an empty `AllowedTaskIDs` rather than `[""]`. This
alone converts the current silent lie into an honest empty scope; `CanMutateTask` already
returns false for both, so no task-bound behavior moves.

**Taskless runs materialize the runner set.** When the trimmed id is empty, the builder
resolves the runner set through a new reader and the resulting ids become
`AllowedTaskIDs`, ordered `updated_at DESC, id DESC` and capped at 500.

### The runner-set predicate

`CountActionableTasksForAgent` supplies four of the five clauses — runner projection
equals the agent, `state IN ('TODO','IN_PROGRESS')`, `archived_at IS NULL`, and not
automation-origin — but it takes only an agent id and has **no workspace clause**. The
new reader adds `workspace_id = ?` bound to `runCtx.WorkspaceID` (AC-003.13). The
capability's scope is a write authority, so it is filtered explicitly rather than left to
rely on agent profiles happening to be workspace-unique.

An empty `runCtx.WorkspaceID` is not a filter value. Binding it would match only tasks
that themselves carry an empty workspace and then freeze that as a **final** `runner_set`
scope, so the query is not issued at all and the scope takes the provisional
`unavailable` path instead (AC-003.15) — the same answer a failed query gets, for the
same reason: authority that could not be verified is not granted.

That filter binds **this branch only**. A task-bound run's scope is its own trimmed
`task_id` and is not workspace-checked, exactly as today — the id came from the run's own
payload, not from a board query, and adding a check there would be new behavior that
AC-003.3 disclaims.

### ContextBuilder dependencies

`ContextBuilder` today holds `Agents shared.AgentReader` and `Runs RunSnapshotStore`. It
gains **two** narrow local interfaces, both satisfied at the composition root and both
nil-safe:

- `RunnerTaskLister` — returns the ordered, capped runner-set ids plus the true total, so
  truncation is detectable without a second count query. A nil lister is **not** an empty
  runner set: it is a scope that could not be derived, so it takes the provisional
  `unavailable` path (AC-003.15) and is retried on the next build, rather than freezing a
  final `runner_set` empty scope no later build would revisit.
- `ScopeEventAppender` — appends a run event. A nil appender skips the event and never
  fails the build.

Derivation returns an internal `scopeDerivation` value — the ids, the true total, whether
it was truncated, the query error if any, and the resulting marker. It fills
`AllowedTaskIDs` and is not exposed on `RunContext`, so an unexported `build` returns
`(RunContext, scopeDerivation, error)`: exported `Build` drops the derivation, and
`BuildAndPersist` keeps it to choose the event and its payload after the swap.

**Event timing follows the winning write, not the derivation.** `BuildAndPersist` emits
`runtime.scope_truncated` / `runtime.scope_unavailable` only after its compare-and-swap
succeeds, so the events on a run describe the scope actually in force rather than every
scope that was speculatively computed for it. `BuildAndPersist` owns the
`ScopeEventAppender` call site outright: `Build` derives the scope and never appends, so a
`ContextBuilder` with no `Runs` store persists nothing and therefore emits nothing. This is
the mechanism behind AC-003.14, which scopes AC-003.5's and AC-003.6's event obligation to
the build that wins and to a configured appender: a losing build, a build with no appender,
and a build that persists nothing each append none and still return a context.

Failure is closed: a query error yields an empty scope and a context that still builds. This is deliberately opposite to `checkIdleSkip`,
which fails open: its open failure means "launch the agent anyway", costing a wasted run, whereas an open failure here would mean "grant authority you could not
verify."

### Run events

Both are new `RunEventType` values. That type is an explicitly open set ("Adapters may
emit additional values"), so no enum gate blocks them, and the run-detail Events tab
renders them through its existing generic renderer.

| Event | Level | Payload keys |
|---|---|---|
| `runtime.scope_truncated` | `warn` | `cap` (500), `total` (true runner-set size), `agent_id`, `run_id` |
| `runtime.scope_unavailable` | `warn` | `error` (query error, or AC-003.15's unmet condition), `agent_id`, `run_id` |

A clipped coordinator is therefore visible in its own run rather than merely
under-powered, and a fail-closed empty scope is distinguishable from an honestly empty
one.

### Snapshot semantics

`BuildAndPersist` is called from `prepareAndLaunch`
(`internal/office/service/scheduler_integration.go`) on **every launch attempt**, and it
always re-derives `Capabilities` from the live agent and payload before overwriting
`runs.capabilities` and `runs.input_snapshot`. There is no read-back path. `runtime-01.md`
records that "a re-issued JWT after restart uses the same capability set", and that holds
today only because the capability booleans are a pure function of the agent: re-deriving
them is idempotent.

A runner set is not a pure function of the agent. It is a projection of a mutable board,
so re-deriving it on a relaunch or after `RecoverStale` produces a *different* answer, and
AC-003.9 ("taken once per run") and AC-003.11 ("a re-minted token reproduces the persisted
snapshot") would both be false. The precedent does not transfer, so the reuse is made
explicit rather than inherited.

The serialized capability snapshot gains a `task_scope_source` marker with three values:

| Marker | Finality | Meaning |
|---|---|---|
| `payload` | final | scope is the run's own trimmed `task_id` |
| `runner_set` | final | scope was materialized from the runner set |
| `unavailable` | **provisional** | the runner-set query failed; scope is empty, fail-closed |

`Build` reads the run's persisted capabilities first. If `task_scope_source` names a
**final** marker, the persisted `AllowedTaskIDs` is reused verbatim and no runner-set query
is issued. Otherwise — no marker, the provisional `unavailable`, a value outside the three
above, or a `runs.capabilities` string that does not parse — the scope is derived afresh
and written with its marker (AC-003.12(b)). The marker's *finality*, not the scope's
emptiness, distinguishes "derived, and legitimately empty" from "not usably derived yet".

Three consequences worth stating, because each was a silent gap:

- **A transient failure is retried, not frozen.** `unavailable` being provisional means the
  next `BuildAndPersist` for that run tries the runner set again. Treating it as final
  would let one database blip de-scope a coordinator for the whole life of the run, with
  no event distinguishing "will retry" from "never again".
- **An unparseable or unrecognized snapshot derives rather than fails.** `Build` reads
  `run.Capabilities` for the first time under this design, so this parse path is new.
  Deriving is the safe answer: it is the normal path, it cannot widen authority beyond the
  runner set, and it avoids bricking a run on a value we cannot interpret.
- **Reuse covers the task scope only.** Capability booleans keep re-deriving from the agent
  on every build (AC-003.12(d)).

#### First write wins, and how

AC-003.12(c) is a concurrency guarantee, so it needs a mechanism rather than an intention.
`UpdateRunRuntimeSnapshot` is an unconditional
`UPDATE runs SET capabilities=?, input_snapshot=?, session_id=? WHERE id=?` — no predicate
at all — so two processors that both read "no final marker" would both derive and the
**last** would win, which is the opposite of the requirement.

The runs repository therefore gains a compare-and-swap sibling:

```
UpdateRunRuntimeSnapshotCAS(ctx, id, prevCapabilities, capabilities, inputSnapshot, sessionID) (bool, error)

UPDATE runs SET capabilities = ?, input_snapshot = ?, session_id = ?
WHERE id = ? AND COALESCE(capabilities, '') = ?
```

`prevCapabilities` is the exact string `Build` read when it checked the marker. The bool is
`RowsAffected() > 0`. On `false` another processor wrote first: re-read the run, and if its
marker is now final, adopt that scope and emit no scope event. If it is still not final,
retry the derive-and-swap — bounded at three attempts, after which `BuildAndPersist`
returns an error and the launch attempt fails, which the next attempt retries from scratch.

**The swap guards the task scope, not the whole row.** A loser still has a `session_id` and
an `input_snapshot` to persist, and today `BuildAndPersist` always writes them; a CAS that
simply skipped the write on a loss would silently drop that. So the loser, having adopted
the winning scope, performs the ordinary unconditional
`UpdateRunRuntimeSnapshot` with it. That write is safe precisely because the scope it
carries is the one that already won — the two processors now agree on it, so the write is
idempotent in the field the guarantee is about, and `session_id` keeps its existing
last-write-wins behavior, unchanged by this design.

Comparing the previous **value** rather than extracting the marker in SQL is deliberate:
`json_extract` is SQLite-flavoured, Postgres is a supported driver
(`internal/persistence/provider.go`), and `CancelRunsForTasks` already records that
dialect-specific SQL is kept out of the shared runs writer. A value compare is
dialect-neutral and needs no `dialect.JSONExtract`.

No lock and no transaction are added: a single conditional `UPDATE` is atomic, and the
losing side reconciles by reading.

The consequence is stated rather than hidden: a task assigned to the agent while a run is
in flight is not writable by that run. The next run picks it up. For a routine firing
every five minutes, worst-case latency is one cadence.

## Three scope classes

`CanMutateTask` stays pure and keeps its current meaning — it is now the
**state-mutation** predicate, consulted by `UpdateTaskStatus`, `CreateSubtask`, and
`CreateTask` with a parent. Unchanged for every run, taskless or not.

`PostComment` stops calling it and calls a new **annotation** predicate, which branches on
tasklessness using the trimmed-id test defined above:

- **Task-bound run** — the target must equal the run's own bound task. Anything else is
  `ErrTaskOutOfScope` (`403`), which is byte-for-byte today's behavior, since
  `CanMutateTask` short-circuits on `taskID == c.TaskID` and a task-bound run's scope is
  that single id (AC-004.12). It never reads `runCtx.WorkspaceID`, so the empty-claim guard
  below does not reach it and AC-005.5's "unchanged" holds for annotation too.
- **Taskless run** — an empty or whitespace-only `runCtx.WorkspaceID` is
  `ErrWorkspaceOutOfScope` (`403`), decided before the target is resolved (AC-004.13);
  without that guard an empty claim would compare equal to an unowned task's empty
  `workspace_id` and record the comment. The guard belongs to this branch alone because
  only this branch compares workspaces at all. Then resolve the target task's workspace and
  compare it to `runCtx.WorkspaceID`. Equal, record the comment (AC-004.1, AC-005.6).
  Anything else — a different workspace, or no such task — is `ErrTaskOutOfScope` (`403`), the *same*
  sentinel and therefore the same response body, so annotation cannot be used to probe
  whether a task id exists elsewhere. No target and no bound task, `ErrTaskOutOfScope`.

  The sentinels are aligned deliberately. `respondRuntimeError` writes
  `{"error": err.Error()}`, so pairing a cross-workspace `ErrWorkspaceOutOfScope` with an
  unresolvable `ErrTaskOutOfScope` would produce the bodies `forbidden: workspace out of
  scope` and `forbidden: task out of scope`, letting an unattended agent separate "exists
  elsewhere" from "does not exist" by reading them. One sentinel on both paths is what
  makes the anti-oracle claim true. Only the annotation path is aligned;
  `validateTaskRelations` keeps `ErrWorkspaceOutOfScope` for parented creation, unchanged.

  A *failed lookup* is not a refusal. `GetTaskWorkspaceID` already distinguishes them —
  `("", nil)` for no such row, `("", err)` for a query failure — so a failure is returned
  as-is and surfaces as `500` (AC-004.3). Collapsing it into the `403` would disguise a
  database outage as an authorization decision.
- **Either** — an empty or whitespace-only body is `ErrCommentBodyRequired` (`400`),
  checked before the branch so a malformed request is not also an existence oracle. The
  full order is capability, body, branch, with the taskless branch adding its workspace
  claim before the target.

This is the only behavioral widening in the design, and it is bounded by the workspace on
every path and by tasklessness at the branch.

The predicate needs no new dependency. `Actions.deps.Tasks` is a `TaskCreator`, and that
interface already declares `GetTaskWorkspaceID(ctx, taskID) (string, error)` alongside
`GetTaskProjectID`; `validateTaskRelations` already calls it for exactly this comparison,
a target task's workspace against the run's. The annotation predicate calls the same
method on the same dependency.

No `TaskWorkspaceReader` interface is introduced. A second reader behind one question,
wired at two composition sites that can drift, is the trap `office/AGENTS.md` records under
"Participant slate: one builder".

The **board-read** class is workspace-only and consults neither predicate.

## What stays denied

No `move_task` or `archive_task` key is added. `tasksMove` and `tasksArchive` keep
failing locally before any HTTP request, preserving `agents-03.md`.

No `record_step_decision` key is added. The MCP tool
(`internal/mcp/server/agent_decision_tool.go`) resolves task, step and role from
`s.taskID` / `s.sessionID` and returns an error when either is empty. Adding a runtime key
would put a second identity model on the same decision ledger, which is the failure mode
`office/AGENTS.md` records for participant slates. A taskless run gains decision recording
when it gains a session, and not before.

## Concurrency and revocation

Concurrent annotations do not conflict: comments are additive. `ListComments` orders by
`created_at` with **no** secondary key, so two comments written within one clock tick have
an unspecified relative order. That is recorded rather than fixed — the read path is
shared with the dashboard, and changing it is outside this capability.

A board read paged over a mutable sort column can see a row twice or miss it if that row
is written between pages. The contract guarantees a total order within one page, not a
stable order across a mutating board.

Board-read events are not deduplicated (AC-001.11): each accepted read appends one
`runtime.action` and each refusal one `runtime.denied`, so a retry loop is visible as
repeated rows rather than being silently collapsed.

Two runs of one agent can hold overlapping task scope only after `RecoverStale` re-queues
a claimed run whose process is still live with an unexpired JWT (default 4h);
`ClaimNextEligibleRun`'s per-agent busy lock (`agent_profile_id`, claimed count zero)
prevents it otherwise. The compare-and-swap in
[Snapshot semantics](#snapshot-semantics) makes that overlap harmless: the first final
marker wins, the loser reconciles by reading, and only the winner emits a scope event. Task-status
mutation is untouched and keeps last-write-wins and its existing approver gate, with no
revision token or precondition. Revoking an agent's permissions mid-run does
not affect the in-flight JWT's snapshot, unchanged from `runtime-01.md`.

## Failure modes

- **Runner-set read fails.** Empty scope, `runtime.scope_unavailable`, provisional marker
  `unavailable`, context built. Every task-scoped mutation then refuses with
  `ErrTaskOutOfScope`; board read and workspace-scoped actions are unaffected. The next
  `BuildAndPersist` for that run derives again, because the marker is not final.
- **Persisted snapshot unparseable, or its marker unrecognized.** Treated as "not usably
  derived": scope derived afresh and rewritten. No run is bricked by a value we cannot
  read.
- **Runner set underivable.** No lister wired, or an empty run workspace. No query is
  issued; empty scope, provisional `unavailable` marker, retried on the next build.
- **Empty workspace claim on taskless annotation.** `ErrWorkspaceOutOfScope` (`403`), no
  comment, decided before the target is resolved. A task-bound run reads no claim and is
  unaffected.
- **Annotation workspace lookup fails.** `500`, no comment. Distinct from the `403` a
  missing or foreign task gets, so an outage is not read as a refusal.
- **Board-read parameters invalid.** `ErrInvalidListParams` (`400`), no query issued, no
  rows, no cursor.
- **Board-read query fails.** `500`, no partial page, no cursor. The run continues; the
  agent sees an error from the CLI.
- **No such task during annotation.** `ErrTaskOutOfScope` (`403`); no comment written;
  indistinguishable from a cross-workspace target by design, same sentinel.
- **Cap exceeded.** First 500 by `(updated_at DESC, id DESC)`, `runtime.scope_truncated`
  recorded with the true total.
- **Overlapping runs for one agent.** The compare-and-swap decides: first final marker
  wins, the loser re-reads and adopts it, and emits no scope event.

## Durability

- **`runs.capabilities` / `runs.input_snapshot`**: durable, now carrying a materialized
  task-scope list plus `task_scope_source` for taskless runs, written together by one
  conditional `UPDATE` so they cannot diverge. A JWT re-minted after restart reproduces the
  same authority because a *final* scope is read back rather than re-derived; a provisional
  `unavailable` is retried instead.
- **`office_run_events`**: durable. `runtime.action`, `runtime.denied`,
  `runtime.scope_truncated`, `runtime.scope_unavailable`.
- **Runner set itself**: not durable. It is a projection of `tasks`, materialized once per
  run and thereafter read from the snapshot.
- **No migration.** Runs persisted before this capability carry no marker and derive fresh
  on their next context build; nothing backfills `runs`.

## Testing

- Vocabulary: `Allows` returns the granted state for `list_tasks` and still returns false
  for an unknown key; `AllowedKeys` includes it in stable order; `FromAgent` grants it for
  every role.
- Scope: `WithTaskScope("")` and `WithTaskScope("  ")` both yield an empty list; a taskless
  run yields the runner set; a task-bound run yields exactly its trimmed `task_id` and a
  `RunContext.TaskID` trimmed to match; the runner-set workspace filter does not apply to
  the task-bound branch; a whitespace-only payload id is treated as taskless; a query
  error, a nil lister and an empty run workspace each yield an empty list, an
  `unavailable` marker and no build error, the last two issuing no query; over-cap truncates deterministically and
  records the true total; the scope excludes tasks in other workspaces.
- Snapshot: a second `BuildAndPersist` for a run with a **final** marker reuses the
  persisted scope and issues no runner-set query; a run with no marker derives and persists
  one; a run with a provisional `unavailable` marker derives again; an unparseable
  snapshot and an unrecognized marker both derive rather than fail; capability booleans
  still re-derive. Concurrency: a CAS whose `prevCapabilities` no longer matches reports
  `false`, does not overwrite, and the loser adopts the persisted final scope and emits no
  `runtime.scope_truncated` / `runtime.scope_unavailable` row; a build with no appender, and
  one with no `Runs` store, each emit nothing and still return a context, so a scope-event
  test must wire a `Runs` store to observe one.
- Enforcement: status update, subtask and parented create refuse outside the runner set; a
  taskless run comments inside its workspace and refuses outside it; a taskless run with an
  empty workspace claim refuses even a task whose own `workspace_id` is empty, while a
  task-bound run with an empty claim still reaches its own task; a task-bound run
  refuses any target but its own task; an empty body is `400`; root create succeeds on a
  taskless run. A cross-workspace target and a nonexistent one return the **same** sentinel
  and the same body, while a failing workspace lookup returns `500`. A task-bound run whose
  payload id is padded with whitespace reaches its own task.
- Board read: capability denial is `403` with `runtime.denied` and precedes parameter
  validation; empty workspace claim refuses; the filtered path is taken with no query
  parameters; an out-of-allow-list sort is `400`; a default read comes back descending; a
  repeated `sort` is `400` while a repeated `status` unions; a cursor without `cursor_id`
  is `400`; a two-page walk over rows sharing an `updated_at` returns each row exactly
  once; two identical reads append two `runtime.action` rows. Normalization: `order=DESC`,
  `include_system=1` and `limit=abc` are each `400`, while `limit=` and `limit=0` both take
  the 100-row default, and `?cursor=&cursor_id=x` is `400` rather than a first page.
- CLI: `tasks list` succeeds with `KANDEV_WORKSPACE_ID` unset, and a repeated `--status`
  reaches the endpoint as two `status` values rather than one.
- Regression: a task-bound run's capabilities, scope and refusals are byte-identical to
  today, except that `list_tasks` is now granted.

## Out of scope

- **The taskless session seam.** `task_sessions.task_id` is `NOT NULL` and `launchAgent`
  refuses a run with no `task_id` (`failTasklessRun`). This says what such a run may do
  once it can run, not how it runs.
- **Recording a step decision from a taskless run.** Reasons under REQ-005.
- **Workflow-step moves and archival for agents.** Denied per `agents-03.md`.
- **Narrowing `spawn_agent_run`.** Its unscoped workspace-wide reach is a real containment
  gap, and is why REQ-004 rejects it as a precedent, but it is a separate contract change.
- **A tiebreak on comment ordering.** `ListComments` orders by `created_at` with no
  secondary key, so two comments in one clock tick have an unspecified order. That read
  path is shared with the dashboard.
- **Preconditions or optimistic concurrency on task-status mutation.** Last-write-wins and
  the existing approver gate are kept.
- **Backfilling scope markers onto pre-existing runs.** They derive fresh per AC-003.12(b).
- **Per-task ACLs, or role-varying authority.** `FromAgent` remains the single derivation.
- **Live re-resolution of task scope during a run.** Rejected for the snapshot semantics
  `runtime-01.md` already froze.
- **Changing `checkIdleSkip`.** A board-reading coordinator may deserve a different
  idle-skip predicate, but that is a scheduler contract change.
- **Frontend surfaces.** No UI. The run detail Events tab renders the four new
  `runtime.*` event rows through its existing generic renderer.
