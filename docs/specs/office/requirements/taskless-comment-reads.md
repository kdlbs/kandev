---
status: draft
system: office
created: 2026-09-27
owners:
  - Kandev
---

# Office: Taskless Run Comment Reads Requirements

## Overview

This extends [Agent comment reads](agent-comment-reads.md) to taskless run
callers, and records refused agent reads on the caller's run. It was split out
because the base document had reached its size limit; its terminology, window,
ordering, byte budget and projection apply here unchanged.

The defect it answers is issue #3976: the coordinator heartbeat, a taskless run,
was refused on every comment read and still completed as a clean run.

## Terminology

- **Taskless run caller:** An agent caller whose validated JWT carries a caller
  task that is empty after trimming, a non-empty run identifier, and a workspace
  claim that is non-empty after trimming. This is the token a lightweight
  routine fire, such as the coordinator heartbeat, runs with. A token missing
  the run identifier or the workspace claim is not a taskless run caller.

## Requirements

### REQ-OFFICE-AGENT-COMMENT-READS-009: A taskless run reads its own workspace

**Intent:** A lightweight routine fire has no task, so the read relation has
nothing to relate the target to and denies every read. The same run may already
list the whole board and post a comment on any task in its workspace
(REQ-OFFICE-COORDINATOR-AUTHORITY-001 and -004). A coordinator that can flag a
blocker but cannot read the thread it is flagging spends every heartbeat on
refused reads. This requirement gives a taskless run the same workspace reach
for reads that it has for annotation, and records every refused read so the
run does not look clean when it is not.

**User story:** As a workspace owner, I want the coordinator heartbeat to read
the comments on the tasks it monitors, and I want to see it on the run when an
agent read was refused.

#### Acceptance criteria

- **AC-OFFICE-AGENT-COMMENT-READS-009.1:** When a taskless run caller requests
  a target task whose workspace is the caller's workspace claim, the endpoint
  shall return that task's comments under the same window, ordering, byte
  budget, and agent projection as REQ-OFFICE-AGENT-COMMENT-READS-002 to -005,
  whether or not the target is related to any task.
- **AC-OFFICE-AGENT-COMMENT-READS-009.2:** When a taskless run caller requests
  a target task in another workspace, or one that does not exist, the endpoint
  shall return the forbidden status and message of
  AC-OFFICE-AGENT-COMMENT-READS-001.12, identical between the two cases.
- **AC-OFFICE-AGENT-COMMENT-READS-009.3:** When the target task lookup itself
  fails, the endpoint shall return an error status, not the forbidden status,
  so an outage is not reported as a refusal.
- **AC-OFFICE-AGENT-COMMENT-READS-009.4:** An agent caller whose JWT carries a
  non-empty caller task shall keep the read relation of
  REQ-OFFICE-AGENT-COMMENT-READS-001 unchanged, and shall not gain workspace
  reach from this requirement.
- **AC-OFFICE-AGENT-COMMENT-READS-009.5:** When the endpoint refuses an agent
  caller whose JWT carries a non-empty run identifier, the system shall append
  exactly one `runtime.denied` event at level `warn` to that run, with
  `action=read_comments`, `target_type=task`, the requested target identifier,
  the caller agent and session identifiers, and the refusal message. This
  applies to taskless and task-bound callers alike.
- **AC-OFFICE-AGENT-COMMENT-READS-009.6:** A refused read from a caller with no
  run identifier shall append no run event. An accepted read and a dependency
  or storage error shall append no `runtime.denied` event.
- **AC-OFFICE-AGENT-COMMENT-READS-009.7:** Appending the refusal event shall
  not change the response status or body, and a failure to append it shall not
  change them either. The refusal shall not change the run's status or outcome.
- **AC-OFFICE-AGENT-COMMENT-READS-009.8:** The coordinator heartbeat
  instructions shall direct the agent to read a task's comments with
  `agentctl kandev comment list --task <task-id>` before concluding the task is
  stalled.

## Out of scope

- Changing a run's status or outcome because it was refused.
- Persisting a taskless run's transcript.
- Any change to task-bound callers' read relation.

## System design

[Agent comment reads](../system-design/agent-comment-reads-01.md), section
"Taskless run callers".
