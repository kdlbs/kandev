# ADR-2026-09-05-workflow-script-session-binding: Bind workflow scripts to the trigger-owning agent session

**Status:** accepted
**Date:** 2026-09-05
**Area:** backend, agentctl, frontend, protocol, security, workflow

## Context

Workflow scripts need a working directory, executor, environment, transcript,
and lifecycle owner. Those values can change at a step transition because the
destination step can choose another agent profile, reuse a parked session, or
create a new session.

Repository setup scripts are not a suitable execution path. Their runner is
host-local and tied to worktree preparation. A workflow step can run in Docker,
SSH, Kubernetes, or Sprites, and its output belongs in the agent transcript
rather than the preparation surface.

Scripts can also be non-idempotent. Replaying a workflow event after a backend
restart cannot safely imply that the command should run again.

## Decision

`run_script` is a session-bound workflow action.

- `on_enter` binds to the destination session after profile/session routing and
  before automatic prompting.
- `on_turn_complete` and `on_exit` bind to the source session that produced or
  owns the transition.
- The bound session's managed agentctl process runner executes the command in
  its execution workspace. The host-local repository script runner is not used.
- Command output and status are persisted as one `script_execution` message in
  the bound session.

Script admission is at most once for a trigger occurrence and action position.
A durable run record snapshots the command and policy before admission. The
runtime passes its run ID to agentctl as a stable process request identity.
Recovery reconciles a provably existing process. An ambiguous or lost process
becomes interrupted and is never automatically restarted.

Failure policy is part of each action. `continue` records the failure and lets
later actions proceed. `block` prevents a pending turn-complete or exit
transition. An entry transition is already committed, so an entry block stops
later session-bound entry work and automatic prompting without rolling the step
back.

## Consequences

- Profile switches place entry output in the destination agent tab and keep
  completion and exit output with the source agent's history.
- Every supported executor uses the same command path and workspace ownership.
- Workflow scripts can run before an agent subprocess starts, but only after
  the destination execution workspace exists.
- The workflow engine and orchestrator must carry stable trigger occurrence
  identities and script results through transition evaluation.
- SQLite and Postgres gain durable workflow script run state.
- Agentctl managed-process start gains a stable request identity.
- Recovery prefers a visible interrupted result over a possible duplicate
  side effect.
- Entry failures cannot provide transactional rollback for the committed step
  or for earlier session-independent actions.
- Anyone who can author a workflow can execute commands with the task
  executor's permissions. Imported and synchronized workflows require code
  review.

## Alternatives considered

- Run every script in the source session: rejected because entry scripts would
  prepare the wrong profile/session after a profile switch and would appear in
  the wrong transcript.
- Run every script in the destination session: rejected because completion and
  exit checks are causally part of the source agent's work and may need its
  executor identity.
- Run scripts on the backend host: rejected because remote/container workspaces
  and runtime environment are not necessarily available on the host.
- Reuse repository setup-script execution: rejected because it is preparation
  scoped, host-local, and filtered into a different user interface.
- Retry an uncertain run after restart: rejected because arbitrary commands can
  create commits, publish artifacts, or otherwise produce non-idempotent side
  effects.
- Store scripts as a new top-level step field: rejected because typed workflow
  actions already provide trigger ordering, portable serialization, and future
  extensibility.
