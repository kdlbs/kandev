---
status: active
system: tasks
created: 2026-10-20
owners:
  - kandev
---


# Unanswered clarification inbox Requirements



## Overview



An opt-in workspace setting keeps questions an agent moved past without an answer visible as
unanswered, so they can still be answered, closed, or found again without reviving the original tool
request.



## Why

The active lifecycle ([requirements](clarification-active-lifecycle.md)) ties every question to the
turn that asked it. When a coordinating agent starts its next turn soon after asking, the question
leaves the Needs-you inbox and `list_pending_questions_kandev` while still unanswered. Users then
find questions only by opening each task's transcript, and the agent cannot see that it already
asked, so it asks again. The active rules stay correct; what is missing is a separate, non-blocking
view of the questions nobody answered.

## What

- A workspace setting, `clarification_unanswered_inbox_enabled`, is off by default. With it off,
  every behavior in this document is absent and Kandev behaves exactly as before.
- `clarification_unanswered_retention_days` (1 to 90, default 14) bounds how long an unanswered
  question stays listed.
- A question is **unanswered** when its bundle is still `pending`, a newer turn of its own session
  superseded it, it is not an autopilot parent-question record, every row has a resolvable
  `question_id`, its task is not archived, and nothing has closed or merged it. A bundle that the
  terminal-session rule expired stays expired.
- Unanswered questions never become active. They do not drive the chat overlay, the task-row
  pending projection, workflow guards, or `answer_question_kandev`.
- Closing writes `metadata.closed_reason` (and `closed_at`) on that bundle's rows only; status and
  turn ownership are unchanged. `answered_late` also records `late_answered_at`,
  `late_answer_message_id`, and `late_answer_session_id`.

## Requirements



### REQ-TASKS-CLARIFICATION-UNANSWERED-001: Unanswered clarification inbox



**Intent:** Questions an agent stopped waiting on stay answerable as ordinary messages, never as
tool responses, while their workspace opts in.



#### Acceptance criteria



- **AC-TASKS-CLARIFICATION-UNANSWERED-001.1:** When the workspace setting is on and a newer turn
  supersedes a pending question, the Needs-you inbox shall list it in a separate "unanswered (agent
  not waiting)" section with answer, close, and open-task actions. When the setting is off, the
  section, its endpoint data, duplicate merging, and retention shall not exist, and every existing
  surface shall behave as before.
- **AC-TASKS-CLARIFICATION-UNANSWERED-001.2:** The unanswered list shall exclude active questions,
  questions expired by the terminal-session rule, autopilot parent-question records, bundles with an
  unresolvable `question_id`, questions of archived tasks (they return when the task is
  unarchived), and closed or merged questions. It shall be workspace-scoped with the Needs-you
  inbox's authorization and the caller's own dismiss and snooze rows.
- **AC-TASKS-CLARIFICATION-UNANSWERED-001.3:** Answering an unanswered question shall send an
  ordinary late-answer message under AC-TASKS-CLARIFICATION-LIFECYCLE-001.5 to .8. It shall target
  the question's original session by default.
- **AC-TASKS-CLARIFICATION-UNANSWERED-001.4:** When a late-answer message for an unanswered question
  is admitted, Kandev shall close exactly that question with `closed_reason=answered_late`,
  `late_answered_at`, and `late_answer_message_id`. It shall not reopen the tool request, restore a
  workflow barrier, or resolve or close any other question. The caller must have prompt access to
  the target session, and an existing user-message admission must match that task, session, and
  message identity. If admission cannot be established, the question stays unanswered.
- **AC-TASKS-CLARIFICATION-UNANSWERED-001.5:** When the question's original session has ended and
  its task has a different primary session, the user shall be able to choose that session as the
  late answer's target. The choice shall be explicit, absent while the original session is live, and
  rejected for a session of another task.
- **AC-TASKS-CLARIFICATION-UNANSWERED-001.6:** When a new question in the same task repeats an
  unanswered one, Kandev shall keep the new question active and mark the older one
  `merged_into=<new pending_id>`, hiding it from unanswered lists while keeping it in history. Two
  questions repeat when every question of the older bundle has the same agent-chosen `question_id`
  or the same prompt after trimming, collapsing whitespace, and case folding. Positional default
  identifiers such as `q1` never prove a repeat on their own.
- **AC-TASKS-CLARIFICATION-UNANSWERED-001.7:** Closing an unanswered question requires task write
  access and shall record
  `closed_reason=closed_by_user` without sending a message or resuming the agent. An unanswered
  question older than the workspace retention shall close as `expired_unanswered` before any
  unanswered read returns it.
- **AC-TASKS-CLARIFICATION-UNANSWERED-001.8:** `list_pending_questions_kandev` shall accept an
  optional `include_unanswered` argument, default false. When true, each bundle carries `state`
  (`active` or `unanswered`) in one cursor sequence, and an automation caller also sees its own
  task's unanswered questions. Without it, the response is unchanged. `answer_question_kandev`
  shall continue to answer active questions only.
- **AC-TASKS-CLARIFICATION-UNANSWERED-001.9:** Settings, Workspace shall expose the toggle and the
  retention in days, saved as a partial workspace update that keeps omitted stored values, rejected
  outside 1 to 90, and carried by boot state and workspace events.

## Out of scope



The unified control-room inbox, coordinator prompt wording, and any change to the active
clarification rules belong to their own systems and changes.
