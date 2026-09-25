---
status: draft
system: tasks
created: 2026-09-16
owners:
  - kandev
---

# Queued session ownership requirements

## Overview

A workflow can select its next session before instance capacity permits launch.
Users can open earlier conversations with normal recovery behavior without
changing the selected workflow recipient or losing the visible queued state. The task system owns the selected recipient,
durable launch intent, and its presentation across task surfaces.

This capability extends [workflow session lifecycle](workflow-profile-session-lifecycle.md).
The [agent ceiling](../../agents/requirements/session-concurrency-ceiling.md)
continues to own capacity admission. A ceiling queue is distinct from a
[workflow WIP queue](wip-limit-pull-system.md): WIP admission occurs before
destination entry, whereas a ceiling can delay a session already selected by entry.

## Terminology

- **Passive inspection:** Opening, focusing, reloading, reconnecting, previewing,
  or selecting a conversation without requesting execution.
- **Parked predecessor:** A conversation stopped by a workflow session switch.
  It remains available for reading, explicit follow-up, or later workflow reuse.
- **Queued destination:** The exact session selected for a deferred workflow launch.
- **Explicit execution:** A user selects Start/Resume or sends a message to an
  identified conversation. A valid workflow re-entry is a separate authorized trigger.

## Requirements

### REQ-TASKS-QUEUED-SESSION-OWNERSHIP-001: Inspection preserves execution ownership

**Intent:** Opening a conversation resumes it without transferring workflow ownership.

#### Acceptance criteria

- **AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.1:** Superseded by criterion 001.9
  under [the session-open decision](../../../decisions/2026-09-18-session-open-resumes-conversation.md).
  The original prohibition on resuming workflow-stopped conversations no longer applies.
- **AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.2:** When a destination is queued,
  passive inspection of any conversation shall preserve its recipient, prompt,
  queue time, primary ownership, and workflow step. It shall not duplicate or
  replace that accepted launch. A different conversation can recover under 001.9.
- **AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.3:** Explicit execution for a parked
  predecessor shall target that conversation only. It shall not replace the
  queued destination, consume its prompt, or acquire its workflow ownership.
- **AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.4:** A later workflow entry that
  legitimately selects a parked conversation shall be able to reuse it under
  the existing lifecycle policy. Historical parking shall not permanently block reuse.
- **AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.5:** Passive recovery that remains
  eligible under existing preferences shall respect the automatic session
  ceiling. It shall never produce a manual-override audit message.
- **AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.6:** If pending launch ownership cannot be
  established, passive inspection shall remain available without starting work
  or attempting a fresh-session fallback. Explicit recovery shall remain separately available.

- **AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.7:** After a workflow legitimately
  reuses a conversation, historical workflow stops shall not prevent its otherwise
  eligible recovery after restart. Desktop and phone task opening shall honor
  the existing auto-start preference and automatic capacity limit. Pending launch ownership shall still prevent duplicate execution.
  Workflow parking shall not independently block recovery.
- **AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.8:** After a deferred launch settles
  and no pending launch remains, queue history shall not block otherwise eligible
  open-time recovery. Recovery shall preserve the conversation and shall not
  replay the settled workflow prompt.

- **AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.9:** Opening or selecting a
  workflow-stopped conversation shall use normal automatic recovery, including
  non-primary conversations. Recovery shall honor auto-start prevention, capacity,
  authorization, archive, and terminal-session rules. It shall preserve conversation
  context without sending a new prompt or transferring workflow ownership.

### REQ-TASKS-QUEUED-SESSION-OWNERSHIP-002: Deferred work survives sibling lifecycle events

**Intent:** A queued launch must remain owned and retryable until its own outcome is known.

#### Acceptance criteria

- **AC-TASKS-QUEUED-SESSION-OWNERSHIP-002.1:** While an eligible queued
  destination exists and no session is executing work, the task shall remain
  Scheduling. A sibling becoming ready, idle, stopped, failed, or completed
  shall not move that task to Review or clear its queue.
- **AC-TASKS-QUEUED-SESSION-OWNERSHIP-002.2:** If a sibling explicitly runs
  while the destination is queued, task activity shall reflect the running work
  and retain the queue indicator. When that sibling settles, the task shall
  return to Scheduling while the destination remains queued.
- **AC-TASKS-QUEUED-SESSION-OWNERSHIP-002.3:** When capacity becomes available,
  automatic retry shall deliver the accepted launch to its exact eligible
  destination once. It shall not wake or send the step prompt to the predecessor.
- **AC-TASKS-QUEUED-SESSION-OWNERSHIP-002.4:** Reload and backend restart shall
  preserve queued ownership and parked inspection behavior. Repeated refusal
  shall not reset queue time, replace the accepted prompt, or duplicate sessions.
- **AC-TASKS-QUEUED-SESSION-OWNERSHIP-002.5:** If the destination is removed,
  cancelled, or superseded by a later workflow entry, its obsolete launch shall
  not run or retarget another session. The task shall expose the disposition.
  A transient read or replay error shall preserve accepted work for retry.
- **AC-TASKS-QUEUED-SESSION-OWNERSHIP-002.6:** A stale replay or lifecycle event
  shall not clear, launch, or reconcile state for a newer queued entry. Explicit
  task cancellation and archive shall retain their existing precedence.

### REQ-TASKS-QUEUED-SESSION-OWNERSHIP-003: Visible launch queue

**Intent:** Users can distinguish waiting for capacity from a stopped or failed agent.

#### Acceptance criteria

- **AC-TASKS-QUEUED-SESSION-OWNERSHIP-003.1:** Desktop sidebar rows and the
  phone task navigator shall expose a labelled Queued indicator for deferred
  session launches, without requiring the queued conversation to be selected.
- **AC-TASKS-QUEUED-SESSION-OWNERSHIP-003.2:** Task details shall show the
  queued destination, waiting reason, queue time, capacity observation, and
  automatic retry explanation outside the historical transcript. This status
  shall remain visible when the user selects a parked predecessor.
- **AC-TASKS-QUEUED-SESSION-OWNERSHIP-003.3:** Superseded by criterion 003.10.
  The original requirement to identify a conversation as parked no longer applies.
- **AC-TASKS-QUEUED-SESSION-OWNERSHIP-003.4:** Queue status shall clear after
  confirmed dispatch or final disposition. Older snapshots shall not restore a
  cleared queue or erase newer queued work. Pending questions and real errors
  shall remain visible alongside queue status.
- **AC-TASKS-QUEUED-SESSION-OWNERSHIP-003.5:** Capacity observations shall show
  when they were checked. When unavailable or disconnected, the interface
  shall retain known queued ownership and identify stale or unavailable counts.
  It shall not claim a queue position or an estimated start time.
- **AC-TASKS-QUEUED-SESSION-OWNERSHIP-003.6:** Phone users shall inspect the
  same status and navigate conversations through the existing task drawer and
  session picker. Required information shall not depend on hover. New touch
  actions shall have at least 44 px hit targets, with no page horizontal overflow.
- **AC-TASKS-QUEUED-SESSION-OWNERSHIP-003.7:** Passive inspection and ordinary
  queue waiting shall not generate an empty-output completion warning. A real
  prompt that completes without output shall retain its existing warning behavior.
- **AC-TASKS-QUEUED-SESSION-OWNERSHIP-003.8:** When a launch waits for session
  capacity, its banner shall name the Global session limit and state that it
  applies across all workspaces. It shall offer a link to the Session capacity
  section in Settings. The link shall work on desktop and phone without starting
  or resuming a session. Settings permissions and environment locks still apply.
- **AC-TASKS-QUEUED-SESSION-OWNERSHIP-003.9:** The banner shall distinguish global
  session capacity from workflow WIP, ownership errors, and replay errors. A
  stale or unavailable capacity count shall not hide the known limit scope.
  After a limit change, the banner shall show refreshed state without a page
  reload and remain until dispatch or final disposition is confirmed.

- **AC-TASKS-QUEUED-SESSION-OWNERSHIP-003.10:** Desktop and phone conversation
  surfaces shall not show a parked-session banner, toolbar, badge, or recovery
  explanation. Normal conversation controls shall remain available. Genuine queued
  destinations shall retain queue status without a duplicate Start or Resume action.

## Compatibility and exclusions

Actual manual starts and resumes retain the existing ceiling override. This
package does not turn the ceiling into a hard limit for explicit execution.
Ordinary open-time recovery remains preference-controlled, subject to the
ownership and automatic-admission conditions above.

No new session-state enum, global queue dashboard, queue reordering, ETA,
capacity setting, workflow lifecycle setting, or provider-routing policy is added.
Office scheduling, WIP admission order, terminal conversation recovery, and
unrelated prompt queues retain their owning contracts.

## Design

- [Queued session ownership](../system-design/queued-session-ownership.md)
