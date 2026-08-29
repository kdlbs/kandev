---
status: draft
system: plugins
requirements:
  - REQ-PLUGINS-STEP-MOVE-001
  - REQ-PLUGINS-STEP-MOVE-002
  - REQ-PLUGINS-STEP-MOVE-003
  - REQ-PLUGINS-STEP-MOVE-004
  - REQ-PLUGINS-STEP-MOVE-005
created: 2026-08-26
owners:
  - nova28
---

# Plugin-Initiated Workflow Step Transitions System Design

## Purpose and boundaries

This design carries the technical detail behind
`docs/specs/plugins/requirements/plugin-initiated-step-transitions.md`: the
wire contract a third party compiles against, the enforcement seam, the error
mapping, the two trigger vocabularies, and the code-path facts that constrain
what the acceptance criteria can promise. The requirements file owns the behavior; this
file owns how it is expressed and why the alternatives were rejected.

Nothing here redefines the task system's move semantics. The move path is
reused unchanged; the design decisions are about the plugin-facing surface.

## Corrections to the originating report

**`UpdateTask` is not silent about the transition.** It sets `steptelemetry`
attribution and calls `recordManualStepTransition`, so the task-level ledger row
*is* written. What is missing is the `task.moved` event, WIP admission,
target-step validation, and the queue-vacate pull.

**`StepHistoryActor` is never persisted as a value.** It only decides whether an
authenticated user id is looked up, so the persisted actor of record is
`task_step_transitions.actor_kind` / `actor_id`.

## Prior art


**Our own wiki: unavailable, no substitute consulted.** The configured vault is
outside this sandbox's readable tree and `obsidian-wiki` / `qmd` are not
installed, so the semantic pass could not run; the one reachable vault has zero
Kandev pages. This leg returned nothing useful, and the reason is tooling access
rather than an empty wiki — a later pass on a machine with vault access may find
prior reasoning this spec did not see.

**What other products shipped (`saas-kb`, `category: ai_sdlc`).** One hit was on
point: Factory.ai's audit model splits attribution into **Actor** ("the user or
service principal who initiated the action") and **Source** ("the surface that
originated the event") (`Factory.ai/Guides/enterprise_audit-log.md`) — the same
shape as `actor_kind`/`actor_id` plus `trigger`, confirming a service principal
is a first-class actor rather than a stand-in for the human who configured it.
Everything else scored ~0.009 and is noise.

**What we are doing differently.** Factory.ai reports a `null` actor for
system-initiated events. We do not: that would lose *which* plugin moved the
card, the only question an operator will ask.

## Why the actor is `integration`, identified by `plugin:<id>`

The originating report offered a new `plugin` actor kind, or the authenticated
user driving the plugin surface. Neither is right.

**The authenticated user is not implementable and not honest.** `hostForPlugin`
(`internal/plugins/service.go:612`) binds a host to `pluginID` and capabilities
only. There is no `authn` identity on a plugin gRPC context — that seam exists
only in the HTTP handlers (`authn.FromGin`). A plugin's Go backend can act on a
webhook, a timer, or its own logic with no human anywhere, so "the user driving
the surface" frequently does not exist. The frontend Host API
(`apps/web/lib/plugins/host-api.ts`) contributes UI components and exposes no
task-write path, so it supplies no authenticated alternative.

**A new `plugin` actor kind invents a value we already have.**
`steptelemetry.ActorKind` includes `ActorIntegration`
(`internal/steptelemetry/steptelemetry.go:51`) with a production writer for
exactly this shape of caller: registered external automation with a stable id
(`watcher_dispatch.go:378` sets `ActorID: src.WatchID(evt)`, under
`ActorKind: ActorIntegration` on 377, for Jira and Linear creates). A plugin is that.

**`ActorID = "plugin:<id>"` reuses one provenance string.** The same value is
already produced by `pluginHost.pluginSource()` and already appears in
`metadata.source` on plugin-created tasks and plugin-sent messages, so the
ledger joins to those rows without a second vocabulary.

**`MoveTaskOptions.StepHistoryActor` must not be `Human`.** It is a control
flag, not a stored value: `recordManualStepTransition`
(`service_workflow.go:1199`) uses it only to decide whether to look up an
authenticated user id, and `CreateStepTransition` takes no actor-kind argument.
`Human` is what causes the operator's user id to be stamped on the row, and an
unattended automated move must not inherit it — the same reasoning the field's
own doc comment gives for agent moves. Because the value itself is never
persisted, `Agent`, `System` and the zero value are observationally identical
here; the requirement is only that it is not `Human`.

## Why `api_write:tasks` and not a new capability

A plugin holding `api_write:tasks` can already create tasks and start agents, so
a move-specific capability closes no door that is open today. It can also set
`workflow_step_id` through `UpdateTask` right now — that is the defect — so
gating the *fixed* path behind a new capability would make the correct path
harder to reach than the broken one, and every existing plugin would keep the
broken behavior until it re-declared.

The `PluginOwnedTaskTree` ownership rule that guards plugin-owned task deletion
is deliberately not applied here. Moving a task is not destructive, and
restricting moves to plugin-created tasks would defeat the purpose: a dispatch
plugin exists precisely to move work it did not create. Safety comes from the
auto-start gates the move runs through, not from ownership.

## The two trigger vocabularies

The "why" column is already caller-specific, but the two persisted surfaces
have **separate, non-overlapping** vocabularies. A plugin move writes to both,
so `plugin_move` must be added to both.

| | Task-level ledger | Session-level history |
|---|---|---|
| Table | `task_step_transitions` | `session_step_history` |
| Enum | `steptelemetry.Trigger` (`steptelemetry.go:28-42`) | `wfmodels.StepTransitionTrigger` (`workflow/models/models.go:310-325`) |
| Written by | `steptelemetry.FromContext`, in-transaction (`step_transitions.go:88`) | `recordManualStepTransition` → `CreateStepTransition` |
| Declares | `task_created`, `manual_move`, `task_update`, `mcp_move`, `mcp_deferred_move`, `engine_transition`, `user_cancellation`, `wip_pull`, `bulk_move`, `unarchive_restore`, `workflow_attached`, `workflow_detached`, `unknown` | `manual`, `auto_complete`, `approval`, `on_turn_start`, `on_children_completed`, `task_update`, `queue_promotion` |
| Notably lacks | `queue_promotion` | `mcp_move`, `wip_pull` |

**The session-level row is conditional, and usually absent.**
`recordManualStepTransition` returns early when the task has no session; its
comment gives the reason: `session_step_history.session_id` is a NOT NULL FK to
`task_sessions`, so a session-less move cannot be recorded without a schema
change. This feature's most common case — a plugin moving a card onto a step
whose `on_enter` starts an agent — is by definition a task with no session yet,
because the move is what creates it. So a plugin move writes the task-level
ledger row always and the session-level row only when a session already exists,
and `plugin_move` in `wfmodels.StepTransitionTrigger` is exercised only by the
latter. AC-003.5 states this; adding the value is still required, because the
moment a task has a session the fallback below applies.

Adding `plugin_move` to only the first is the failure mode this design exists to
prevent. `recordManualStepTransition` defaults an empty trigger to
`StepTransitionTriggerManual`, so a plugin move would persist
`session_step_history.trigger = "manual"` — the value reserved for human board
moves — under a requirement whose entire point is that a plugin move is not a
person's action.

`steptelemetry.ContractVersion`'s doc states it is bumped "only for a change to
what a row means, **not** for adding a new Trigger/ActorKind value", so
`plugin_move` costs no contract bump and no telemetry re-activation. Neither
`trigger` column carries a CHECK constraint, so neither addition needs a
migration.

## The wire contract

This is externally consumed: third-party plugin authors compile against it
through `pkg/pluginsdk` and the generated clients. It is specified here rather
than invented during build, because a correction after release is a *second*
breaking change on top of the `UpdateTask` one.

Added to the existing `Host` service in
`apps/backend/proto/kandev/plugin/v1/plugin.proto`, beside `CreateTask` and
`UpdateTask`:

```proto
rpc MoveTask(MoveTaskRequest) returns (MoveTaskResponse);

message MoveTaskRequest {
  string task_id = 1;
  string workflow_step_id = 2;
  optional string workflow_id = 3;
  int32 position = 4;
}

message MoveTaskResponse {
  Task task = 1;
  bool transitioned = 2;
  optional string queued_for_step_id = 3;
  string from_step_id = 4;
}
```

- **`task_id`, not `id`.** `GetTaskRequest` uses a bare `id`, but this message
  carries three identifiers; `DeletePluginOwnedTaskTreeRequest` sets the
  precedent for qualifying (`root_task_id`) when a bare `id` would be
  ambiguous.
- **`workflow_step_id` is a bare `string`, required.** There is no move with no
  destination, so omitted and empty are the same error and presence adds
  nothing.
- **`workflow_id` is `optional`; `position` is a bare `int32`.** proto3 cannot
  distinguish an omitted bare `string` from `""`, and `workflow_id` has a
  meaningful absent case — inherit the task's current workflow — that is
  genuinely distinct from a present empty value, which AC-005.5 rejects. So it
  carries presence. `position` does **not**: AC-005.4 defines an omitted
  position and a position of zero as the same request, both placing the task at
  the top of the target step, so presence would encode a distinction with no
  behavioral consequence and invite an implementer to invent one. An earlier
  draft made `position` `optional` on the reasoning that "no position stated"
  was distinct from zero; it is not, because Out of scope keeps position
  semantics to pass-through and AC-002.3 applies a position change even on a
  same-step move — so a load-bearing absent case would have silently changed
  where a step-only move lands. `int32` matches `plugin.proto:378` and `:445`.
- **Exactly one admission discriminator: the presence of
  `queued_for_step_id`.** There is deliberately no `bool admitted` beside it,
  because two fields encoding one fact can disagree and a builder would have to
  decide which wins. Read as: `transitioned = false` → the task was
  already on the target step; `transitioned = true` with `queued_for_step_id`
  absent → admitted; `transitioned = true` with `queued_for_step_id` present →
  queued for that step, holding no slot. `queued_for_step_id` is reported on
  **both** values of `transitioned`, never suppressed by a no-transition answer:
  per Terminology a queued task *is* on its target step, so a repeat against a
  full step is a same-step move whose honest answer is "no transition, still
  queued here". Suppressing it would make the retry AC-005.1 describes
  indistinguishable from a successful admission.
- **`from_step_id`** is the step the task left, empty when `transitioned` is
  false, so a plugin can render the transition without a second read.
- **Both outcome fields come from the write transaction, not from a pre-read.**
  `MoveTaskResult` (`service_workflow.go:388-391`) carries only `Task` and
  `WorkflowStep` today, so this feature adds `FromStepID string` and
  `Transitioned bool` to it. **Neither is populated from the `oldStepID` and
  `stepChanged` values `MoveTaskWithOptions` computes near `:493`.** Those come
  from the earlier unlocked `GetTask`, and the ledger row is not written from
  them: `updateTaskTx` and every sibling step-writer
  (`UpdateTaskIfWorkflowStepHasCapacity`, the admission-gated path, …)
  independently re-derive the from-step through `readTaskStepInTx` — a
  `FOR UPDATE` read taken *inside* the transaction, after the workspace row
  lock — and pass that to `recordStepTransition`. Under the concurrency
  AC-005.10 pins, the two reads can name different steps, so a response built
  on the earlier one would contradict its own ledger row. *(Round-4
  correction: round 3 specified the `:493` variables here. AC-002.6 itself was
  right and is unchanged; only this mechanism was wrong.)*
- **`Transitioned` is `WorkflowStepTransitionID != 0`.** That field already
  exists on `models.Task`, is assigned from `recordStepTransition`'s return on
  every write path, and reaches the event payload as `step_transition_id`.
  `recordStepTransition` returns `0` and writes no row when the from-step
  equals the to-step, so zero *is* "no transition".
  Deriving it from `stepChanged` instead would let a stale snapshot report
  `transitioned = true` with no ledger row behind it, breaking AC-003.1.
- **`FromStepID` has no carrier yet, so it gets the same one.** The from-step
  the ledger row was written from is carried up beside the transition id: one
  more transient `json:"-"` field on `models.Task`, assigned at the same
  statement, from the same variable. That is the existing
  `WorkflowStepTransitionID` pattern applied to the other half of the same row,
  not a new mechanism. Adding reported fields changes no move semantics, so
  this stays inside Out of scope's boundary. A plugin-seam pre-read is still
  rejected: its own snapshot can disagree with the ledger too. Taking both
  fields from the row the transaction wrote makes AC-002.6's agreement
  structural rather than something a builder must maintain.
- **No enum is introduced.** `plugin.proto` declares none today, and a
  three-value outcome enum would be the only one in the file; the
  presence-based reading carries the same information in the file's existing
  style.
- **The response carries the full `Task`**, so no follow-up `GetTask` is
  needed. Its admission fields are whatever the read shape offers at the time;
  because the outcome is readable from `transitioned` / `queued_for_step_id`
  alone, this feature does not depend on PR #3044 landing.

### Why not route `UpdateTask` internally to the move path

`UpdateTaskRequest` has no `workflow_id` or `position` field, so cross-workflow
moves would stay unrepresentable and position would silently default to the top
of the step. A partial failure across two service methods with no shared
transaction has no defined outcome. And a dedicated RPC lets the response report
admission, which `UpdateTaskResponse` cannot.

## Where `UpdateTask`'s rejection is enforced

The rejection lives on the plugin-facing update path, **not** in the shared task
service. `taskservice.UpdateTask` keeps accepting `WorkflowStepID` unchanged:
its own comment describes it as "a mutation boundary used by plugins and MCP"
(`service_tasks.go:1643`), so rejecting there would break the MCP use that
comment anticipates and would exceed this feature's plugin-only scope.

The seam is the plugins package's own task-update entry and the adapter behind
it — `internal/plugins/host_write.go:203`, which builds `TaskUpdateInput`, and
`pluginsTaskWriterAdapter.UpdateTask` (`internal/backendapp/services.go:1281`),
which is constructed exactly once (`services.go:233`) and wired only into
`pluginsSvc.SetDataSources`, so nothing but the plugin path reaches it.

That adapter already sets the precedent: it validates `state` there and returns
`codes.InvalidArgument`, with a comment saying it does so because "the REST/MCP
path validates state at its HTTP handler". Plugin-only input validation already
belongs at this layer; `workflow_step_id` joins `state`.

The field stays declared in the proto and starts returning an error, rather than
being removed, so a plugin compiled against the current proto gets a named
rejection instead of wire-level unknown-field silence.

**"Carries" means present, not non-empty.** `UpdateTaskRequest.workflow_step_id`
is `optional string` (`plugin.proto:774`), so presence and emptiness are
distinguishable on the wire, and AC-004.3 rejects on *presence* — an explicitly
present `""` is rejected exactly like a present step id. The reasoning is
AC-005.5's, inverted: a present empty value states an intent, and here the
intent it states is "update the step", which is the operation this path no
longer serves. Treating present-empty as absent would leave a silently accepted
no-op on the one path the feature exists to close.

**It is checked before the request's other field validations.** The adapter
already validates `state` and returns `InvalidArgument`; AC-004.3 requires the
error to *name the move operation*, which it cannot do if a co-submitted invalid
`state` answers first. Ordering the step check first makes the guidance
deterministic for a request that is wrong in two ways at once, and matches
AC-004.3's "change no persisted field including the other fields in the same
request".

## Why the session gate wins over retry idempotency

AC-001.8 rejects a move against a task holding any starting or running session.
AC-005.1 promises an identical repeat succeeds as a same-step move. **Spec
Review round 3 found these collide on this feature's own headline path**, and
this section records the decision: **AC-001.8 wins, and AC-005.1 is narrowed to
the window where no session is active.**

The collision is structural, not a wording accident.
`MoveTaskWithOptions` calls `validateTaskMove` at `service_workflow.go:486`;
`stepChanged := oldStepID != workflowStepID` is not computed until `:494`. The
session gate therefore runs *before* the same-step determination exists and
cannot consult it. `validateMoveSessions` (`:1155-1169`) then rejects on **any**
session in `Starting`/`Running` unless `AllowActivePrimarySession` is set — the
option the board's handlers pass and the agent-initiated path does not, and
which AC-001.8 declines by choosing the agent shape.

It bites on the common case, not an edge: AC-003.5 records that a move onto an
auto-start step *precedes* the session it creates, so the first successful move
produces a live session within moments. A webhook redelivery — exactly what
REQ-005's Intent names — arrives inside that window.

**Why AC-001.8 is the one that survives.** Narrowing it instead would need a
same-step carve-out either inside `MoveTaskWithOptions`, which changes move
semantics for every existing caller and is out of scope, or in front of it in
the plugin seam, which AC-001.1 forbids and which races anyway: the task's step
can change between the seam's check and the shared path's read. Narrowing
AC-005.1 needs no shared-path change at all and leaves AC-001.8 a truthful
`(pin)`.

**What the rejection tells a plugin, and why it is a usable answer.** A
`FailedPrecondition` here is not a bare failure — it is the signal that this
task already has a live session, which for a redelivery is precisely the
information the caller wanted: the first delivery landed and the agent is
running, so do not retry. A re-read to "same step, live session" cannot prove
this move caused it — a pre-existing session looks identical, so certainty
needs the plugin's own delivery state. The cost is that a plugin cannot treat
move-and-forget as unconditionally idempotent; it must handle this code. That
cost is stated here and in AC-005.1 rather than discovered at build time.

The narrower promise still holds where it matters: a redelivery arriving after
the agent finished, or against a step with no `auto_start_agent`, finds no live
session and gets AC-005.1's clean no-op.

## Error mapping

Third parties branch on these codes, so they are fixed here rather than left to
the handler. AC-004.6 makes the table binding.

| Case | AC | Code |
|---|---|---|
| Target step unknown, not in the named workflow, or in another workspace | 001.6 | `InvalidArgument` |
| Task is archived | 001.7 | `FailedPrecondition` |
| Task has a starting or running session | 001.8 | `FailedPrecondition` |
| Plugin lacks `api_write:tasks` | 004.1 | `PermissionDenied` |
| `UpdateTask` carries `workflow_step_id` | 004.3 | `InvalidArgument` |
| Empty `task_id` or `workflow_step_id`; present-but-empty `workflow_id` | 005.5 | `InvalidArgument` |
| Task id names no task | 005.6 | `NotFound` |
| Named `workflow_id` is well-formed but no such workflow exists | 001.6 | `InvalidArgument` |
| Task on no workflow and none named | 005.11 | `InvalidArgument` |
| Negative `position` | 005.4 | `InvalidArgument` |
| Target step at its WIP limit | 002.1 | **not an error** — success reporting queued |

**`NotFound` is reserved for the addressed resource.** The task is what the RPC
addresses, so naming no task answers `NotFound`; every other identifier in the
request — the target step, and the workflow qualifying it — describes the
*destination* and is request content, so naming nothing answers
`InvalidArgument`. That rule is what places the unknown-workflow row above, and
it is stated because the table is binding under AC-004.6 and a builder must be
able to place a case the table does not enumerate verbatim.

**When one request trips two rows, the first check in this ladder wins.** The
table lists cases, not order; this orders them: capability (004.1) → request
shape, including `UpdateTask`'s rejected `workflow_step_id` and a negative
position (004.3, 005.5, 005.4) → the task load (005.6) → the task's own state
(001.7, 001.8) → the destination, which can only be checked once the task has
supplied the workflow it is qualified against (001.6, 005.11) → WIP admission,
which is not an error at all. So a request naming both a task that does not
exist and an unknown target step answers `NotFound`, and one from a plugin
lacking the capability answers `PermissionDenied` whichever else is wrong —
which also stops a non-writer probing which task ids exist, the reasoning Reach
applies to a future per-workspace scope. Stated because AC-004.6 gives every row
its own test, so two implementations checking in different orders return
different codes for the same request.

`InvalidArgument` versus `FailedPrecondition` splits on whether the *request* is
malformed or the *task's state* forbids an otherwise well-formed request. An
archived or busy task is the latter: the caller sent nothing wrong and retrying
the identical request can succeed once the state changes.

**The existing classifier must not be reused.** `classifyMoveTaskError`
(`internal/mcp/handlers/config_task_handlers.go:243`) is the only precedent for
turning these errors into caller-facing codes, and it buckets *four* of the
cases above — archived, active session, different workspace, and step not
belonging to the target workflow — into a single `Conflict` by lowercase
substring matching. Two of those are cases AC-001.6 requires to answer
`InvalidArgument`, so adopting it would violate the requirement it appears to
serve. The plugin path maps its own codes at the plugin-facing seam, beside the
`state` validation already there.

The shared validators (`validateTaskMove`, `validateMoveSessions`) stay
untouched: they return bare `fmt.Errorf` strings today, every other caller
depends on that, and adding typed sentinels would be a change to shared move
semantics this feature puts out of scope. The seam therefore recognises those
errors to classify them. That is a real coupling to message text, and it is the
lesser cost — the alternative changes a shared path for every existing caller.
It is contained by keeping the mapping in one function with a test per row of
the table above, so a reworded validator error fails a test rather than silently
degrading a plugin's error handling to `Unknown`.

## Reading the `(pin)` and `(new)` labels

REQ-005's criteria carry one of two labels. A **`(pin)`** records behavior the
system already has, so that routing plugin moves through the shared path cannot
silently change it: it is discharged by a test that the behavior still holds,
needs no new implementation, and a failure is a shared-path regression rather
than work to build here. A **`(new)`** is construction this feature owes.

## Code-path facts that constrain the criteria

### The defect's call sites, in full

`publishTaskMovedEvent` (`service_events.go:719`) has exactly three call sites,
all in `service_workflow.go` — `579`, `882`, `900` — inside `MoveTaskWithOptions`
and the queue-promotion paths. None is reachable from `taskservice.UpdateTask`,
which is why a plugin-set `workflow_step_id` relocates a card and dispatches
nothing.

### The post-commit failure window is real and is not narrowed here

`MoveTaskWithOptions` commits the step change and its in-transaction ledger row
through `updateMovedTask`, and only then publishes `task.moved`, records session
history, runs `pullNextTaskOnVacate` and `pullTasksFromNewFeederWork`, and
finally re-reads the task — returning an error if that re-read fails
(`"failed to refresh task after feeder pull"`). A move can therefore return an
error with the row already written and the agent already dispatched.

This is why the requirements scope "no ledger row" to failures *before* the
commit, and pin the post-commit behavior as-is. The consequence for a plugin
author — a move error does not mean nothing happened, so re-read the task rather
than assume the move was rolled back — belongs in the plugin authoring guide
rather than in an acceptance criterion, since no test of this system can
discharge it. Narrowing the window would
change move semantics for every existing caller, which is out of scope.

### Admitted-versus-queued must be read from `QueuedForStepID`

In `updateTaskWithWorkflowStepAdmission` (`repository/sqlite/task.go`):

```go
admitted = task.IsEphemeral || limit <= 0 || occupants < limit
// on the admitted branch:
task.WIPAdmitted = !task.IsEphemeral
task.QueuedForStepID = ""
```

`WIPAdmitted` and `QueuedForStepID` are not complements. An ephemeral task is
admitted by the function's own return value while persisting
`WIPAdmitted = false` **and** `QueuedForStepID = ""`. Reading `WIPAdmitted`
alone would report that task as queued when it is queued for nothing.

The discriminator is therefore `QueuedForStepID`: queued if and only if it is
non-empty, admitted otherwise. This makes the ephemeral case report correctly
instead of becoming a third outcome. It is defensive rather than reachable
today — task creation rejects an ephemeral task that names a workflow
(`service_tasks.go:594`), so an ephemeral task is not expected to be on a board
— but the branch exists in the writer and an implementer reading it would
otherwise infer a third reportable state.

### Rejections are checked before the write, not atomically with it

`MoveTaskWithOptions` runs `validateTaskMove` (`:486`) — archived task, target
step, workspace, and sessions — before it computes any change and before the
admission write. AC-001.9 states the consequence plainly: "change no persisted
field" in AC-001.6 through AC-001.8 binds *the rejected move*, which writes
nothing on the rejecting path, and is not a claim that the task is unchanged
when the caller observes the error. A concurrent writer committing between the
check and the write is AC-005.10's territory, and this path serializes on the
workspace row lock rather than re-validating under it.

This is the same shape as the post-commit window above: both are properties of
the shared path that this feature pins rather than narrows, because narrowing
either means changing move semantics for every existing caller.

### Promotion ordering is by position and priority first

AC-005.2 pins the order queued tasks are promoted in. The promotion path is
`pullNextTaskOnVacate` → `promoteNextQueuedTask` → `nextQueuedCandidate` →
`NextQueuedTaskForStepExcluding` (`repository/sqlite/task.go:1538`) or
`NextPullCandidateExcluding` (`:1491`).

**AC-005.2's order is the one in `NextQueuedTaskForStepExcluding`**, quoted in
full because the pin test is written from it:

```sql
ORDER BY t.position ASC,
         CASE LOWER(COALESCE(t.priority, ''))
           WHEN 'critical' THEN 0
           WHEN 'high'     THEN 1
           WHEN 'medium'   THEN 2
           WHEN 'low'      THEN 3
           WHEN 'none'     THEN 4
           ELSE 4
         END ASC,
         COALESCE(t.queued_at, t.created_at) ASC,
         t.created_at ASC,
         t.id ASC
```

That `CASE` is the "priority tier" AC-005.2 names: five declared values, with
any unrecognised or absent priority folded onto the same rank as `none`. A pin
test must therefore not assume unknown priorities sort last on their own — they
tie with `none` and are separated by the keys below.

**The two queries do not share this order, and the sibling is not on the
production path.** `NextPullCandidateExcluding` has **four** keys — position,
the same priority `CASE`, `created_at`, `id` — and **no `queued_at` tier at
all**. `nextFeederQueuedCandidate` (`service_workflow.go:1006-1026`) prefers
`NextQueuedTaskForStepExcluding` whenever the repository satisfies that
narrower interface, which the real sqlite `*Repository` always does, so
`NextPullCandidateExcluding` is reached only by test doubles. Pinning AC-005.2
against it would produce a red test that looks like a shared-path regression and
is not one.

Position and priority tier are the primary and secondary keys; queued-at is only
the third. An earlier draft of AC-005.2 named queued-at as the primary key,
having read the `ORDER BY` of `ListQueuedTasks` (`:1705`) — a general listing
helper that is not on the promotion path at all. A pin test written from that
wording fails against any data with mixed positions or priorities, and the
`(pin)` convention would then have pointed the implementer at a shared-path
regression that does not exist.

### The plugin path does not rebase onto a concurrent writer's state

AC-005.10 pins concurrent-move behavior, and what it can pin is bounded by which
admission writer this path uses. `updateTaskWithWorkflowStepAdmission` runs
`rebaseTaskForStepAdmissionCAS` only when `expectedStepID != ""`, and the sole
caller passing a non-empty expected step is the workflow engine's guarded
re-evaluation (`orchestrator/workflow_store.go:429`). `MoveTaskWithOptions`
reaches the writer with no expected step and captures `oldStepID` from a read
taken before the transaction.

That is deliberate. The writer's own comment: "The unconditional
(`expectedStepID == ""`) callers — manual/bulk/feeder moves — must NOT call
this: their caller-built `task` already carries this operation's own field
changes (Position, move-lifecycle metadata, …) that have no other source of
truth, so rebasing from a fresh row would drop them instead of protecting them."

So the plugin path serializes on the workspace row lock, commits in arrival
order, and writes the caller's snapshot: the *destination* and caller-built
fields are the ones this caller asked for, not a rebase onto what the earlier
writer committed. The recorded *from*-step is a separate read — `readTaskStepInTx`,
inside the write transaction under its row lock — so the ledger chain stays
contiguous even where the caller's snapshot had gone stale. AC-005.10 states that
split rather than promising a rebase, because promising one would mean changing
shared move semantics for every existing caller — out of scope, and the reason the
CAS variant exists as a separate entry point in the first place.

## Reach

AC-005.6 answers `NotFound` for a task id that names no task, and deliberately
says nothing more restrictive. Should a future contract introduce per-workspace
plugin scoping — listed under Out of scope — a task outside a plugin's scope
should also answer `NotFound` rather than a permission error, so that scoping
cannot be used to probe which task ids exist. That is recorded as the intended
direction, not as a criterion this feature discharges.

A plugin holding `api_write:tasks` may move any task in the install, in any
workspace. There is no narrower scope available to enforce: `hostForPlugin`
binds no workspace, and `authorizeTaskID` (`service_access.go:59`) returns `nil`
immediately when `callerScope(ctx)` reports the caller unscoped, which is every
plugin gRPC context. A plugin is an install-level extension the operator chose
to install — the same trust level at which it can already create tasks and start
agents.

Per-workspace plugin scoping is a separate contract. If it is ever added, this
reach narrows with it and no acceptance criterion changes, because the criteria
state the not-found convention rather than a workspace filter.
