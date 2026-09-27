---
status: draft
system: platform
created: 2026-09-27
owners:
  - kandev
---

# Detached Agent Continuity Requirements

## Overview

A remote agent keeps running when the link between the Kandev backend and its
agentctl breaks. Examples are a VPN reset, a Wi-Fi change, or a laptop that
sleeps while the executor host stays up. Today Kandev treats that break as a
failed turn. It never reconnects. The agent's Kandev tool calls fail within
seconds, and the chat shows nothing.

This capability makes a link break a visible, recoverable condition:

- The agent keeps doing work that needs only its host.
- Anything that needs Kandev waits for Kandev.
- Kandev reconnects by itself.
- Nothing a person must decide is crossed while nobody can see it.

Platform owns this contract because it spans every executor and extends the
shared session recovery services. It builds on [durable agent
delivery](durable-agent-delivery.md), which owns the journal, cursor replay,
projection, and submission reconciliation. This document does not restate
those contracts. The [executor system](../../executors/README.md) owns each
transport. Here it only supplies the ability to re-establish one.

## Terminology

- **Link:** the backend's connection to one agentctl control stream, carried
  by the executor's transport.
- **Detached:** agentctl has no attached backend stream.
- **Disconnected:** the backend has lost the link to a live execution, and
  has no evidence yet that the agent stopped.
- **Redial:** the executor re-establishes its transport to the same surviving
  agentctl and supplies a new control client.
- **Kandev tool call:** a Kandev MCP tool call from the agent that needs the
  backend to answer, for example step completion, a user question, or a task
  plan read or write.
- **Offline budget:** the longest continuous detached period after which
  agentctl cancels the running turn.

## Requirements

### REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-001: A link break is not a failure

**Intent:** A broken link says nothing about the agent. Kandev must not
report failure, completion, or idleness for an agent that may still be
working.

**User story:** As a user running an agent on a remote executor, I want a
network drop to show as a disconnection, so that I do not act on a false
failure.

#### Acceptance criteria

- **AC-PLATFORM-DETACHED-AGENT-CONTINUITY-001.1:** When the link to an
  execution on a redial-capable executor breaks, the session shall show
  Disconnected within 5 seconds of the backend observing the break. It shall
  not show failed, completed, or waiting for input.
- **AC-PLATFORM-DETACHED-AGENT-CONTINUITY-001.2:** While a session is
  Disconnected, no elapsed time shall move the session to a terminal state,
  move its task to another workflow step or state, or start a replacement
  agent.
- **AC-PLATFORM-DETACHED-AGENT-CONTINUITY-001.3:** A session shall leave
  Disconnected only when:
  - the link is re-established;
  - a reconnect finds that the agent process no longer exists; or
  - the user stops the session.
- **AC-PLATFORM-DETACHED-AGENT-CONTINUITY-001.4:** When a reconnect finds that
  the agent process no longer exists, the session shall take the outcome that
  durable delivery reconciliation assigns. It shall not assign a separate
  outcome.
- **AC-PLATFORM-DETACHED-AGENT-CONTINUITY-001.5:** While a session is
  Disconnected, Stop shall remain available.
- **AC-PLATFORM-DETACHED-AGENT-CONTINUITY-001.6:** When the user stops a
  Disconnected session, the session shall show stopped. The next successful
  reconnect shall cancel the running turn and stop the agent before any other
  work is admitted.
- **AC-PLATFORM-DETACHED-AGENT-CONTINUITY-001.7:** When a reconnect finds that
  agentctl no longer exists, but an agent process it started for the session
  is still running on the host, Kandev shall stop that agent process and its
  children. It shall do this before it reports the session outcome, so that
  no untracked agent keeps changing the workspace.

### REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-002: Automatic reconnection

**Intent:** Recovery must not depend on a person noticing the drop.

**User story:** As a user, I want Kandev to reconnect to my running agent by
itself, so that a short network drop costs me nothing.

#### Acceptance criteria

- **AC-PLATFORM-DETACHED-AGENT-CONTINUITY-002.1:** When executor reachability
  changes to reachable for a host with a Disconnected session, Kandev shall
  start a reconnect attempt for that session within 10 seconds.
- **AC-PLATFORM-DETACHED-AGENT-CONTINUITY-002.2:** While a session remains
  Disconnected, Kandev shall keep attempting to reconnect:
  - at increasing intervals, starting at 5 seconds and capped at 5 minutes;
  - with no limit on the number of attempts.
- **AC-PLATFORM-DETACHED-AGENT-CONTINUITY-002.3:** When the user chooses
  Reconnect on a Disconnected session, Kandev shall start an attempt
  immediately. It shall not wait for the next scheduled interval.
- **AC-PLATFORM-DETACHED-AGENT-CONTINUITY-002.4:** When a reconnect succeeds,
  Kandev shall continue the running agent. It shall not initialize, create,
  load, or resume a harness session. It shall not resend a prompt.
- **AC-PLATFORM-DETACHED-AGENT-CONTINUITY-002.5:** When a reconnect succeeds,
  output the agent produced while detached shall appear once and in order,
  using durable delivery replay.
- **AC-PLATFORM-DETACHED-AGENT-CONTINUITY-002.6:** The SSH, remote Docker,
  Sprites, and Kubernetes executors shall be redial-capable. An executor that
  is not redial-capable shall keep its current disconnect behavior.

### REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-003: Kandev tool calls wait while detached

**Intent:** The agent must not treat a temporary outage as a permanent
failure. It must not route around a gate either.

**User story:** As a user, I want my agent's Kandev calls to wait for the
connection, so that it neither loops on retries nor completes a step I have
not seen.

#### Acceptance criteria

- **AC-PLATFORM-DETACHED-AGENT-CONTINUITY-003.1:** When the agent makes a
  Kandev tool call while detached, the call shall wait. It shall complete with
  the backend's answer after reattach. It shall not return an error before the
  offline budget expires.
- **AC-PLATFORM-DETACHED-AGENT-CONTINUITY-003.2:** While a Kandev tool call
  waits, agentctl shall send the agent MCP progress notifications at intervals
  of 20 seconds or less.
- **AC-PLATFORM-DETACHED-AGENT-CONTINUITY-003.3:** A Kandev tool call made
  while detached shall take effect only when the backend processes it after
  reattach. agentctl shall not complete a step, answer a question, or change a
  plan on the backend's behalf.
- **AC-PLATFORM-DETACHED-AGENT-CONTINUITY-003.4:** When the link breaks after
  a Kandev tool call reached the backend but before its answer arrived, the
  call shall fail. The error text shall state that the outcome is unknown, and
  that the agent must check state before retrying. agentctl shall not resend
  the call automatically.
- **AC-PLATFORM-DETACHED-AGENT-CONTINUITY-003.5:** While detached, a tool
  permission request shall remain pending until the user answers it after
  reattach. It shall not be approved or denied automatically.

### REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-004: Offline budget

**Intent:** An agent nobody can see or stop must not run without bound.

**User story:** As a user, I want an unreachable agent to pause after a known
period, so that a long outage cannot let it run away.

#### Acceptance criteria

- **AC-PLATFORM-DETACHED-AGENT-CONTINUITY-004.1:** The offline budget shall be
  15 minutes. An executor profile can set its own value, between 1 and 1440
  minutes.
- **AC-PLATFORM-DETACHED-AGENT-CONTINUITY-004.2:** When agentctl has been
  detached for the full offline budget and a turn is running, agentctl shall
  cancel that turn. Waiting Kandev tool calls shall return an error stating
  that the offline budget was reached.
- **AC-PLATFORM-DETACHED-AGENT-CONTINUITY-004.3:** When the offline budget
  cancels a turn, agentctl shall keep running and keep its journal. Kandev
  shall keep reconnecting. After reconnect, the session shall wait for the
  user's next instruction. It shall not start another turn by itself.
- **AC-PLATFORM-DETACHED-AGENT-CONTINUITY-004.4:** When a backend stream
  attaches, the offline budget shall restart from zero at the next detach.
- **AC-PLATFORM-DETACHED-AGENT-CONTINUITY-004.5:** agentctl shall not exit
  because no backend is attached before the offline budget has expired and the
  running turn has been cancelled.

### REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-005: Agent guidance

**Intent:** The agent must understand the condition instead of inventing a
polling loop.

#### Acceptance criteria

- **AC-PLATFORM-DETACHED-AGENT-CONTINUITY-005.1:** When a Kandev tool call
  fails because the offline budget was reached, its error text shall state
  that Kandev is unreachable. It shall tell the agent to stop and end its turn
  without polling or sleeping.
- **AC-PLATFORM-DETACHED-AGENT-CONTINUITY-005.2:** The Kandev system context
  given to the agent shall state that Kandev tool calls can wait during a
  connection loss. It shall state that they must not be retried in sleep
  loops.

### REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-006: Visible disconnection

**Intent:** The user must see what happened, what continues, and when the
agent will pause.

**User story:** As a user, I want the task to tell me it is disconnected and
what the agent is doing, so that I know whether to wait or act.

#### Acceptance criteria

- **AC-PLATFORM-DETACHED-AGENT-CONTINUITY-006.1:** While a session is
  Disconnected, the chat on desktop and phone shall show:
  - the executor host;
  - the time the link broke;
  - that the agent keeps working on the host;
  - the time the agent will pause if it is still disconnected;
  - Reconnect and Stop actions.
- **AC-PLATFORM-DETACHED-AGENT-CONTINUITY-006.2:** While a session is
  Disconnected, its task card and remote executor status shall indicate the
  disconnection.
- **AC-PLATFORM-DETACHED-AGENT-CONTINUITY-006.3:** When a reconnect succeeds,
  the disconnection notice shall clear within 5 seconds after replay
  completes. The conversation shall record that Kandev reconnected, and after
  how long.
- **AC-PLATFORM-DETACHED-AGENT-CONTINUITY-006.4:** When a turn ended while
  detached, the conversation shall record the time it ended. Workflow actions
  on turn completion shall apply once after replay, as they would have with
  the link up.
- **AC-PLATFORM-DETACHED-AGENT-CONTINUITY-006.5:** When the offline budget
  cancelled a turn, the conversation shall record the time it paused and
  state that it waits for the user's instruction.
- **AC-PLATFORM-DETACHED-AGENT-CONTINUITY-006.6:** While a session is
  Disconnected, a page reload shall show the same disconnection state.

## Out of scope

- The agent or agentctl opening a connection to the backend. Kandev keeps
  initiating every link. See the [decision
  record](../../../decisions/2026-09-27-backend-dialed-detached-agents.md).
- Queuing or replaying Kandev tool calls. Calls wait, or fail with a stated
  reason.
- Exactly-once execution of tools, MCP calls, or model calls. Durable
  delivery excludes this too.
- Reconnecting local and worktree executors, whose agentctl runs on the
  backend host.
- Keeping a Disconnected session reconnecting across a backend restart. The
  existing resume path applies after a restart.
- Durable delivery behavior that the durable delivery contract owns: the
  reconciliation window, recovery block resolution, the uncertain delivery
  state, and event forwarding while detached.
- Network or VPN configuration that causes the break.
