---
status: draft
system: plugins
created: 2026-08-26
owners:
  - nova28
---

# Plugin-Initiated Workflow Step Transitions Requirements

> Technical design, wire contract, and code-path evidence:
> `docs/specs/plugins/system-design/plugin-initiated-step-transitions.md`.

## Overview

A plugin can already relocate a card on the board, and doing so starts nothing.
`Host.UpdateTask` accepts `workflow_step_id` and the adapter forwards it to
`taskservice.UpdateTask`, which assigns `task.WorkflowStepID` directly
(`service_tasks.go:1593`) and never calls `publishTaskMovedEvent`, whose only
call sites are the shared move and queue-promotion paths. Without that event
`autoStartTaskForStep` never runs, so no `on_enter` action fires and no agent
launches. A plugin "Dispatch" button built on `UpdateTask` moves cards and
starts no work.

This system owns the contract because the durable decision is *what a plugin
may do to an operator's board and under whose identity*, not how a move is
implemented.

## Terminology

- **Move:** a change to a task's `workflow_step_id`, `workflow_id`, or
  `position` through the task service's move path.
- **Transition:** a move where the step actually changed. A same-step move is
  not a transition and writes no ledger row (`step_transitions.go:74`).
- **Admitted / Queued:** a move is **queued** if and only if `QueuedForStepID`
  is non-empty, meaning the task *is* on the target step but holds no capacity
  slot (`internal/task/models/models.go:805-810`); every other outcome is
  **admitted**. The discriminator is `QueuedForStepID`, not `WIPAdmitted`; the
  design file records why the two are not complements.
- **Plugin identity:** the string `plugin:<id>`, already produced by
  `pluginHost.pluginSource()` and stamped on plugin-created tasks and messages.

## Prior art

Recorded in the design file: the wiki was unavailable (tooling access, not an
empty wiki); one on-point `saas-kb` hit (Factory.ai's Actor/Source audit
split), plus what we do differently.

## Contract decisions

The argument, rejected alternatives, and code citations are in the design file.

**Actor: `integration`, identified by `plugin:<id>`, under a new `plugin_move`
trigger.** Not a new `plugin` actor kind, and not the authenticated user driving
the surface: there is no `authn` identity on a plugin gRPC context, and a plugin
can act on a webhook or timer with no human anywhere. `ActorIntegration`
already covers this shape of caller. `StepHistoryActor` must not be `Human`, because `Human` is what stamps the
operator's user id onto an unattended move.

**`plugin_move` is added to both trigger vocabularies.** The task-level ledger
and the session-level history use separate, non-overlapping enums. Adding it to
only one leaves the session history recording `manual`, the value reserved for
human board moves, on every plugin move.

**Capability: `api_write:tasks`, unchanged.** It already confers agent launch,
so a new capability closes no open door. The `PluginOwnedTaskTree` ownership
rule is deliberately not applied; safety comes from the auto-start gates, not
from ownership. The design file argues both.

**Reach, stated plainly: a plugin holding `api_write:tasks` may move any task in
the install, in any workspace.** Deliberate, not an omission: a plugin host
binds no workspace and no scope filter applies. Per-workspace scoping is a
separate contract (see Out of scope).

**Shape: a dedicated `MoveTask` RPC; `UpdateTask` stops accepting the field.**
`UpdateTaskRequest` has no `workflow_id` or `position`, so routing it internally
leaves cross-workflow moves unrepresentable and its response cannot report
admission. The design file specifies the full field list and the single
admission discriminator, because third parties compile against it. `UpdateTask`
must then reject `workflow_step_id` — a breaking change to a shipped field, and
the right one, since every current use is already silently broken — enforced on
the plugin-facing update path only, never in the shared task-service method that
MCP also uses.

## Requirements

### REQ-PLUGINS-STEP-MOVE-001: Plugin-initiated moves use the board's move path

**Intent:** A plugin that moves a card runs the same move implementation the
board runs — the same validation, admission, event publication, and step-entry
dispatch — so a plugin surface can dispatch work.

**One deliberate divergence.** The plugin path is *not* option-for-option
identical to the board's: a plugin move is unattended, so it takes the
stricter agent-shaped option and rejects a task with a live session (AC-001.8).
Read AC-001.1 as "same method, same gates", not "same options"; the design file
names the option, why it diverges, and what it costs a retrying plugin.

**User story:** As a plugin author, I want to move a task to a workflow step
and have the step's configured actions run, so that a "Dispatch" control in my
plugin starts work instead of silently relocating a card.

#### Acceptance criteria

- **AC-PLUGINS-STEP-MOVE-001.1:** When a plugin requests a move, the system
  shall perform it through the same task-service move method the board's HTTP
  and WebSocket handlers call, and shall not re-implement validation,
  admission, event publication, or step-entry dispatch in the plugin layer.
  The move options are not identical to the board's: see AC-001.8 and Intent.
- **AC-PLUGINS-STEP-MOVE-001.2:** When a plugin moves a task onto a step whose
  `on_enter` actions include `auto_start_agent`, and no auto-start gate
  applies, the system shall start an agent for that task.
- **AC-PLUGINS-STEP-MOVE-001.3:** When a plugin moves a task onto a step
  without an `auto_start_agent` action, the system shall complete the move and
  start no agent.
- **AC-PLUGINS-STEP-MOVE-001.4:** When a plugin move would start an agent but
  the task has an unresolved dependency, is queued for capacity, has a terminal
  pull request, or consumes a deferred launch intent instead, the system shall
  complete the move and start no agent, applying each of those gates exactly as
  the board's move does.
- **AC-PLUGINS-STEP-MOVE-001.5:** When a plugin moves an Office task, the
  system shall route step entry to the Office run path rather than the
  session-based launch path, leaving the Office scheduler's ownership intact.
- **AC-PLUGINS-STEP-MOVE-001.6:** When a plugin names a target step that does
  not exist, does not belong to the named workflow, or belongs to a workflow in
  a different workspace than the task, the system shall reject the move with an
  invalid-argument error and change no persisted field.
- **AC-PLUGINS-STEP-MOVE-001.7:** When a plugin moves an archived task, the
  system shall reject the move with a failed-precondition error and change no
  persisted field.
- **AC-PLUGINS-STEP-MOVE-001.8:** When a plugin moves a task that has any
  session in a starting or running state, the system shall reject the move with
  a failed-precondition error and change no persisted field, matching the
  agent-initiated move path rather than the human board path.

- **AC-PLUGINS-STEP-MOVE-001.9:** The rejections AC-001.6 through AC-001.8
  name shall be evaluated before the move's write. "Change no persisted field"
  binds the rejected move itself, not a concurrent writer that commits between
  the check and the write.

### REQ-PLUGINS-STEP-MOVE-002: The move's outcome is reportable

**Intent:** A plugin can tell what actually happened, so it renders truth
rather than an assumption.

**User story:** As a plugin author, I want the move result to tell me whether
the task was admitted, queued, or unchanged, so that my UI does not claim work
started when it is waiting for capacity.

#### Acceptance criteria

- **AC-PLUGINS-STEP-MOVE-002.1:** When a plugin move lands on a step that is at
  its WIP limit, the system shall treat the move as successful, place the task
  on the target step holding no capacity slot, and report the queued outcome
  and the step it is queued for.
- **AC-PLUGINS-STEP-MOVE-002.2:** When a plugin move is admitted, the system
  shall report the admitted outcome, deriving admitted-versus-queued solely
  from whether the task is queued for a step and never from the capacity-slot
  flag alone.
- **AC-PLUGINS-STEP-MOVE-002.3:** When a plugin requests a move to the step the
  task already occupies, the system shall apply any position change, report
  that no transition occurred, write no transition ledger row, publish no move
  event, and start no agent. The answer shall still report the task's admission
  state as AC-002.2 derives it, so a repeat against a step that is at its WIP
  limit reports the task still queued.
- **AC-PLUGINS-STEP-MOVE-002.4:** The system shall report the move outcome in
  fields owned by the move response itself, so that reporting does not depend
  on any other change to the task read shape, and shall carry exactly one
  admission discriminator rather than two fields that could disagree.
- **AC-PLUGINS-STEP-MOVE-002.5:** The system shall answer the move once the step
  change is committed, without waiting for step-entry actions to finish, and the
  answer shall report admission rather than whether an agent started. AC-005.13
  governs when post-commit work fails.
- **AC-PLUGINS-STEP-MOVE-002.6:** The system shall report the step the task
  left, read in the same operation that determines whether the step changed, so
  that the reported from-step and the transition ledger row name the same step
  for the same move.

### REQ-PLUGINS-STEP-MOVE-003: Plugin moves are attributable

**Intent:** An operator can tell which plugin moved a card, and a plugin move
is never recorded as a person's action.

**User story:** As an operator, I want a plugin's move recorded as that
plugin's action, so that an unattended automated move is not attributed to me.

#### Acceptance criteria

- **AC-PLUGINS-STEP-MOVE-003.1:** When a plugin move changes a task's step, the
  system shall record a transition ledger row whose actor kind is the
  integration actor and whose actor identifier is that plugin's `plugin:<id>`
  provenance string.
- **AC-PLUGINS-STEP-MOVE-003.2:** When a plugin move changes a task's step, the
  system shall record a trigger value distinct from every existing trigger in
  **both** the task-level ledger's and the session-level history's trigger
  vocabularies, and shall not change the ledger's collection-contract version.
  The session-level row is written only when the task already has a session;
  see AC-003.5.
- **AC-PLUGINS-STEP-MOVE-003.3:** When a plugin move is recorded, the system
  shall not write any authenticated user's identifier as the move's actor
  identifier on either the task-level ledger or the session-level history, and
  the session-level history's trigger shall not fall back to the manual value
  reserved for human board moves.
- **AC-PLUGINS-STEP-MOVE-003.4:** When a plugin move changes a task's step, the
  system shall record exactly one transition ledger row for that move.
- **AC-PLUGINS-STEP-MOVE-003.5:** When a plugin moves a task that has no
  session, the system shall write the task-level ledger row and write no
  session-level history row, because that history's session column is a
  non-null foreign key to the session table. This is the feature's most common
  case, since a move onto an auto-start step precedes the session that move
  creates. The session-level trigger value is therefore exercised only by moves
  of tasks that already have a session.

### REQ-PLUGINS-STEP-MOVE-004: One move path, gated by task write access

**Intent:** The corrected path is the only plugin move path, so a second one
cannot drift from the board's.

**User story:** As a maintainer, I want exactly one way for a plugin to change
a task's step, so that a future change to move semantics cannot leave a plugin
path behind.

#### Acceptance criteria

- **AC-PLUGINS-STEP-MOVE-004.1:** When a plugin without declared task write
  access requests a move, the system shall deny it as a permission failure and
  change no persisted field.
- **AC-PLUGINS-STEP-MOVE-004.2:** When a plugin with declared task write access
  requests a move, the system shall not require any additional capability
  beyond task write access.
- **AC-PLUGINS-STEP-MOVE-004.3:** When a plugin requests a task update whose
  workflow step identifier is present, whether or not its value is empty, the
  system shall reject the request with an invalid-argument error that names the
  move operation, and shall change no persisted field including the other fields
  in the same request. This rejection shall be evaluated before the request's
  other field validations, so that a request carrying both a step identifier and
  another invalid field names the move operation.
- **AC-PLUGINS-STEP-MOVE-004.5:** The rejection in AC-004.3 shall be enforced on
  the plugin-facing update path only. A non-plugin caller of the shared
  task-service update method shall retain its ability to set a workflow step
  identifier, leaving the shared method's role for other callers unchanged.
- **AC-PLUGINS-STEP-MOVE-004.4:** The set of code paths permitted to mutate a
  task's workflow step shall remain pinned by test after this change, with the
  plugin move path routing through an already-pinned path rather than adding a
  new one.
- **AC-PLUGINS-STEP-MOVE-004.6:** Every rejection these requirements name shall
  answer with the status code the system design's error-mapping table assigns
  it, and each such code shall be observable by a plugin. The plugin path shall
  not reuse the MCP handler's move-error classifier, which buckets step and
  workflow mismatches as a conflict where AC-001.6 requires invalid-argument.

### REQ-PLUGINS-STEP-MOVE-005: Defined behavior under retry, races, and absent input

**Intent:** Concurrency, retry, and defaulting behavior are stated rather than
discovered by an implementer.

**User story:** As a plugin author, I want retry and race behavior defined, so
that a webhook redelivery or two plugins acting at once has a knowable result.

Criteria below are labelled **(pin)** or **(new)**; the design file defines
both labels.

#### Acceptance criteria

- **AC-PLUGINS-STEP-MOVE-005.1:** (new) When a plugin repeats an identical move
  request after one has already succeeded, and the task holds no starting or
  running session, the system shall treat the repeat as a same-step move: it
  shall succeed, report that no transition occurred, and start no additional
  agent. When such a session exists, AC-001.8 governs instead and the repeat is
  rejected with a failed-precondition error; the design file states why the gate
  wins and what that rejection tells a plugin.
- **AC-PLUGINS-STEP-MOVE-005.2:** (pin) When two callers move the same task
  concurrently, the system shall serialize them on the target step's capacity
  so that admissions never exceed the step's WIP limit, and shall promote
  queued tasks in the shared path's existing order: position, then priority
  tier, then queued-at falling back to created-at, then created-at, then task
  identifier. The design file names the priority ladder and the query this
  order belongs to; both are binding on the pin test.
- **AC-PLUGINS-STEP-MOVE-005.3:** (pin) When a move event is delivered for a
  transition the task has already left, the system shall not run the superseded
  destination's step-entry actions.
- **AC-PLUGINS-STEP-MOVE-005.4:** (new) When a plugin omits the target workflow
  identifier, the system shall use the task's current workflow, and that
  omission shall be carried by field presence so that an omitted identifier and
  a present empty one are distinguishable on the wire. Position carries no
  presence: an omitted position and a position of zero are the same request,
  both writing position zero; a negative position is invalid.
- **AC-PLUGINS-STEP-MOVE-005.5:** (new) When a plugin supplies an empty task
  identifier or an empty target step identifier, the system shall reject the
  request with an invalid-argument error naming the missing field. A
  present-but-empty target workflow identifier shall likewise be rejected rather
  than fall back to the task's current workflow, since a present empty value
  states an intent that cannot be honored.
- **AC-PLUGINS-STEP-MOVE-005.6:** (new) When a plugin names a task identifier that
  does not exist, the system shall answer not-found. A plugin holding task write
  access is not otherwise restricted in which tasks it may name.
- **AC-PLUGINS-STEP-MOVE-005.7:** (new) When a move fails before the step change is
  committed, the system shall leave no transition ledger row for that move and
  change no persisted field.
- **AC-PLUGINS-STEP-MOVE-005.13:** (pin) When a move fails *after* the step
  change is committed, the system shall report the failure while leaving the
  committed step change, its ledger row, and any dispatched step-entry actions
  in place. This feature shall not narrow the window for the existing callers
  that share the path; the design file records what runs inside it.
- **AC-PLUGINS-STEP-MOVE-005.8:** (pin) When a plugin move vacates a step that has
  tasks queued for it, the system shall run the same capacity reconciliation
  the board's move runs, and shall record the resulting promotion under its own
  trigger rather than the plugin's.
- **AC-PLUGINS-STEP-MOVE-005.9:** (new) The system shall provide no caller-supplied
  deduplication key for moves. A repeated request is judged only against the
  task's step at that moment, so a redelivery arriving after the task returned
  to its origin shall be treated as a fresh move and may start an agent again.
- **AC-PLUGINS-STEP-MOVE-005.10:** (pin) When two callers move the same task to
  different steps concurrently, the system shall serialize the writes on the
  workspace row lock, commit them in the order they reach the task write, and
  record one transition row per committed change rather than collapsing them.
  It shall not rebase a plugin move onto the state the earlier writer
  committed: the destination is the caller's, while the recorded from-step is
  read under the write transaction's own row lock.
- **AC-PLUGINS-STEP-MOVE-005.11:** (new) When a plugin moves a task that is attached
  to no workflow and names no target workflow, the system shall reject the move
  with an invalid-argument error rather than infer a workflow from the step.
- **AC-PLUGINS-STEP-MOVE-005.12:** (pin) When the target step's WIP limit is absent,
  zero, or negative, the system shall admit the move rather than queue it,
  treating a non-positive limit as unlimited, and shall report it as admitted
  because the task is queued for no step.
- **AC-PLUGINS-STEP-MOVE-005.14:** (pin) When the moved task is ephemeral, the
  system shall admit the move and report it as admitted even though the task
  holds no capacity slot, because an ephemeral task is queued for no step.

## Out of scope

- **Changing move semantics for any existing caller.** The board's HTTP and
  WebSocket handlers, the MCP move tool, bulk move, queue promotion, and launch
  recovery keep their current behavior, options, and attribution — including the
  post-commit failure window pinned by AC-005.13.
- **A new plugin actor kind or a ledger contract-version bump.** The existing
  integration actor and an added trigger value cover this.
- **Exposing move history to plugins.** Reading the ledger or session history
  through the Host API is a separate contract.
- **A frontend Host API move.** That surface holds no task-write path; adding
  one is a separate decision with a different actor answer, since it does carry
  an authenticated user.
- **Position semantics beyond passing the caller's value through.** Kandev does
  not renumber siblings on move today and this does not introduce it.
- **Making `Task`'s read shape carry board fields.** That is PR #3044's
  contract; this feature reports its outcome without depending on it.
- **Plugin-initiated task state changes.** `UpdateTask`'s `state` field is
  unchanged.
- **Per-workspace plugin scoping.** Whether plugins should be installable per
  workspace affects every capability, not just moves.
- **Removing `workflow_step_id` from the update request message.** The field
  stays declared and starts returning an error, so a plugin compiled against the
  current proto gets a named rejection rather than wire-level silence.
