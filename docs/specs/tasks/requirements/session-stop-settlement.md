---
status: active
system: tasks
created: 2026-10-20
owners:
  - kandev
---

# Session Stop Settlement

## Overview

Two stop paths could fail although nothing was left to stop. A
prompt-readiness resume refused to continue when another lifecycle path had
already finished tearing down the same execution, and a session stop on a
session whose runtime was already gone returned "no execution for session"
while its open turn stayed active, so task-wide idle checks such as
workspace-source attach kept reporting an active turn.

## Requirements

### REQ-TASKS-SESSION-STOP-SETTLEMENT-001: A stop with nothing left to stop settles

**Intent:** A stop or teardown request whose work is already done completes
instead of failing, without disturbing a launch that is still in flight.

**User story:** As a user stopping a session or resuming queued work, I want a
stop to succeed when the agent is already gone, so that the task is not left
with an open turn or a failed resume.

#### Acceptance criteria

- **AC-TASKS-SESSION-STOP-SETTLEMENT-001.1:** When a prompt-readiness resume
  finds that another lifecycle path has already completed teardown of the same
  execution, it shall not stop that execution again and shall continue the
  resume. A teardown owner that has not completed shall still fail the resume.
- **AC-TASKS-SESSION-STOP-SETTLEMENT-001.2:** When a session stop finds no live
  execution and the session is waiting for input or terminal, the stop shall
  succeed, close only the session's observed open turn, and keep the session
  row and state. A session in any other state shall keep the original
  not-found error and its turn, and a turn lookup or settlement failure shall
  be returned instead of a success.

## Compatibility and exclusions

No new state, setting, or retry. Stops of a live execution, forced cleanup
arbitration, and the synchronous cleanup variant are unchanged.

## Design and delivery

- [System design](../system-design/session-stop-settlement.md)
- [Delivery package](../../../plans/session-stop-settlement/plan.md)
