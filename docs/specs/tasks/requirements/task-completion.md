---
status: draft
system: tasks
created: 2026-09-09
updated: 2026-09-28
owners:
  - kandev
---

# Task Completion Requirements

## Overview

Workflow authors control when work is complete. Users can continue a completed
task's conversation to ask follow-up questions. The task system owns both
contracts because it owns task state, workflow entry, and session identity.

Task completion and conversation availability are independent. A completed task
does not need to reopen merely because its agent answers a question.

## Requirements

### REQ-TASKS-COMPLETION-001: Explicit completion and evidence on step entry

**Intent:** Make task completion a visible, portable, verifiable workflow choice.

#### Acceptance criteria

- **AC-TASKS-COMPLETION-001.1:** Only the workflow's final step shall expose
  **Complete task when entering this step** on desktop and mobile. An adjacent
  info icon shall disclose the helper description on hover, keyboard focus,
  or touch. The description shall not appear as permanent text below the checkbox.
- **AC-TASKS-COMPLETION-001.2:** When a nonterminal task enters the final step
  with completion enabled, the task shall become `COMPLETED`, regardless of
  the step's name. Final position alone shall not complete a task.
- **AC-TASKS-COMPLETION-001.3:** Entering an unchecked step shall not complete
  the task. An unchecked final step shall permit further conversation. A
  non-final step shall not complete a task, even if its stored setting is enabled.
- **AC-TASKS-COMPLETION-001.4:** Renaming or reordering a step shall not change
  its stored completion setting. Only the current final step's setting shall
  apply. Reordering shall not transfer or enable another step's setting. New
  custom steps shall default to unchecked.
- **AC-TASKS-COMPLETION-001.5:** Upgrade shall enable the setting on existing
  final steps whose trimmed, case-insensitive names are Done, Complete,
  Completed, or Approved. Other existing steps shall remain unchecked.
- **AC-TASKS-COMPLETION-001.6:** After upgrade, disabling that setting shall
  survive restart and schema replay. Upgrade shall preserve task and session
  history and shall not synthesize completion notifications.
- **AC-TASKS-COMPLETION-001.7:** Create, update, reload, duplication, templates,
  workspace bootstrap, import/export, and workflow synchronization shall retain
  explicit true and false values. Omitted update fields shall preserve values.
- **AC-TASKS-COMPLETION-001.8:** Legacy portable workflows shall retain their
  previous completion behavior when imported or synchronized. Current exports
  shall state each step's completion setting explicitly.
- **AC-TASKS-COMPLETION-001.9:** Parent completion and successful dependency
  processing shall observe actual task completion. An unchecked final step
  shall not trigger either process through its name or position.
- **AC-TASKS-COMPLETION-001.10:** Repeated delivery of one completion transition
  shall not repeat parent actions or dependency launches. A chat follow-up shall
  not create another task-completion cycle.
- **AC-TASKS-COMPLETION-001.11:** Saving a step setting shall affect future
  entry. It shall not complete or reopen tasks already in that step. Explicit
  movement from a completed step to an unchecked step shall retain existing
  task-reopening behavior.
- **AC-TASKS-COMPLETION-001.12:** Manual, bulk, queued, automated, and initial
  task entry shall apply the same completion setting. Existing clarification,
  admission, cancellation, and transition-ledger rules shall remain effective.
- **AC-TASKS-COMPLETION-001.13:** The editor shall show saved and unsaved state,
  use coordinated Save changes and discard, and disable editing for read-only
  synchronized workflows. The phone control shall be reachable by touch.

#### Optional evidence gate amendment (September 27)

**Intent:** Make task completion evidence enforceable across every entry point.

- **AC-TASKS-COMPLETION-001.14:** A task shall support versioned optional
  completion criteria with evidence and verifier identity. Changing a criterion
  or its declared evidence subject shall invalidate affected verification.
  Removing or weakening an unmet criterion shall require explicit human
  confirmation.
- **AC-TASKS-COMPLETION-001.15:** An enabled completion gate shall block every
  transition into a completing workflow step until current criteria are
  verified, including manual, bulk, queued, agent, and automation paths.
- **AC-TASKS-COMPLETION-001.16:** A human shall be able to inspect blockers and
  explicitly override a gate with a recorded reason. Plugin absence shall
  leave a visible blocker and shall never require a synchronous plugin callback
  to evaluate completion.

### REQ-TASKS-COMPLETION-002: Follow-ups in completed conversations

**Intent:** Preserve conversation continuity after task or session completion.

#### Acceptance criteria

- **AC-TASKS-COMPLETION-002.1:** A `COMPLETED` session shall show **Resume**
  alongside **New Agent** in its existing chat on desktop and mobile.
- **AC-TASKS-COMPLETION-002.2:** Successful Resume shall restore the composer
  in the selected session. A follow-up shall reach the agent with that session's
  existing conversation context and appear in the same transcript.
- **AC-TASKS-COMPLETION-002.3:** Opening, reloading, or reconnecting to a
  completed chat shall not resume it or change its lifecycle state, regardless
  of the open-time auto-start preference.
- **AC-TASKS-COMPLETION-002.4:** Resume and a follow-up shall preserve task and
  session identity, history, workflow position, task state, agent ownership,
  primary-session selection, and valid provider conversation identity.
- **AC-TASKS-COMPLETION-002.5:** Root and child tasks shall both permit user
  follow-ups. Successful child completion without active clarification shall
  not permanently close its interactive conversation.
- **AC-TASKS-COMPLETION-002.6:** Explicit delivery to a completed session shall
  resume that exact session before accepting its turn. Task-addressed agent
  messages shall prefer an eligible live session over a completed sibling.
- **AC-TASKS-COMPLETION-002.7:** Resume shall preserve message order, queue
  identity, attachments, and Auto-run policy. Concurrent resume and send shall
  not launch duplicate runtimes or dispatch the same accepted prompt twice.
- **AC-TASKS-COMPLETION-002.8:** Late callbacks or cleanup from the completed
  execution shall not stop, complete, or clear the resumed execution's work.
- **AC-TASKS-COMPLETION-002.9:** A failed resume shall show actionable feedback
  without discarding history or unsent content. A missing profile or
  unrecoverable workspace shall not silently create a different conversation.
- **AC-TASKS-COMPLETION-002.10:** Active clarification ownership, answer
  delivery, and question barriers shall remain unchanged. Resume shall not
  reactivate historical questions or discard a current question.
- **AC-TASKS-COMPLETION-002.11:** FAILED and CANCELLED shall retain their
  existing recovery and message-delivery rules. The completed-session change
  shall not make ordinary sends revive those states.
- **AC-TASKS-COMPLETION-002.12:** A historical session retired by a profile
  switch shall remain excluded from automatic workflow reuse after manual
  Resume. Its follow-ups shall not advance the active workflow session.
- **AC-TASKS-COMPLETION-002.13:** Runtime cleanup shall retain recoverable
  conversation and workspace identity. Concurrent archive, delete, or explicit
  stop shall prevent a stale resume from resurrecting work.

### REQ-TASKS-COMPLETION-003: Workspace access and management claims

**Intent:** Let users inspect retained task work and govern its management
ownership without resuming its agent conversation. The task system owns
admission and recovery; workspace ownership remains governed by the canonical
task environment.

#### Acceptance criteria

- **AC-TASKS-COMPLETION-003.1:** When a completed session has a retained,
  accessible task workspace, opening the task shall restore file browsing,
  file contents, Git views, and supported workspace terminals without requiring
  Resume. This shall also work after its previous runtime has stopped.
- **AC-TASKS-COMPLETION-003.2:** Workspace restoration shall preserve session
  state, completion time, transcript, task state, workflow position, primary
  selection, and agent ownership. It shall not start an agent, dispatch a
  prompt, or replay workflow actions.
- **AC-TASKS-COMPLETION-003.3:** Explicit Resume shall remain available under
  `REQ-TASKS-COMPLETION-002` after workspace restoration. Restoration shall
  preserve the provider identity needed to continue the same conversation,
  including across another backend restart.
- **AC-TASKS-COMPLETION-003.4:** A FAILED or CANCELLED conversation with a
  retained, accessible workspace shall permit workspace-only restoration.
  Its existing agent recovery and message-admission rules shall remain unchanged.
- **AC-TASKS-COMPLETION-003.5:** Restoration shall enforce current access and
  ownership permissions. An archived or deleted task, an invalid session
  reference, or cleanup that has claimed the environment shall prevent stale
  restoration from recreating resources or exposing the workspace.
- **AC-TASKS-COMPLETION-003.6:** Concurrent workspace requests and explicit
  Resume shall not leave duplicate runtimes or stop a newer execution. A
  historical session sharing an environment shall not take over its live agent.
- **AC-TASKS-COMPLETION-003.7:** When retained workspace inventory is missing
  or unsafe, passive restoration shall report its unavailability. It shall not
  replace a branch, discard changes, or silently create a replacement workspace.
- **AC-TASKS-COMPLETION-003.8:** A workspace restoration failure shall appear
  within the affected workspace surface with a concise cause and expandable,
  bounded technical details. It shall not appear as a page-wide session-start
  failure, fail the conversation, or leave an indefinite preparation indicator.
- **AC-TASKS-COMPLETION-003.9:** A recoverable failure shall offer workspace
  Retry. An in-progress attempt shall prevent duplicate retries. Failure shall
  not trigger an unbounded retry loop; success shall clear matching feedback.
  A late result shall not replace feedback for a different environment or attempt.
- **AC-TASKS-COMPLETION-003.10:** Desktop and mobile shall expose the same
  workspace results and recovery actions. Phone controls shall have at least
  44-pixel touch targets. Expanded details shall remain within the workspace
  surface, preserve keyboard access, and cause no horizontal page overflow.

#### Management-claim amendment (September 27)

**Intent:** Prevent competing managers from changing the same delegated work.

- **AC-TASKS-COMPLETION-003.11:** The system shall support one optional
  management claim per task, separate from its worker assignee, with an
  installation, opaque instance key, and fencing generation.
- **AC-TASKS-COMPLETION-003.12:** Claim acquisition, release, and transfer
  shall compare observed task and claim versions. A conflicting plugin or an
  obsolete generation shall not perform a management mutation.
- **AC-TASKS-COMPLETION-003.13:** A human shall be able to inspect and transfer
  or release a claim, including when its plugin is disabled or uninstalled.
  The system shall audit the action and shall not silently steal a claim after
  a timeout.

### REQ-TASKS-COMPLETION-004: Plugin coordination without native task controls

**Intent:** Preserve ordinary task details while plugins use task-owned coordination contracts.

This amendment replaces the native task-detail presentation specified by
the original coordination plan. It does not remove management claims or gates.

#### Acceptance criteria

- **AC-TASKS-COMPLETION-004.1:** Desktop and phone task details shall omit the
  native Manager and Completion requirements controls, including their dialogs,
  drawers, loading states, and empty states. This applies with or without claims
  or criteria. The task page shall not request their detail endpoints on open.
- **AC-TASKS-COMPLETION-004.2:** Removing these controls shall leave no reserved
  toolbar space. The phone header, existing error feedback, content, composer,
  and bottom navigation shall remain visible and usable without overlap or
  horizontal page overflow. Existing desktop panels shall retain their layout.
- **AC-TASKS-COMPLETION-004.3:** Claims, criteria, evidence, history, version
  checks, and completion enforcement shall survive the presentation change.
  Tasks without criteria shall retain their completion behavior. Human recovery
  through existing authorized APIs shall remain available without a plugin.
- **AC-TASKS-COMPLETION-004.4:** Existing completion-move failures shall remain
  visible through normal task error feedback. Removing the controls shall not
  grant plugins human override authority or erase unmet criteria.

This amendment removes the built-in inspection and recovery UI. It does not
promise an equivalent plugin UI already exists. Existing authorized human APIs
continue to satisfy the recovery contracts in criteria 001.16 and 003.13.
Plugin presentation and any future native recovery entry require separate scope.

## Compatibility and exclusions

This contract replaces name-based workflow completion and the permanent
completed-session lock. It does not change manual task-state APIs, failed-child
rollups, successful-dependency semantics, Office scheduler ownership, archive
policy, or intentional New Agent behavior. Existing manual FAILED/CANCELLED
recovery is preserved; these states are not newly eligible for ordinary sends.

Automatic profile reuse remains governed by
[workflow profile sessions](workflow-profile-session-lifecycle.md). Prompt
admission during startup follows [resume prompt queue](resume-prompt-queue.md).
Provider restoration uses [agent recovery](../../agents/requirements/agent-resume-runtime-recovery.md).

Workspace restoration does not imply read-only access: existing edit and shell
permissions remain authoritative. This contract does not add workspace access
for tasks with no sessions, change archive/unarchive recovery, introduce a new
executor, or change physical workspace ownership.

## Implementation plans

- [Task completion and follow-ups](../../../plans/task-completion/plan.md)
- [Workspace restoration after completion](../../../plans/completed-workspace-restoration/plan.md)
- [Remove native coordination controls](../../../plans/remove-native-coordination-ui/plan.md)

## Coordination controls

The [coordination controls design](../system-design/coordination-controls.md)
specifies task management claims and the optional evidence gate. Tasks without
completion criteria retain their current behavior.
