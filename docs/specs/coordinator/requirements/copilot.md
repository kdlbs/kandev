---
id: coordinator-copilot
title: Coordinator copilot and tool surface
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-26
last_updated: 2026-09-28
---

# Coordinator copilot and tool surface Requirements

## Overview

A manager talks to a coordinator through a chat panel on the right side of
the Coordinator screens, laid out like the board's task preview panel. The conversation is an ordinary Kandev session on an ephemeral task.
Phase 1 is attended: only a manager's message starts a turn. The coordinator's
Kandev tools can read the workspace and propose tasks, and nothing else.

Kandev enforces the coordinator's Kandev surface. It does not control the agent
CLI's own tools (a shell on its executor, its own MCP servers, its own
permission settings); in phase 1 a coordinator is as capable through those as
any Kandev chat on the same profile, and no more.

## Terminology

- **Conversation task:** the ephemeral task with origin `coordinator` that
  holds one coordinator's conversation.
- **Turn:** one agent run in response to one message.
- **Coordinator surface:** the set of Kandev MCP tools registered for a
  conversation task's session.
- Other terms are defined in the [system README](../README.md#terms).

## Mockup

The phase 1 mockup screenshots are the visual reference for this document's
user-facing criteria; each requirement below cites the ones it covers. Where
a screenshot and an acceptance criterion differ, the criterion governs. The
prototype banner, the demo controls and the `P1` and `WC-` labels are mockup
chrome, not product; the data is seeded fiction.

The mockup draws the copilot as a floating popover. The copilot is a
right-side panel instead (maintainer direction on PR #3981, 2026-09-28), so
the screenshots are the reference for the copilot's content (header,
transcript, context chip, composer and proposal card), not for its frame, size
or position; [REQ-COORDINATOR-COPILOT-004](#req-coordinator-copilot-004-copilot-panel)
governs those.

- [`docs/plans/workspace-coordinator/assets/p1-02-ask-about-this.png`](../../../plans/workspace-coordinator/assets/p1-02-ask-about-this.png)
- [`docs/plans/workspace-coordinator/assets/p1-05-chat-create-task-proposal.png`](../../../plans/workspace-coordinator/assets/p1-05-chat-create-task-proposal.png)

## Requirements

### REQ-COORDINATOR-COPILOT-001: Conversation

**Intent:** Each coordinator keeps one persistent conversation.

**User story:** As a workspace manager, I want to ask the coordinator why
something needs me, so that I can decide without reading the board.

Mockup:

- [`docs/plans/workspace-coordinator/assets/p1-05-chat-create-task-proposal.png`](../../../plans/workspace-coordinator/assets/p1-05-chat-create-task-proposal.png): a conversation in the panel: the user message with its about tag and a Kandev read tool call.

#### Acceptance criteria

- **AC-COORDINATOR-COPILOT-001.1:** When a manager opens a coordinator's
  conversation for the first time, the system shall create one ephemeral
  conversation task with origin `coordinator`, using the coordinator's agent
  profile and executor, and return its `task_id`, `session_id` and archive
  state.
- **AC-COORDINATOR-COPILOT-001.2:** When the conversation is opened again, the
  system shall return the same task; when two opens race, both shall return the
  same task and exactly one conversation task shall exist.
- **AC-COORDINATOR-COPILOT-001.3:** When the conversation task no longer exists,
  the next open shall create a new one.
- **AC-COORDINATOR-COPILOT-001.4:** When a conversation starts, the agent shall
  receive the coordinator's standing instructions: its job, the workspace, its
  context text and that every write is a proposal. After a saved context
  change, the next open shall return a new conversation task carrying the new
  context; the previous conversation task shall be archived, is never returned
  by an open again, and is deleted with the coordinator or the workspace.
- **AC-COORDINATOR-COPILOT-001.5:** The conversation task shall never appear on a
  board, in a task list, in Needs you or Queue, or as a Quick Chat tab.
- **AC-COORDINATOR-COPILOT-001.6:** When a coordinator is deleted, its current
  conversation task and its archived conversation tasks shall be deleted.
- **AC-COORDINATOR-COPILOT-001.7:** An open shall return a conversation task
  whose session exists and has no agent running unless a manager's message
  started one; the open itself shall never start an agent.
- **AC-COORDINATOR-COPILOT-001.8:** A conversation task shall not be removed by
  Quick Chat idle expiry, however long it stays idle.
- **AC-COORDINATOR-COPILOT-001.9:** When an open loses the race described in
  `AC-COORDINATOR-COPILOT-001.2` and finds no other task to converge on
  because a context or profile change cleared the reference concurrently, the
  open shall return 409; the panel's next open retries with the fresh
  value. The same applies when a context or profile change archives the task
  an open is about to return, whether that task was reused or just created,
  between the open reading it as current and the open's own response: the
  open shall return 409 rather than a task that is actually archived.
- **AC-COORDINATOR-COPILOT-001.10:** When the current conversation task's
  session has ended (failed, cancelled or completed), so it can no longer
  accept a message, the next open shall archive that task and return a new
  conversation task, as after a context change; the open itself still starts
  no agent.

### REQ-COORDINATOR-COPILOT-002: Attended turns

**Intent:** Nothing but a manager's message makes the coordinator act.

#### Acceptance criteria

- **AC-COORDINATOR-COPILOT-002.1:** The only way to start a coordinator turn
  shall be a message a manager sends to its conversation task.
- **AC-COORDINATOR-COPILOT-002.2:** Opening, closing or reloading the copilot,
  creating the conversation, a stall event, a workspace deletion event, a
  proposal decision, startup recovery and session recovery after a restart
  shall not send a message to, resume or start the conversation session.
- **AC-COORDINATOR-COPILOT-002.3:** When a reader sends a message to a
  conversation task, the system shall refuse it and start nothing.
- **AC-COORDINATOR-COPILOT-002.4:** After a reload, the launcher shall show
  the session's current state, and when the panel is opened the transcript
  shall reappear with that state: a turn still running shows as running, an
  idle session shows as idle. The
  reload shall send no resume or restore request; an idle agent starts again
  only when the manager sends a message.

### REQ-COORDINATOR-COPILOT-003: Kandev tool surface

**Intent:** Through Kandev, a coordinator can only read and propose.

Mockup:

- [`docs/plans/workspace-coordinator/assets/p1-05-chat-create-task-proposal.png`](../../../plans/workspace-coordinator/assets/p1-05-chat-create-task-proposal.png): the coordinator's one write, a task proposal pending approval.

#### Acceptance criteria

- **AC-COORDINATOR-COPILOT-003.1:** A coordinator session shall have exactly
  these six Kandev tools: `list_tasks_kandev`, `get_task_conversation_kandev`,
  `list_workflows_kandev`, `list_workflow_steps_kandev`,
  `list_repositories_kandev` and `propose_task_kandev`. It shall have no other
  Kandev tool, including no `list_related_tasks_kandev`, no plan read, no
  user-question tool, no task-title tool and no plugin tool.
- **AC-COORDINATOR-COPILOT-003.2:** When a coordinator session calls any other
  Kandev tool or action, the system shall refuse it with an error naming the
  tool and change nothing.
- **AC-COORDINATOR-COPILOT-003.3:** When a coordinator session names a
  workspace other than its coordinator's (including a `workspace_id` argument
  of `list_workflows_kandev` or `list_repositories_kandev`), or a task,
  workflow, step or repository of another workspace, the system shall refuse
  the call and return nothing from that workspace.
- **AC-COORDINATOR-COPILOT-003.4:** When a session that is not a coordinator
  session sends the proposal action, the system shall refuse it.
- **AC-COORDINATOR-COPILOT-003.5:** A session is a coordinator session only when
  its task has origin `coordinator` and is the named coordinator's current
  conversation task; task creation through HTTP or MCP shall not be able to set
  origin `coordinator`.
- **AC-COORDINATOR-COPILOT-003.6:** When a coordinator session would start while
  the flag is off, its coordinator does not exist, its task cannot be read, its
  agent profile is missing or CLI-passthrough, or its executor profile is
  missing, the session shall not start and the
  system shall report an error; it shall never start with the regular task
  tools.
- **AC-COORDINATOR-COPILOT-003.7:** In a coordinator session, Kandev shall
  auto-approve permission requests only for the exact tools of
  `AC-COORDINATOR-COPILOT-003.1`, and shall not auto-approve any other tool of
  any MCP server named `kandev`.
- **AC-COORDINATOR-COPILOT-003.8:** A coordinator session shall ignore the agent
  profile's auto-approve setting and any agentctl auto-approve environment
  setting, on first launch, on a re-created or resumed execution and on a
  workspace-only execution that is later promoted.
- **AC-COORDINATOR-COPILOT-003.9:** Permission requests the agent raises for its
  own tools shall appear in the copilot with Approve and Deny, as in any Kandev
  chat.

### REQ-COORDINATOR-COPILOT-004: Copilot panel

**Intent:** The manager asks beside the list, the way a board card is
previewed beside the board.

Mockup:

- [`docs/plans/workspace-coordinator/assets/p1-02-ask-about-this.png`](../../../plans/workspace-coordinator/assets/p1-02-ask-about-this.png): the copilot's header, transcript and composer over Needs you (drawn as a popover; the panel frame is set by the criteria below).

#### Acceptance criteria

- **AC-COORDINATOR-COPILOT-004.1:** On the Coordinator screens, managers shall
  see a launcher at the bottom right while the panel is closed. It opens a
  panel on the right side of the content area, the full height of the content
  area, titled "Coordinator: <name>" with Close and no Expand or maximize
  action. While the panel is open the launcher is hidden. Readers shall see no
  launcher and shall have no way to open the panel.
- **AC-COORDINATOR-COPILOT-004.2:** While the conversation's agent is running,
  the launcher shall show a busy state when the panel is closed, and the panel
  header shall show a busy state when it is open.
- **AC-COORDINATOR-COPILOT-004.3:** The panel shall show the transcript and a
  composer without a mode or model selector.
- **AC-COORDINATOR-COPILOT-004.4:** When the conversation is empty, the panel
  shall show "Ask why something is on the list. I read the same facts the list
  is derived from; the only change I can make is to propose a task, which you
  approve." and one "Try asking" suggestion that fills the composer without
  sending.
- **AC-COORDINATOR-COPILOT-004.5:** When a turn is running and the panel
  closes, the turn shall continue and the launcher shall show it is busy; the
  composer's Stop shall end the turn.
- **AC-COORDINATOR-COPILOT-004.6:** When the session cannot start, or has
  ended, the panel shall show Kandev's session recovery feedback with an
  action that opens the conversation again (which returns a fresh session under
  `AC-COORDINATOR-COPILOT-001.10`), and the lists shall keep working. This
  state is the copilot's own; the task page, mobile and Settings chat recovery
  are unchanged.
- **AC-COORDINATOR-COPILOT-004.7:** Escape pressed while focus is inside the
  panel shall close the panel and return focus to the launcher. When the panel
  floats (`AC-COORDINATOR-COPILOT-004.8`), a click on the backdrop shall also
  close it and return focus to the launcher.
- **AC-COORDINATOR-COPILOT-004.8:** The panel shall lay out as the board's
  task preview panel does. When the list keeps at least half of the content
  area's width beside the panel, the panel sits beside the list and the list
  narrows to the remaining width, so the panel covers no part of the list;
  otherwise the panel floats over the list from the right edge above a
  backdrop, and the list keeps its width. At a 1440px-wide viewport with the
  sidebar expanded and the default panel width, the panel sits beside the list
  and covers no item action. At the mobile breakpoint (390px included) the
  panel fills the screen width and height with no resize handle and no
  horizontal scroll.
- **AC-COORDINATOR-COPILOT-004.9:** Configuration chat on `/settings` shall
  behave as before, Expand included.
- **AC-COORDINATOR-COPILOT-004.10:** While a turn is running, the composer
  shall not send a message; it offers Stop instead.
- **AC-COORDINATOR-COPILOT-004.11:** Outside the mobile breakpoint, dragging
  the panel's left edge shall resize it. The first opening in a browser uses
  500px; the width shall never be narrower than 320px (380px at a coarse
  pointer) nor wider than 95% of the viewport width. The chosen width shall
  persist in that browser across reloads and coordinators, separately from the
  board preview panel's width.
- **AC-COORDINATOR-COPILOT-004.12:** The panel, its chip and its composer draft
  shall stay as they are when the manager switches between the same
  coordinator's Needs you and Queue. Leaving that coordinator's screens (to
  another coordinator or any other page) shall close the panel and clear the
  chip and the draft. A reload shall leave the panel closed.
- **AC-COORDINATOR-COPILOT-004.13:** The board's task preview panel shall
  behave as before: the same layout rule, resize bounds, persisted width,
  Escape and backdrop close, and maximize action.

### REQ-COORDINATOR-COPILOT-006: Activity display

**Intent:** The manager sees that the coordinator is working, not how Kandev
runs it.

This requirement changes mockup `p1-05`, which shows each Kandev tool call as
its own row. It follows the pattern of assistant chats aimed at non-developers:
calm by default, detail on demand.

#### Acceptance criteria

- **AC-COORDINATOR-COPILOT-006.1:** While a turn is running, the panel shall
  show one status line above the composer that updates in place with a plain
  verb for the current tool (for example "Reading tasks", "Checking
  workflows", "Drafting a proposal") and the elapsed seconds. Tool calls shall
  not render as one row each while the turn runs.
- **AC-COORDINATOR-COPILOT-006.2:** When a turn ends, its tool calls shall
  collapse into one chip naming how many were made and how long the turn took,
  collapsed by default; expanding it shows one row per tool call with its
  existing detail.
- **AC-COORDINATOR-COPILOT-006.3:** A `propose_task_kandev` call shall never be
  collapsed: its proposal card always renders in full.
- **AC-COORDINATOR-COPILOT-006.4:** Once the agent has started successfully, the
  session start-up rows (environment preparation and agent start) shall be
  hidden; while the agent is still starting, or when it failed to start, they
  shall stay visible.
- **AC-COORDINATOR-COPILOT-006.5:** The activity display applies to the
  coordinator panel only; Settings configuration chat, Quick Chat and the
  task page shall render as before.

### REQ-COORDINATOR-COPILOT-005: Ask about this

**Intent:** A question starts from the item it is about.

Mockup:

- [`docs/plans/workspace-coordinator/assets/p1-02-ask-about-this.png`](../../../plans/workspace-coordinator/assets/p1-02-ask-about-this.png): Ask about this: the chip, the drafted question and the stored-form hint.

#### Acceptance criteria

- **AC-COORDINATOR-COPILOT-005.1:** When a manager chooses **Ask about this** on
  an item, the panel shall open with a removable context chip naming the item
  and "Why is <id> here?" in the composer, focused and not sent; `<id>` is the
  card identifier, or the proposal title for a proposal without a source task.
- **AC-COORDINATOR-COPILOT-005.2:** While the chip is set, each message sent
  shall be stored with the prefix "About <id>: ", and the composer shall show
  `your message is sent as "About <id>: ..."`.
- **AC-COORDINATOR-COPILOT-005.3:** In the coordinator's transcript, a message
  beginning "About <id>: " shall render as the text without the prefix plus an
  "about <id>" tag; the stored message shall keep the prefix.
- **AC-COORDINATOR-COPILOT-005.4:** When the chip is removed, the next message
  shall be sent without a prefix.
- **AC-COORDINATOR-COPILOT-005.5:** Choosing **Ask about this** on another item
  shall replace the chip and replace the composer text with that item's
  question.

## Out of scope

- The launcher on other pages, the settings header switch, Expand and a Quick
  Chat tab of kind `coordinator` (phase 3, decision D11).
- Context ids on the wire (phase 3, decision D12); phase 1 sends text only.
- Waking the coordinator on events, schedules or backstops (phase 4, gate G4).
- Enforced containment of the agent CLI's own tools: a gate G4 condition before
  any unattended turn. Phase 1 states the residual instead.
- `ask_user_question` for coordinators: coordinators ask through proposals.
