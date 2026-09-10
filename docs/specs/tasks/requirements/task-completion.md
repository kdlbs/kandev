---
status: draft
system: tasks
created: 2026-09-09
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

### REQ-TASKS-COMPLETION-001: Explicit completion on step entry

**Intent:** Make task completion a visible, portable workflow choice.

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
