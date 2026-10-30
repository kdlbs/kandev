---
status: active
system: tasks
created: 2026-10-30
owners:
  - kandev
---

# Async agent questions requirements

## Overview

An agent can ask a question early and continue independent work during the same turn.
It can read the answer at a checkpoint.
When no independent work remains, it can explicitly pause.
Tasks owns the request identity, lifecycle, recipient, and answer delivery.
The Needs-you Inbox consumes this state on desktop and phone.

This is the opt-in extension proposed in [issue #4836](https://github.com/kdlbs/kandev/issues/4836).
It does not change the active blocking clarification contract.

## Terminology

- **Request:** One bundle of one to four questions, with one durable `question_id`.
- **Item:** One question inside a request, identified by its local `questions[].id`.
- **Author:** The task session that creates a request.
- **Recipient:** The operator, or the author's direct parent task under Autopilot.
- **Forward:** An authorized parent exposes its child's existing request to the operator, preserving its identity.
- **Explicit wait:** The author declares that its remaining work depends on a named set of pending requests.

## Requirements

### REQ-TASKS-ASYNC-QUESTIONS-001: Ask and continue

**Intent:** Separate question creation from a decision to pause work.

#### Acceptance criteria

- **AC-TASKS-ASYNC-QUESTIONS-001.1:** With `async: true`, either question tool shall return a durable `question_id` without waiting for an answer.
  Creation shall not cancel the author's turn, change its running state, or start another turn.
- **AC-TASKS-ASYNC-QUESTIONS-001.2:** A request shall become discoverable immediately after creation.
  Operator requests shall appear in the enabled Inbox. Parent requests shall appear in the direct parent's question list and message queue.
  Discoverability shall not depend on the author's turn ending.
- **AC-TASKS-ASYNC-QUESTIONS-001.3:** A session shall support multiple independent pending requests.
  An exact creation retry shall return the original request without a second notification.
  Reusing an explicit retry identity with different content shall fail without a write.

### REQ-TASKS-ASYNC-QUESTIONS-002: Durable resolution

**Intent:** Preserve the request and one winning outcome across turns, retries, and restart.

#### Acceptance criteria

- **AC-TASKS-ASYNC-QUESTIONS-002.1:** An open async request shall remain answerable across ordinary turns and backend restart in its original input-capable session.
  A newer conversational turn alone shall not close it.
- **AC-TASKS-ASYNC-QUESTIONS-002.2:** Concurrent answer, rejection, and withdrawal attempts shall produce one terminal outcome.
  An exact winning retry shall return that outcome. A losing attempt shall expose the winner without another delivery.
- **AC-TASKS-ASYNC-QUESTIONS-002.3:** Explicit session cancellation, terminal session state, task archival, or deletion shall prevent further answer delivery.
  Invalidating the recorded direct-parent relationship shall make a parent request stale.
  No operation shall redirect an answer to another session or resurrect terminal work.
- **AC-TASKS-ASYNC-QUESTIONS-002.4:** Answers shall preserve item identity, selected option IDs and labels, free text, and responder provenance.
  Rejection shall remain distinct from an affirmative answer.

### REQ-TASKS-ASYNC-QUESTIONS-003: Read and deliver answers

**Intent:** Let an agent obtain answers during independent work without interrupting that work.

#### Acceptance criteria

- **AC-TASKS-ASYNC-QUESTIONS-003.1:** The author shall be able to list its request states and answers during a running turn.
  Reads shall be bounded, repeatable, and available after restart. A read shall not consume or close a request.
- **AC-TASKS-ASYNC-QUESTIONS-003.2:** An accepted answer or rejection shall also create a durable, correlated continuation notification for the original session.
  A running turn shall finish before this notification is dispatched.
  Acceptance shall not cancel, steer, or supersede the running turn.
- **AC-TASKS-ASYNC-QUESTIONS-003.3:** Restart or delivery retry shall not create duplicate queue admissions for the same outcome and destination.
  A transport-ambiguous dispatch shall obey existing queue recovery rules without a blind replay.
- **AC-TASKS-ASYNC-QUESTIONS-003.4:** A direct parent shall be able to list its direct children's pending requests during its own turn.
  This list shall not expose requests from unrelated tasks or bypass workspace access.

### REQ-TASKS-ASYNC-QUESTIONS-004: Withdraw a question

**Intent:** Remove a question that its author no longer needs answered.

#### Acceptance criteria

- **AC-TASKS-ASYNC-QUESTIONS-004.1:** The author shall be able to withdraw a pending request with a reason.
  Withdrawal shall close every item in that request as `withdrawn`, without recording an answer.
- **AC-TASKS-ASYNC-QUESTIONS-004.2:** Withdrawal shall remove the actionable request from the operator Inbox and parent list.
  History and transcript views shall retain the request, withdrawal reason, and outcome.
- **AC-TASKS-ASYNC-QUESTIONS-004.3:** A repeated withdrawal shall be idempotent.
  A later answer shall not reopen it. A withdrawal shall not replace an answer that already won.

### REQ-TASKS-ASYNC-QUESTIONS-005: Forward a child request

**Intent:** Let the responsible parent obtain an operator decision without losing the child's question identity.

#### Acceptance criteria

- **AC-TASKS-ASYNC-QUESTIONS-005.1:** A current direct parent with an operator-question capability shall be able to forward a pending child request.
  The operator shall see the child task, forwarding parent, original context, and the same request identity.
- **AC-TASKS-ASYNC-QUESTIONS-005.2:** An operator answer to a forwarded request shall resolve the original child request.
  The child shall receive the correlated outcome. The parent shall see that outcome without manually copying it.
- **AC-TASKS-ASYNC-QUESTIONS-005.3:** Parent and operator answers to a forwarded request shall compete for the same terminal outcome.
  Forward retries shall create no duplicate Inbox row.
- **AC-TASKS-ASYNC-QUESTIONS-005.4:** Forwarding shall preserve existing question capabilities and authorization.
  An Autopilot session shall not gain direct operator access. An unrelated parent or inaccessible workspace shall receive no question content.

### REQ-TASKS-ASYNC-QUESTIONS-006: Explicit dependency wait

**Intent:** Pause for questions only after the agent declares that independent work is complete.

#### Acceptance criteria

- **AC-TASKS-ASYNC-QUESTIONS-006.1:** Open async requests alone shall not establish a question wait or suppress ordinary turn completion.
  An explicit wait shall name pending requests owned by the current session.
- **AC-TASKS-ASYNC-QUESTIONS-006.2:** When all named dependencies remain pending, an explicit wait shall pause the captured author turn.
  It shall preserve those requests, withhold workflow completion, and settle as waiting for input.
- **AC-TASKS-ASYNC-QUESTIONS-006.3:** When at least one named dependency resolves before waiting begins, the call shall return a ready outcome without pausing.
  If resolution races the pause, the system shall preserve the outcome and resume only after the pause settles.
- **AC-TASKS-ASYNC-QUESTIONS-006.4:** Any terminal outcome of a named dependency shall release the wait once.
  Other pending requests shall remain open. A newer accepted prompt shall invalidate the old wait without cancelling that newer turn.

### REQ-TASKS-ASYNC-QUESTIONS-007: Inbox and conversation outcomes

**Intent:** Give operators one accessible place to answer async questions while agents continue work.

#### Acceptance criteria

- **AC-TASKS-ASYNC-QUESTIONS-007.1:** The enabled Inbox shall include open operator requests and explicitly forwarded child requests, grouped by originating task.
  Unforwarded parent requests shall remain absent. The actionable count shall count requests, not task groups.
- **AC-TASKS-ASYNC-QUESTIONS-007.2:** Async rows shall distinguish an agent that continues work from one that explicitly waits.
  Answering shall preserve the current page and shall not open a blocking overlay over a running author's composer.
- **AC-TASKS-ASYNC-QUESTIONS-007.3:** Desktop and phone shall support task grouping, answering, rejection, and readable withdrawal history.
  Phone controls shall remain touch-accessible, with one answer surface and no document horizontal overflow.
- **AC-TASKS-ASYNC-QUESTIONS-007.4:** Failed reads shall show an error instead of an empty success.
  Failed submissions shall retain drafts. A concurrent winner shall close the row and disclose its outcome.
- **AC-TASKS-ASYNC-QUESTIONS-007.5:** All new interface copy shall use the repository locale catalogs.
  Existing visibility, per-user hiding, workspace isolation, and stale-response protection shall apply to async rows.

### REQ-TASKS-ASYNC-QUESTIONS-008: Blocking compatibility

**Intent:** Preserve the existing contract for callers that do not choose async mode.

#### Acceptance criteria

- **AC-TASKS-ASYNC-QUESTIONS-008.1:** Omitting `async`, or setting it to `false`, shall preserve the existing tool schemas' other fields, responses, pauses, timeouts, and answer behavior.
  A non-boolean `async` value shall fail before creation.
- **AC-TASKS-ASYNC-QUESTIONS-008.2:** Blocking clarification activeness shall retain current-turn ownership.
  Async requests shall not enter the blocking waiter, detached-resume, or earlier-turn blocking-question paths.
- **AC-TASKS-ASYNC-QUESTIONS-008.3:** Session question-tool selection shall remain mutually exclusive.
  Autopilot roots and non-task profiles shall not gain async question creation through this feature.

## Related contracts

- [Blocking clarification lifecycle](clarification-active-lifecycle.md)
- [Blocking user-question turn boundary](user-question-turn-boundary.md)
- [Autopilot mode](autopilot-mode.md)
- [Needs-you Inbox presentation](../../ui/requirements/needs-you-inbox.md)
- [System design](../system-design/async-agent-questions.md)
- [Implementation plan](../../../plans/async-agent-questions/plan.md)

## Out of scope

- Reopening historical blocking tool requests or implementing issue #4820.
- Granting Autopilot roots direct operator access or adding multi-hop forwarding.
- Permission approvals, Office-specific question routing, and external MCP question creation.
- Automatic workflow movement on a question answer.
- Changes to runtime feature-toggle defaults or existing blocking-image behavior.
