---
status: draft
system: tasks
created: 2026-10-12
owners:
  - kandev
---

# Primary Session Ownership Requirements

## Overview

Each task has one primary session. Status projections, task-level prompts, and
the default conversation use it. The task system owns this contract because it
stores task sessions and their primary flag. Workflow steps choose their
recipients under [workflow session lifecycle](workflow-profile-session-lifecycle.md);
this document covers how the flag itself changes and how Kandev replaces a
primary session that stopped or was deleted.

## Terminology

- **Promotion:** Making one session primary and demoting every other session of
  the task in one operation.
- **Automatic promotion:** A promotion that Kandev starts when the primary
  session stops or is deleted.
- **Parked session:** A nonterminal session stopped by a workflow profile switch.

## Requirements

### REQ-TASKS-PRIMARY-SESSION-OWNERSHIP-001: One primary session that the workflow can use

**Intent:** The primary flag changes only through a promotion, and an automatic
promotion prefers a non-parked session and never picks a finished session.

**User story:** As a workflow user, I want the task's primary conversation to
stay the one the workflow selected, so that task prompts and status do not
return to an earlier conversation.

#### Acceptance criteria

- **AC-TASKS-PRIMARY-SESSION-OWNERSHIP-001.1:** When a session update does not
  request a promotion, an ordinary full-row write shall preserve the stored
  primary flag. A snapshot-guarded rollback may release only its still-owned
  selection.
  A session copy read before a promotion shall not make an earlier session
  primary again, and the task shall keep exactly one primary session.
- **AC-TASKS-PRIMARY-SESSION-OWNERSHIP-001.2:** When the primary session stops
  or is deleted, an automatic promotion shall prefer a nonterminal session that
  uses the current step's effective profile and shall rank a parked session
  last. Within a rank, running, then other active, then idle sessions win.
- **AC-TASKS-PRIMARY-SESSION-OWNERSHIP-001.3:** An automatic promotion shall
  never select a terminal session. After a stop, it shall still require an
  active session. After a deletion, it may select any nonterminal session. When
  only sessions without a matching profile remain, one of them shall still
  become primary.
- **AC-TASKS-PRIMARY-SESSION-OWNERSHIP-001.4:** An automatic promotion shall
  not move queued messages between sessions or change an explicit session
  selection.

## Out of scope

- Moving queued messages to the new primary session. Ordinary Send keeps its
  session target under the
  [plan comment decision](../../../decisions/2026-09-02-task-owned-plan-comments.md).
- Workflow recipient selection, which
  [workflow session lifecycle](workflow-profile-session-lifecycle.md) owns.
- A database uniqueness constraint on the primary flag.

## System design

- [Primary session ownership](../system-design/primary-session-ownership.md)
