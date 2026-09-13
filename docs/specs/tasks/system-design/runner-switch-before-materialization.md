---
status: draft
system: tasks
requirements:
  - REQ-TASKS-RUNNER-SWITCH-001
  - REQ-TASKS-RUNNER-SWITCH-002
  - REQ-TASKS-RUNNER-SWITCH-003
  - REQ-TASKS-RUNNER-SWITCH-004
---

# Runner Switch Before Materialization System Design

## Purpose and boundaries

The task system owns task metadata, the point at which a launch resolves an
executor, and the durable rows that record what a task has materialized. All of
the state this capability reads and the single value it writes are task-owned,
so the task system owns this contract.

Two adjacent contracts are consumed, not owned:

- The executor system owns executor profiles, executors, and the per-executor
  capability that says whether a runtime needs a git clone URL. The compatibility
  gate calls that capability; it does not enumerate executor types.
- The workspace system owns repositories, worktrees, and workspace groups. The
  mutability gate reads task-scoped rows only and does not reach into workspace
  ownership rules.
- The office system owns shared workspace groups and their membership rows. This
  is a boundary the capability genuinely crosses rather than merely consumes: the
  membership insert is one of the writers AC-TASKS-RUNNER-SWITCH-002.3b requires
  to take the task-scoped lock, so satisfying the concurrency contract means
  changing office-owned code, not only task-owned code. Naming it here is what
  keeps that from being discovered during implementation.

## Prior art

**Wiki leg (our own prior reasoning).** Receipt: searched for a `wiki-query`
skill in `~/.claude/skills/`, `.claude/skills/`, and the repository's
`.agents/skills/`; searched `~/.claude` and `~/.claude-kandev` for any path
matching `*wiki*`; checked for `~/.obsidian-wiki/config`; checked `PATH` for
`wiki-query`, `wiki-switch`, and `qmd`. None of these exist in this environment,
so no `OBSIDIAN_VAULT_PATH` could be resolved and no QMD collection was queried.
This leg is unavailable, not empty. In its place the repository's own durable
record was searched: `docs/specs/tasks/` and `docs/specs/executors/` indexes,
and every requirement naming an executor. That search produced one directly
adjacent position, described below.

**In-repo prior reasoning.**
[Task Create Executor Default](../requirements/task-create-executor-default.md)
already settled how a task's *initial* runner is chosen, and settled it in a
direction this design deliberately extends rather than contradicts: a
repository-backed task must start in an isolated workspace unless an explicit
contrary choice is made, and a portable last-used preference must never silently
switch the executor. That decision is why every card starts on the worktree
runner, which is the condition this capability exists to relieve. It is also why
this design does not change any default: the existing requirement's position is
that defaults are safety, and an explicit per-task move is the correct escape
hatch rather than a looser default.
[Kanban task cache preserves executor fields across merges](../requirements/kanban-task-executor-cache-staleness.md)
is the second position consulted. It establishes that derived, omit-when-empty
executor fields get blanked out by client cache merges, and that the fix is to
gap-fill from cache. AC-TASKS-RUNNER-SWITCH-001.9 deliberately departs from that
position for the mutability verdict: gap-fill is right for an identity label and
wrong for a permission-shaped flag, so this field is always emitted and fails
closed on absence instead.

**External products leg.** Receipt: the `saas-kb` MCP server and its
`search_fsm_docs` tool are not present in this session's tool set, and no MCP
configuration in this environment registers them. This leg is unavailable, and
no claim about what other products shipped is made here.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-RUNNER-SWITCH-001` | [Components and responsibilities](#components-and-responsibilities), [Data and contracts](#data-and-contracts) |
| `REQ-TASKS-RUNNER-SWITCH-002` | [Control flow](#control-flow), [Persistence](#persistence), [Failure and recovery](#failure-and-recovery) |
| `REQ-TASKS-RUNNER-SWITCH-003` | [Control flow](#control-flow) |
| `REQ-TASKS-RUNNER-SWITCH-004` | [Client surface](#client-surface) |

Every anchor above resolves to a heading in this document.

## Components and responsibilities

- **Mutability evaluator (task service).** Single implementation of the ordered
  condition list. Every caller, the projection and the action alike, uses it, so
  a projected verdict and an enforced verdict cannot disagree.
- **Runner action handler (task service).** Authorizes the caller, opens the
  serialized transaction, re-runs the evaluator, applies the compatibility gate,
  writes the single metadata key, and publishes the update event.
- **Task projection (task DTO).** Emits the verdict alongside the executor
  profile identifier it already derives from task metadata.
- **Task dialog (web).** Renders the existing executor profile selector under
  the projected verdict and calls the action on save.

## Data and contracts

**Projected fields.** `runner_editable` (boolean) and
`runner_ineligible_reason` (string) are added to the task projection. Neither is
omit-when-empty; both are always serialized. The reason vocabulary is closed:
`eligible`, `evaluation_unavailable`, and the ten condition codes named in
AC-TASKS-RUNNER-SWITCH-001.3.

**Action.** `task.runner`, payload `{ id, executor_profile_id }`. The two-segment
name is load-bearing: the gateway's top-level task-action rule grants the
task-authorization backstop to the task namespace plus exactly one segment and
promotes `id` to the task identifier. A deeper name silently loses both.

**Conflict vocabulary.** The action returns the ten mutability codes plus
`target_cannot_materialize_repository`. Invalid-request and not-found outcomes
are distinct from all of them, so a client can tell "you may not" from "that is
not a thing". A fourth outcome class, `evaluation_unavailable`, is distinct from
all of the above and is the only retriable one: it says the system could not
decide, not that the answer is no. The full outcome precedence, which stage wins
when several apply, is fixed by AC-TASKS-RUNNER-SWITCH-002.17.

**State read by the evaluator.** Task archive state, the count of the task's
repository attachments, the existence of any session, task environment, running-
executor record, workspace folder attachment, or unreleased workspace group
membership, the task's declared host workspace path, and the task's declared
workspace mode paired with whether it has a parent.

Two of those reads are non-obvious and are the reason the rule is a service
concern rather than a column lookup. A task environment is keyed to the task,
not to a session, so removing every session leaves the environment behind; and a
running-executor record has no foreign key to the session either, so session
deletion does not remove it. Both must be queried directly.

## Control flow

1. A client reads a task. The projection calls the evaluator and emits the
   verdict with the rest of the task.
2. The user changes the executor profile in the task dialog and saves. The dialog
   issues `task.runner` only when the **user** changed the selection during this
   editing session — not merely when the selection differs from the stored value
   (AC-TASKS-RUNNER-SWITCH-004.5). The two tests are not equivalent, and the case
   that separates them is the common one: the dialog seeds an empty selector on
   open, so a task that stores no executor profile always shows a selection that
   differs from the stored value. Issuing on inequality would make an untouched
   save store an explicit runner derived from a remembered preference, which
   AC-TASKS-RUNNER-SWITCH-002.16 then makes durable — the silent carry-over the
   prior art cited above exists to prevent. The dialog therefore tracks whether
   the user touched the control, and a seeded value is displayed
   (AC-TASKS-RUNNER-SWITCH-004.5a) without being treated as a choice.
3. The handler authorizes the caller against the task. WebSocket callers are
   additionally covered by the gateway backstop; that backstop is not relied on
   for any other caller.
4. **Before opening any transaction**, the compatibility gate runs. It resolves
   the target profile's executor, rejecting as invalid when the profile is
   missing or its executor is soft-deleted or not active; asks the executor
   capability whether that runtime requires a clone URL; and if it does, resolves
   whether the task's single repository can produce one. Only the *ordering* of
   candidates is taken from the launch path: a recorded remote URL, then a
   provider-derived URL, then the repository's local checkout origin. The launch
   path's own helper cannot be reused as-is, because it collapses "no origin is
   configured" and "the origin lookup failed" into the same empty answer and puts
   no timeout on the subprocess it shells out to. This gate needs those two cases
   to produce different outcomes and needs the call bounded, so the resolution
   used here preserves the error and timeout signal separately from empty rather
   than inheriting that helper's return shape. Absent,
   empty and whitespace-only candidates fall through to the next; an exhausted
   list yields `target_cannot_materialize_repository`, while a lookup that errors
   or exceeds its timeout yields `evaluation_unavailable`. Because this runs
   before the transaction that validates how many repositories the task has, the
   step records *which* repository it resolved against, and skips entirely when
   the task does not have exactly one at that moment — a skip produces no verdict
   and no outcome, leaving `no_repository` or `multiple_repositories` to be
   reported by the mutability gate (AC-TASKS-RUNNER-SWITCH-002.7c). This step
   **resolves** the verdict but does not report it: the result is carried into the
   transaction and applied at its ordered position in step 7, after the mutability
   re-check, so a task that fails both reports the mutability code
   (AC-TASKS-RUNNER-SWITCH-002.17). Resolution runs outside the lock precisely
   because its last step inspects a local checkout with a subprocess rather than
   reading a column: holding the task row lock across that call would stall every
   session-creation path for the task for as long as the subprocess ran. The same
   property is why the gate is in the action and not in the per-task projection.
5. The handler opens a transaction that takes **the raw task row lock, not the
   task cleanup barrier**. Two primitives exist and they are not
   interchangeable: the barrier takes the row lock *and* refuses the caller when
   a resource-cleanup job for that task is prepared, pending, running, or waiting
   to retry, while the raw lock takes the row lock only. The switch takes the raw
   lock. Using the barrier would introduce a cleanup-in-progress rejection that
   is outside the closed conflict vocabulary and has no stage in the outcome
   precedence, so a builder would have to invent an outcome for it. Ordering
   against cleanup is what the contract asks for, and the raw lock supplies it;
   being *rejected* by cleanup is not required and is not wanted. The gate
   already answers correctly in both cleanup phases without it: while the rows
   still exist the session, environment, or running-executor condition refuses
   the switch, and once cleanup has removed them the task genuinely holds nothing
   and a switch is legitimate. Taking that lock serializes the transaction
   against every writer that can make a gate condition start holding
   (AC-TASKS-RUNNER-SWITCH-002.3a) — every session-creation path, including the
   Office one that takes the row lock independently rather than through the
   barrier, plus task environment creation, running-executor record creation,
   repository attachment, workspace folder attachment, group membership, and the
   workspace path and mode writes. The locked section performs database reads and
   one write, and nothing else.
6. Inside the transaction the evaluator runs again. A failing condition aborts
   with the typed conflict for the first failing condition in the fixed order. A
   read that fails aborts with `evaluation_unavailable`.
7. The compatibility verdict resolved in step 4 is applied — but first the
   handler confirms, still inside the transaction, that the task's single
   attached repository is the one step 4 resolved against. That is a column read,
   not a subprocess, so it does not reintroduce the cost step 4 exists to avoid.
   If the repository changed in between, the resolved verdict describes something
   the task no longer has, so it is discarded and the request rejects as the
   retriable `evaluation_unavailable` rather than being applied
   (AC-TASKS-RUNNER-SWITCH-002.7c). If it rejects on its own terms, the handler
   aborts with that code. Otherwise it compares the target against the
   stored value: equal means a no-op that commits nothing and reports success,
   having passed both gates; different means it writes only the executor profile
   key and commits.
8. On a committed change the handler publishes the task-updated event carrying
   the refreshed projection and the task's updated-at value from the committing
   transaction. Clients discard an event older than the value they hold, so an
   out-of-order delivery cannot resurrect a superseded runner
   (AC-TASKS-RUNNER-SWITCH-002.19).
9. The next session preparation for the task reads the stored executor profile
   and binds it, and the executor it belongs to, onto the created session. No
   other code path changes.

## Failure and recovery

- Every rejection happens before or inside the transaction and commits nothing,
  so a rejected switch is indistinguishable from one never attempted.
- The evaluator fails closed. A read error yields `evaluation_unavailable` in
  the projection and aborts the action with the `evaluation_unavailable` outcome;
  it never yields an eligible verdict and never reports a mutability conflict,
  which would tell the user their task is ineligible when the system merely could
  not tell. That outcome is retriable and is the only one that is.
- A projection path that never runs the evaluation at all, rather than running it
  and failing, emits the same fail-closed pair. The verdict fields are never
  emitted as language defaults, because the empty string is outside the closed
  vocabulary and a `false` with no reason is indistinguishable from a real
  refusal (AC-TASKS-RUNNER-SWITCH-001.1a).
- The action is a value assignment, so a client that cannot tell whether an
  attempt committed may simply repeat it. No idempotency token is defined,
  because none is needed and an unused one would be another thing to keep
  correct.
- The known race is a user opening a task, which prepares a session for a task
  that has none, while a switch is in flight. The row lock forces a total order:
  either the switch commits and the session takes the new runner, or the session
  commits and the switch is refused with `session_exists`. The dialog surfaces
  that refusal; the session-preparation path is not modified.
- The sibling races all have that same shape and that same resolution, and the
  rule is stated once for all of them rather than per row
  (AC-TASKS-RUNNER-SWITCH-002.3a): every writer that can make a gate condition
  start holding takes the same task-scoped lock. Task environment creation and
  running-executor record creation are the least obvious members, because a task
  environment survives the deletion of every session and a running-executor record
  has no foreign key to a session either, so each can appear on a task the session
  checks would call clean — but repository attachment, workspace folder
  attachment, group membership, and the workspace path and mode writes are in the
  same set for the same reason. Serializing only some of them is what would leave
  the rest observed but not enforceable. The rule also reaches a writer that
  changes no row count at all — an in-place update of an existing task-repository
  link — because it can change which repository the compatibility gate resolved
  against, which is why AC-TASKS-RUNNER-SWITCH-002.3a states that second clause
  alongside the first.
- Publishing the task-updated event is outside the transaction and best-effort. A
  failed publication does not roll back or retry the committed switch and is not
  reported as a failed switch, because the switch did commit
  (AC-TASKS-RUNNER-SWITCH-003.3a). The recovery is not a replayed event: a repeat
  of the request is a no-op that publishes nothing, so what closes the gap is that
  any later read of the task carries the committed runner and the recomputed
  verdict. That is why no outbox or delivery receipt is specified — the read path
  is already the convergence path.

### The writer audit, performed

AC-TASKS-RUNNER-SWITCH-002.3b requires this set to be enumerated rather than left
to be discovered, so it is recorded here. Two classes, and the split is what makes
the obligation a bounded piece of work rather than an open-ended one.

**Class 1 — already serialized, no change required.** The switch's transaction
locks the task's own row, so any writer that updates that same row is ordered
against it for free. That covers archiving the task, writing the task's host
workspace path, and writing the task's workspace mode: all three live in the task
row, and the metadata writes reach them with a targeted update of that row rather
than a separate table. Session creation and task environment creation are also in
this class, but for the other reason — they take the task-scoped lock explicitly,
one through the cleanup barrier and one directly. Both barrier and raw lock take
the same row, so a writer using either is ordered against a switch using the other.

**Class 2 — requires the lock to be added.** These write a different table and take
no task-scoped lock today:

| Writer | Gate condition | Why it is unserialized today |
| --- | --- | --- |
| Running-executor record write | 6 `executor_running` | Executes directly on the shared handle, in no transaction. |
| Workspace source attachment | 2, 3 `no_repository` / `multiple_repositories`, 7 `workspace_folder_attached` | Opens a transaction, but its only task-row lock is inside a parent-relation guard that returns immediately when no expected parent is supplied — so a task with no parent, the shape this capability most commonly serves, is not locked at all. |
| Workspace group membership insert | 9 `workspace_group_member` | Executes directly on the shared handle, in no transaction. Office-owned. |
| In-place task-repository link update | none — changes the resolved repository | Executes directly on the shared handle. The attachment count does not move, so the first clause of AC-TASKS-RUNNER-SWITCH-002.3a does not reach it; the second clause does. |

Four writers across two subsystems. That is the whole of the retrofit, and it is
stated here so the work can be sized before it starts rather than found during it.
The in-transaction repository confirmation of AC-TASKS-RUNNER-SWITCH-002.7c stays
even once the fourth writer takes the lock: it compares the link's last-updated
column as well as its identity, and exists so that a writer introduced outside this
audit is still caught rather than silently trusted.

## Client surface

The task dialog already renders the executor profile selector when editing. It is
never written to the task — but it is **not** unused, and the difference matters.
The edit save already forwards the selected profile to the session launch it
issues when an agent profile is chosen, so today the picker silently governs the
first session while leaving the task's stored runner untouched. Two consequences
follow, and both are contract, not implementation detail. First, the edit
interaction can materialize the task, which is why the launch is the last member
of the ordered save sequence and is suppressed by a rejected switch
(AC-TASKS-RUNNER-SWITCH-004.4d). Second, a launch that names a profile explicitly
must name the one the switch just committed, or the observable effect of a switch
would depend on which of the two paths ran (AC-TASKS-RUNNER-SWITCH-003.1).

This capability makes the picker effective on the task itself and puts it under
the server's verdict rather than the current task-state approximation, which gates
on workflow state and matches none of the ten conditions.

The selector's baseline is part of the contract for the same reason. The dialog
seeds an empty selector on open, so for a task that stores no executor profile the
displayed value is always something the dialog chose, never something the task
holds. It is presented as a resolved default rather than as a stored choice, and
displaying it is not selecting it: the switch is issued when the final selection
differs from the value the dialog seeded, not when it differs from the value the
task stores (AC-TASKS-RUNNER-SWITCH-004.5, AC-TASKS-RUNNER-SWITCH-004.5a,
AC-TASKS-RUNNER-SWITCH-004.5b). Comparing against the seed rather than against the
interaction history is what keeps the rule to one observable form, and it is why
pinning a resolved default is a named exclusion rather than an unstated gap.

The dialog reads `runner_editable` and `runner_ineligible_reason` from the task
it was opened with and never recomputes either. When the verdict is `false` the
selector is not offered for editing and the reason is shown; an absent or
unrecognized reason is presented as `evaluation_unavailable` rather than as raw
text.

Save ordering is load-bearing because the dialog's save is already several
non-transactional calls and this adds one more. The runner switch goes first, the
field updates run only if it succeeds, and the session launch, when there is one,
goes last. A rejection leaves every other edit unsaved and still in the open
dialog. That ordering is what keeps a refused switch from leaving half a save
behind, and it is why the rejection path reports that nothing was saved rather
than only that the runner did not move. The launch is last because it is the only
call that materializes the task: once it commits, the switch ahead of it can no
longer be retried on equal terms, since the task now has a session and the gate
would refuse it.

Ordering first is not the same as being atomic, so the rule covers the rest of
the sequence too (AC-TASKS-RUNNER-SWITCH-004.4c). A failure at any call stops the
sequence there, and the calls that already committed stay committed — there is no
compensating action for a runner switch, and inventing one would mean a second
switch that the gate could refuse. So the dialog reports which part of the save
applied instead of collapsing the whole thing to one success or one failure, and
a retry is safe because repeating the switch is a no-op.

Desktop and mobile share this gate, this copy and this interaction; the
capability introduces no new layout and no new mobile interaction pattern.

## Persistence

The write is a single-key update to the task's metadata document. It must not be
a read-modify-write of the whole map: the generic protected metadata update
replaces the document wholesale and preserves only the deferred launch record,
so routing this change through it would delete the task's agent profile and
leave the card unable to start. The repository already provides single-key
metadata writes that set one JSON path in the database rather than rewriting the
document, and a conditional variant of the same primitive that refuses the write
when the task has an active session. This capability uses that family of writes.

Note for implementation, of the same kind as the running-executor note below:
every member of that family currently executes on the shared database handle and
none of them accepts a transaction. The switch's write must happen *inside* the
serialized transaction, so a variant that takes the transaction handle is
required; the repository already has this shape elsewhere, where a metadata
removal is parameterized over its executor rather than bound to the shared
handle. Writing through the existing handle-bound form instead would put the
write outside the lock, where it would land even when the transaction rolls back
and would silently defeat the guarantees that a rejected switch persists nothing
and that a switch never commits alongside a session prepared under the old
runner.

No schema change is required. `runner_editable` and `runner_ineligible_reason`
are derived per read and are never stored.

Because the verdict is derived, a list projection must not fan out into a
per-task query for each of the eight row-existence checks.
AC-TASKS-RUNNER-SWITCH-001.5 is the observable constraint: the same task must
carry the same verdict whether it was read alone or in a list, which is what
makes a batched implementation verifiable rather than merely faster.

The evaluator's reads are several, and the executor profile it is projected
beside is a ninth. Left as independent point-in-time queries they can straddle a
concurrent switch and produce a payload whose profile and verdict describe
different states, which AC-TASKS-RUNNER-SWITCH-001.10 forbids. So the reads
backing one projected task — the stored executor profile and every row-existence
check behind its verdict — resolve against a single consistent snapshot, whether
that is one read transaction, one snapshot read, or one joined query. This is
about coherence within a payload, not authority: the projection stays advisory
and the action stays the thing that decides, per AC-TASKS-RUNNER-SWITCH-002.3.
An advisory verdict may be stale, and the client handles that
(AC-TASKS-RUNNER-SWITCH-004.4); it may not be internally inconsistent.

A read that fails degrades only the tasks whose own verdict depended on it
(AC-TASKS-RUNNER-SWITCH-001.8a), so a batched query covering a whole board
degrades the whole board and one covering a subset degrades that subset, under
one rule rather than two.

## Security

Authorization is per task and is the same authorization that governs the other
top-level task actions. The service authorizes every caller on every transport,
including WebSocket. The gateway backstop covers WebSocket dispatch and is an
extra layer there, not a substitute for the service guard and not a reason to
skip it — the service cannot reliably tell which transport a request arrived on,
and the other top-level task mutations in this system already authorize
unconditionally for exactly that reason. The action accepts exactly two fields and writes
exactly one derived value, so it is not a path for a caller to inject arbitrary
task metadata.

## Observability

A successful switch logs the task, the previous executor profile, and the new
one. A rejection logs the task and the reason code. The reason code is the same
closed vocabulary the client receives, so a support question about a refused
switch can be answered from logs without reproducing the state.

## Related decisions

- [ADR 0028: task-create last-used source of truth](../../../decisions/0028-task-create-last-used-source-of-truth.md)
- [ADR 0041: backend-owned portable user settings](../../../decisions/0041-backend-owned-portable-user-settings.md)
