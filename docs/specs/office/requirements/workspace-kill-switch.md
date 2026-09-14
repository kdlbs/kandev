---
status: draft
system: office
created: 2026-09-06
owners:
  - kandev
---

# Office Workspace Kill Switch Requirements

## Overview

Office runs unattended. Nobody is in the turn. There is no way to stop it.

Existing controls are granular and incomplete: `office_routines.status` and
`office_routine_triggers.enabled` are per-routine and per-trigger, and
`agent_profiles.status = 'paused'` is per-agent. Halting a workspace today means
N + M + K untransacted writes while the scheduler keeps claiming runs between
them, and none covers the event-driven path: a comment, an assignment, an
approval resolution or a webhook still queues a run after every routine is paused.

This capability gives an operator one write that stops one workspace, records who
stopped it and why, and is released only by explicit human action.
`KANDEV_FEATURES_OFFICE` is no substitute: a rollout gate, not an operational
control. Ownership, rejected alternatives and adjacent-contract boundaries are in
the design.

## Terminology

- **Office workspace:** a workspace containing Office agents, identified by
  `workspaces.id` and referenced as `agent_profiles.workspace_id`.
- **Pause record:** the durable record that makes a workspace paused. A
  workspace is paused exactly when it has an unreleased pause record.
- **Launch:** any transition that starts or resumes Office work: dispatching a
  routine run, creating an Office `runs` row, or claiming a queued run and
  starting an agent for it.
- **In-flight work:** an Office `runs` row in status `queued` or `claimed`, plus
  any agent execution running for an Office task in the workspace. The `runs`
  table has no `running` status; `claimed` is the executing state.
- **Halt sweep:** the cancellation of in-flight work when a pause is created.
- **Gate:** the read of pause state at a launch point.

## Requirements

### REQ-OFFICE-KILL-SWITCH-001: Authoritative workspace pause state

**Intent:** One write must put the workspace into a single, unambiguous stopped
state; a fan-out over N routines and M agents is not atomic.

**User story:** As an operator, I want to stop one Office workspace with a single
action, so that the loop cannot launch while I am still writing.

The state is authoritative **on read**: whether a workspace is paused is answered
by the pause record alone, never inferred from any routine, trigger or agent
status.

#### Acceptance criteria

- **AC-OFFICE-KILL-SWITCH-001.1:** When a workspace has no unreleased pause
  record, the system shall treat the workspace as running.
- **AC-OFFICE-KILL-SWITCH-001.2:** When a pause is requested for a workspace
  that is running, the system shall create exactly one pause record and
  report the workspace paused from then on.
- **AC-OFFICE-KILL-SWITCH-001.3:** When two pause requests for the same
  workspace are processed concurrently, the system shall leave exactly one
  unreleased pause record and return that identifier to both callers.
- **AC-OFFICE-KILL-SWITCH-001.4:** When a pause is requested, the system shall
  not modify `agent_profiles.status`, `office_routines.status`, or
  `office_routine_triggers.enabled` for any row.
- **AC-OFFICE-KILL-SWITCH-001.5:** When a pause is requested for a workspace
  that is already paused, the system shall leave the existing record's actor,
  reason and creation timestamp unchanged and report success.
- **AC-OFFICE-KILL-SWITCH-001.6:** When the backend restarts, the system shall
  report a workspace paused before the restart as still paused, with unchanged
  provenance.
- **AC-OFFICE-KILL-SWITCH-001.7:** When a pause record is created, the system
  shall not change launch behavior for any other workspace.

### REQ-OFFICE-KILL-SWITCH-002: Every launch path is gated

**Intent:** A stop covering cron but not events is not a stop, and the event path
is the half nothing covers today.

**User story:** As an operator, I want a paused workspace to launch nothing, so
that I need not reason about which trigger kinds my stop reached.

#### Acceptance criteria

- **AC-OFFICE-KILL-SWITCH-002.1:** While a workspace is paused, when a cron
  trigger of one of its routines becomes due, the system shall not dispatch a
  routine run.
- **AC-OFFICE-KILL-SWITCH-002.2:** While a workspace is paused, when a cron
  trigger becomes due, the system shall advance that trigger's next-fire cursor
  as if it had fired, so that resuming does not release a backlog of missed
  ticks.
- **AC-OFFICE-KILL-SWITCH-002.3:** While paused, when a webhook fires one of its
  routine triggers, the system shall reject it with HTTP 409, include the
  pause reason in the response body, and not dispatch.
- **AC-OFFICE-KILL-SWITCH-002.4:** While paused, when an operator fires one of
  its routines manually, the system shall reject it with HTTP 409 and not
  dispatch.
- **AC-OFFICE-KILL-SWITCH-002.5:** While a workspace is paused, when any event
  that would ordinarily wake one of its agents occurs, the system shall not
  create an Office run row, whatever the wake reason, including wake reasons
  added after this requirement.
- **AC-OFFICE-KILL-SWITCH-002.6:** While paused, when such an event occurs, the
  system shall still complete the originating write, so that the comment,
  assignment or approval is persisted.
- **AC-OFFICE-KILL-SWITCH-002.7:** While paused, when the scheduler claims a run
  belonging to one of its agents, the system shall not launch an agent and shall
  move that run to a terminal state recording the workspace pause.
- **AC-OFFICE-KILL-SWITCH-002.8:** While paused, the system shall serve Office
  read endpoints unchanged.
- **AC-OFFICE-KILL-SWITCH-002.9:** When the gate cannot determine pause state
  because the read fails, the system shall not launch, shall record the gate
  failure, and shall leave the blocked work in its most retryable state, not
  a terminal one: a claimed run stays `queued`, an unprocessed wakeup
  request stays unprocessed, and a cron tick whose next-fire cursor has already
  advanced is skipped without restoring the cursor, so that the routine fires
  again on its next cadence.
- **AC-OFFICE-KILL-SWITCH-002.10:** When an agent has no workspace attribution,
  the system shall leave its launch behavior unchanged, since no pause record can
  apply to it.
- **AC-OFFICE-KILL-SWITCH-002.11:** While a workspace is paused, when a routine
  fire is blocked by any trigger kind, the system shall record the blocked
  attempt as a skipped automation run carrying a durable attribution
  distinguishing a workspace pause from every other skip cause, and naming which
  pause blocked it, so the reason is recoverable from the run's own row.
- **AC-OFFICE-KILL-SWITCH-002.12:** When a gate read fails while a routine fire
  is being dispatched, the system shall reject a webhook or manual fire with
  HTTP 503, rather than the 409 used for a confirmed pause, so that a caller can
  tell an operator's stop from an outage.
- **AC-OFFICE-KILL-SWITCH-002.13:** The system shall record at most one
  pause-attributed skipped run per routine per pause record, however many fires
  that pause blocks, so that a long pause does not bury the routine's history.

### REQ-OFFICE-KILL-SWITCH-003: Pause halts work already in flight

**Intent:** A control that only prevents the next launch does not answer the
runaway it was reached for.

**User story:** As an operator, I want pausing to stop the agents currently
working, so that a runaway stops when I press the button, not after its turn.

#### Acceptance criteria

- **AC-OFFICE-KILL-SWITCH-003.1:** When a pause is requested, the system shall
  make the pause record durable before beginning the halt sweep.
- **AC-OFFICE-KILL-SWITCH-003.2:** When the halt sweep runs, the system shall
  cancel every Office run in status `queued` or `claimed` belonging to an agent
  in the workspace, recording the cancellation reason on each.
- **AC-OFFICE-KILL-SWITCH-003.3:** When the halt sweep runs, the system shall
  request cancellation of every in-flight agent execution for an Office task in
  the workspace.
- **AC-OFFICE-KILL-SWITCH-003.4:** When the halt sweep cancels a run that holds
  a task checkout, the system shall release that checkout, so that the task is
  not locked until the stale-checkout reaper runs.
- **AC-OFFICE-KILL-SWITCH-003.5:** When the halt sweep runs, the system shall
  leave runs already in a terminal state unchanged.
- **AC-OFFICE-KILL-SWITCH-003.6:** When a pause is requested for a workspace
  that is already paused, the system shall run the halt sweep again, so that a
  repeated request clears anything that raced the first one.
- **AC-OFFICE-KILL-SWITCH-003.7:** When one cancellation within the halt sweep
  fails, the system shall continue the sweep, report the pause as in effect,
  and record the failed cancellations.

### REQ-OFFICE-KILL-SWITCH-004: Provenance

**Intent:** An unattended system that stopped must say who stopped it and why.

**User story:** As an operator arriving at a stopped workspace, I want to see who
stopped it, when and why, so that I can tell an incident from maintenance.

#### Acceptance criteria

- **AC-OFFICE-KILL-SWITCH-004.1:** When a pause record is created, the system
  shall record the requesting actor's identity and kind, the supplied reason,
  and the creation timestamp.
- **AC-OFFICE-KILL-SWITCH-004.2:** When a pause is requested without a reason,
  or with a reason that is empty after trimming surrounding whitespace, the
  system shall reject the request with HTTP 400 and shall not create a pause
  record.
- **AC-OFFICE-KILL-SWITCH-004.3:** When a pause is requested with a reason
  longer than 500 Unicode code points, counted after trimming surrounding
  whitespace, the system shall reject the request with HTTP 400 and shall not
  create a pause record.
- **AC-OFFICE-KILL-SWITCH-004.4:** When a pause record is created or released,
  the system shall write a workspace activity log entry naming the actor, the
  reason, and which of the two actions occurred.
- **AC-OFFICE-KILL-SWITCH-004.5:** When a pause record is released, the system
  shall record the releasing actor, the release reason, and the release
  timestamp on that same record, and shall retain the record and its creation
  provenance.
- **AC-OFFICE-KILL-SWITCH-004.6:** When the halt sweep completes, the system
  shall record the number of runs cancelled and the number of task executions
  it requested cancellation for.
- **AC-OFFICE-KILL-SWITCH-004.7:** When authentication is disabled and no user
  identity is available, the system shall record the actor as the documented
  single-user sentinel with actor kind `user`, and shall not leave the actor
  field empty.
- **AC-OFFICE-KILL-SWITCH-004.8:** When a release reason is supplied, the system
  shall apply the same validation as a pause reason, except that it can be
  omitted or blank.
- **AC-OFFICE-KILL-SWITCH-004.9:** When a reason is accepted, the system shall
  store the trimmed value, so that the rendered reason is the validated one, and shall store an omitted or blank release reason as an empty
  string.

### REQ-OFFICE-KILL-SWITCH-005: Resume is explicit and effective

**Intent:** Nothing may restart a paused workspace except a human asking, and
"resumed" is not a claim this capability may make without an observable launch
behind it.

**User story:** As an operator, I want resuming to be deliberate and provably
effective, so that I neither restart the loop by accident nor believe I restarted
it when I did not.

#### Acceptance criteria

- **AC-OFFICE-KILL-SWITCH-005.1:** When a workspace is paused, the system shall
  keep it paused until an explicit resume is processed, regardless of elapsed
  time, backend restarts, or configuration reloads.
- **AC-OFFICE-KILL-SWITCH-005.2:** When a workspace is resumed and an event
  subsequently occurs that would wake an agent in that workspace that is not
  itself paused or stopped, the system shall create a run for it and shall
  launch an agent for that run.
- **AC-OFFICE-KILL-SWITCH-005.3:** When a workspace is resumed and one of its
  cron triggers next becomes due, the system shall dispatch a routine run for
  it.
- **AC-OFFICE-KILL-SWITCH-005.4:** When a workspace is resumed, the system shall
  not restore runs cancelled by the halt sweep, nor re-queue work on their
  behalf.
- **AC-OFFICE-KILL-SWITCH-005.5:** When a workspace is resumed, the system shall
  not change `agent_profiles.status` for any agent, so that an agent paused for
  budget or consecutive failures remains paused.
- **AC-OFFICE-KILL-SWITCH-005.6:** When a resume is requested for a workspace
  that is not paused, the system shall report success and shall create or
  release no pause record.
- **AC-OFFICE-KILL-SWITCH-005.7:** When two resume requests for the same
  workspace are processed concurrently, the system shall release the pause
  record exactly once and report success to both callers.
- **AC-OFFICE-KILL-SWITCH-005.8:** When a pause request and a resume request for
  the same workspace are processed concurrently, the system shall apply them in
  commit order, leave the workspace in the state written by the later commit,
  and write an activity log entry for each request, recording whether it
  committed, so that a rejected request is auditable too.

### REQ-OFFICE-KILL-SWITCH-006: Operator surface

**Intent:** A stop nobody can find is a stop nobody uses: reachable by API,
unmissable in the UI, workspace-scoped like every other Office route.

**User story:** As an operator, I want the pause control and the paused state on
every Office page, so that I can stop the workspace from wherever I noticed the
problem and cannot mistake a paused one for an idle one.

#### Acceptance criteria

- **AC-OFFICE-KILL-SWITCH-006.1:** When a caller requests a workspace's pause
  state, the system shall return whether it is paused and, when paused, the
  creating actor, reason and timestamp.
- **AC-OFFICE-KILL-SWITCH-006.2:** When a caller requests a pause or a resume
  for a workspace they do not have access to, the system shall deny it under
  the same workspace authorization as every other Office route.
- **AC-OFFICE-KILL-SWITCH-006.4:** While a workspace is paused, the system shall
  display a persistent indicator on every Office page for that workspace,
  naming the actor, reason and time, and offering a resume control.
- **AC-OFFICE-KILL-SWITCH-006.5:** When an operator resumes from that indicator,
  the system shall require an explicit confirmation before sending the request.
- **AC-OFFICE-KILL-SWITCH-006.6:** When a pause or resume request fails, the
  system shall keep the displayed state matching the server's and surface the
  failure.
- **AC-OFFICE-KILL-SWITCH-006.7:** The system shall render every string this
  capability introduces from the localization catalogs, in every supported
  locale.
- **AC-OFFICE-KILL-SWITCH-006.8:** On a 390x844 phone viewport the system shall
  render the indicator without clipping its actor, reason or time, and keep
  the pause, resume and confirmation controls reachable and operable, so that
  the same capability is available as on desktop.
- **AC-OFFICE-KILL-SWITCH-006.9:** When a request to read, pause or resume names a
  workspace that does not exist, the system shall reject it with HTTP 404 and
  shall create no record, including when authentication is disabled and the
  shared scope check is a pass-through.
- **AC-OFFICE-KILL-SWITCH-006.10:** When persisting a pause or a release fails,
  the system shall reject the request with HTTP 500 and shall leave the
  workspace in the state it held before the request, so that a failed resume
  leaves the workspace paused.
- **AC-OFFICE-KILL-SWITCH-006.11:** When an Office agent calls the pause or
  resume endpoint, the system shall reject the request with HTTP 403, so that an
  agent can neither stop its own workspace nor undo an operator's stop.
- **AC-OFFICE-KILL-SWITCH-006.12:** While a workspace is not paused, the system
  shall offer a pause control on every Office page for that workspace, and shall
  require both a reason and an explicit confirmation before sending the request,
  so that the stop is reachable from wherever the operator noticed it.
- **AC-OFFICE-KILL-SWITCH-006.13:** The system shall offer a refresh control
  wherever it displays pause state, and when an operator invokes it shall re-read
  that state and display what the server returned, so that a client holding a
  stale or unavailable state can be corrected without reloading the page.

## Out of scope

- **A drain mode that lets in-flight work finish.** REQ-OFFICE-KILL-SWITCH-003
  chooses halt. A second mode doubles the behavior matrix, the API and the UI for
  a capability whose value is that an operator need not think. A future
  requirement may add drain as a distinct mode; it must not silently change what
  pause means.
- **Rendering the pause attribution in the routine runs UI.**
  AC-OFFICE-KILL-SWITCH-002.11 makes the reason durable on the run row; no
  requirement renders it on a page. The design records what a follow-up needs.
- **An instance-wide stop across all workspaces.** The scope is one workspace,
  matching how Office agents, routines and runs are already scoped.
- **Automatic pausing.** Nothing here creates a pause record on its own.
  Budget-exceeded and runaway/WIP detection are separate capabilities; if either
  later stops a workspace it does so as a caller of this contract, recording
  itself as the actor.
- **Fixing the existing per-agent resume.** A separate contract: this capability
  never reads or writes `agent_profiles.status`, so it neither depends on nor
  blocks it.
- **Non-Office tasks and sessions, and the workflow engine's own launches.** A
  paused workspace does not affect ordinary kanban execution there, and does not
  gate the workflow engine: a routine that materialises a real task leaves it on
  the board, so a later transition into an auto-start step launches an agent even
  while paused. REQ-OFFICE-KILL-SWITCH-002 blocks the fire that would create such
  a task and -003 cancels the execution running when pause is pressed; driving it
  onward afterwards takes a human or an API caller, which this capability leaves
  working.
- **Blocking human and API writes.** Creating tasks, commenting, editing routines
  and changing agent configuration all keep working while paused, by
  AC-OFFICE-KILL-SWITCH-002.6 and -002.8.
- **Retention, pruning, and a pause history endpoint.** Released records are
  retained with no expiry and readable in the database, and every create and
  release writes an activity log entry, but no endpoint returns past records and
  the state endpoint reports only the active one. No history ordering is
  specified, because nothing consumes one.
- **Live pause-state updates.** Pause state is read, never pushed, so a second
  operator's pause reaches an already-open page on its next read or through
  AC-OFFICE-KILL-SWITCH-006.13's refresh, not immediately. A workspace-scoped
  event was specified and cut: settling its ordering needs a sequencing primitive
  no Office event payload carries. The design records what a follow-up would need.

## Prior art

**Wiki (our own prior reasoning).** Searched: vault
`/Users/henry/Documents/henry/wiki`, QMD collection `wiki`, for kill switches,
emergency stop, pausing autonomous agent loops. **Did not run**: no `qmd` on
PATH, no `mcp__qmd__query`, every vault read returns `EPERM`. A tool
failure, not an empty result.

**saas-kb (what other products shipped).** Searched: `search_fsm_docs`,
`category: "ai_sdlc"`. **Did not run**: no `saas-kb` MCP server.

**This repository**, the only leg with evidence. `office_task_tree_holds` is this
shape one scope down: an authoritative hold record with release provenance, read
at a gate in run processing, cancelling in-flight executions and queued runs.
This capability is that pattern at workspace scope, departing three ways. (1) The
tree hold records no creation actor or reason, while REQ-004 requires create-side
provenance, because "who stopped this" is what an operator arrives asking.
(2) Office's other workspace-scoped boolean store returns false on any read
error, so a stop kept there silently stops applying; AC-002.9 fails closed.
(3) The tree hold cancels by task id over a subtree, while a workspace pause must
also cover runs with no task id, which lightweight routines produce, and tasks
with no run, which heavy ones do.
