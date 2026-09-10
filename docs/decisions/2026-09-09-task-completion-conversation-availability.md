# ADR-2026-09-09-task-completion-conversation-availability: Separate task completion from conversation availability

**Status:** accepted
**Date:** 2026-09-09
**Area:** workflow

## Context

Task completion currently depends on a final step's name. Child terminal
receipts can also complete a session, after which the composer and backend
resume path refuse follow-ups. These decisions conflate workflow outcome,
resource lifetime, and conversation access.

The requested product contract makes task completion explicit and permits
manual continuation of completed chats. Historical profile sessions also need
this access without taking over the active workflow.

## Decision

Use `complete_task_on_enter` as the authoritative step setting. Preserve legacy
definitions through one-time database backfill and version-1 portable conversion.
Version-2 exports require explicit settings. Only the final step is eligible
to complete work, and its setting must be enabled. Names have no completion
authority; final position alone does not complete work.

The editor shows the checkbox only on the final step, with its explanation
behind an adjacent info icon. Reordering retains each step's stored choice,
but a non-final step's choice is inactive. The new final step uses its own value.

Task completion is durable workflow state. A chat follow-up does not reopen the
task or replay workflow actions. Explicit moves and task-state operations keep
their existing authority. Parent and dependency processing use persisted task
outcomes, not inferred step membership.

Keep completed sessions closed to automatic start and reuse. Permit explicit
Resume or a pinned follow-up through a guarded lifecycle transition that retains
conversation identity. Retired profile conversations remain outside workflow
ownership after manual resume. Runtime cleanup preserves the data needed to
recover the same conversation.

## Consequences

Users can ask follow-up questions without creating another conversation.
Workflow authors can rename steps without changing task completion. Reordering
changes final-step eligibility without transferring stored completion choices.
The migration and portable version boundary preserve existing definitions while
making false an unambiguous saved choice.

Resume must coordinate stale callbacks, cleanup, queue admission, and ownership.
The change does not broaden FAILED/CANCELLED message admission or bypass Office
scheduling, archive, credentials, or workspace-recovery constraints.

This amends the historical-endpoint language in the profile-session design:
completion ends automatic workflow reuse, but permits explicit conversation work.
Public workflow examples that reopen Done on any message must be updated when
the implementation ships.

## Alternatives considered

- Add only a Resume button: insufficient because backend terminal guards reject
  it and cleanup may have removed the executor record.
- Remove COMPLETED from every terminal-state helper: rejected because late
  events and automatic starts would gain authority to revive old executions.
- Keep all child runtimes alive: rejected because chat continuity does not
  require a permanent process or provider reservation.
- Infer completion from names as a fallback forever: rejected because renaming
  a step or saving false would remain ambiguous.
- Reopen tasks automatically on follow-up: rejected because the requested
  conversation continuation must preserve workflow state and notifications.

## Specifications

- [Task completion requirements](../specs/tasks/requirements/task-completion.md)
- [Task completion system design](../specs/tasks/system-design/task-completion.md)
