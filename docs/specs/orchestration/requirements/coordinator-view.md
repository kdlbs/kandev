---
status: draft
system: orchestration
created: 2026-09-17
owners:
  - Kandev
---

# Coordinator view requirements

## Overview

A person coordinating an existing Kanban workspace needs to see its work and
talk to the selected coordinator on one page. This extends the persistent
conversation and linked-task list in the local prototype. The user accepted the
task-overview-plus-chat direction in
[issue #3752](https://github.com/kdlbs/kandev/issues/3752#issuecomment-5713059573).
This is a design for implementation, not evidence of a shipped page.

**Observation** means displaying existing task state. It neither adopts a task
for an agent nor grants authority to mutate it. **Coordinated tasks** are tasks
already linked to the selected coordinator. **Loaded counts** summarize the
records successfully loaded for the current view, not unseen pages.

## Requirements

### REQ-ORCHESTRATION-COORDINATOR-VIEW-001: Workspace task overview

**Intent:** Make existing workspace work discoverable without requiring every
task to have been created or adopted by a coordinator.

#### Acceptance criteria

- **AC-ORCHESTRATION-COORDINATOR-VIEW-001.1:** Opening Coordinator shall show
  visible delivery tasks across the selected workspace's Kanban workflows,
  including tasks with no coordinator link. Hidden conversation/configuration
  tasks and archived tasks shall be excluded from the default view.
- **AC-ORCHESTRATION-COORDINATOR-VIEW-001.2:** Users shall be able to search,
  filter by workflow/repository, and switch between all workspace tasks and tasks
  linked to the selected coordinator. Changing a filter shall not adopt tasks,
  move them, change their profiles or dispatch work.
- **AC-ORCHESTRATION-COORDINATOR-VIEW-001.3:** Each row shall expose a task link,
  identifier/title, workflow step and status. Known execution identity, last
  activity, change-request links and diff counts shall be visible; unavailable
  data shall be identified as unknown, not represented as successful or zero.
- **AC-ORCHESTRATION-COORDINATOR-VIEW-001.4:** Tasks beyond the first page shall
  remain reachable. Group counts and PR/change summaries shall identify partial
  coverage while pages are unloaded or a refresh fails. Partial data shall not
  support a workspace-wide “all clear” statement.

### REQ-ORCHESTRATION-COORDINATOR-VIEW-002: Evidence-based task groups

**Intent:** Direct attention to actionable work without inventing worker intent.

#### Acceptance criteria

- **AC-ORCHESTRATION-COORDINATOR-VIEW-002.1:** Each visible task shall appear in
  exactly one primary group: Needs input, Problems, Running, Review, Queued,
  Done, or Other. A canonical pending question/permission in any relevant session
  shall take precedence over a running indicator. Additional signals shall
  remain inspectable on the row.
- **AC-ORCHESTRATION-COORDINATOR-VIEW-002.2:** A task with no pending request
  shall not be labeled Needs input solely because it is idle or has been quiet.
  Quiet duration shall not by itself establish a stall. Missing/stale signals
  shall remain distinguishable from healthy work.
- **AC-ORCHESTRATION-COORDINATOR-VIEW-002.3:** Finished agent turns, merged PRs
  and review states shall not independently make a task Done. Native task and
  workflow completion remain authoritative; the page shall not move tasks or
  bypass review/approval gates.

### REQ-ORCHESTRATION-COORDINATOR-VIEW-003: Persistent side conversation

**Intent:** Preserve a continuous workspace conversation while inspecting tasks.

#### Acceptance criteria

- **AC-ORCHESTRATION-COORDINATOR-VIEW-003.1:** Desktop shall show task overview
  and the selected coordinator conversation together, with a user control to
  hide/show chat. Returning or changing task filters shall reuse the same
  conversation and preserve its draft for that workspace/assignment.
- **AC-ORCHESTRATION-COORDINATOR-VIEW-003.2:** Multiple workspace coordinators
  shall remain selectable, with role/profile identity and configuration links.
  Switching selection shall load only that assignment's conversation and shall
  not transfer transcripts, drafts, ownership or task assignments.
- **AC-ORCHESTRATION-COORDINATOR-VIEW-003.3:** With no assignment or an invalid
  configuration, the page shall explain the required setup and link to it.
  Opening the page, loading task status or selecting an assignment shall not
  send a prompt, create a delivery task or start an agent.
- **AC-ORCHESTRATION-COORDINATOR-VIEW-003.4:** Sending, streaming, pause and
  recovery shall retain the existing coordinator semantics. A paused, disabled
  or unauthorized assignment shall not gain execution authority through this
  page. Scheduled delivery remains optional and explicit.

### REQ-ORCHESTRATION-COORDINATOR-VIEW-004: Current scoped observations

**Intent:** Keep the view consistent with native task pages and workspace scope.

#### Acceptance criteria

- **AC-ORCHESTRATION-COORDINATOR-VIEW-004.1:** Task/session/status changes shall
  refresh the visible groups and summaries without a full page reload. Older
  responses shall not overwrite newer known status. Reconnection shall reconcile
  with canonical state and identify stale data while reconciliation is pending.
- **AC-ORCHESTRATION-COORDINATOR-VIEW-004.2:** Switching workspace or losing
  access shall clear the previous workspace's rows, conversation and input
  before accepting new responses. Delayed responses shall not restore them.
- **AC-ORCHESTRATION-COORDINATOR-VIEW-004.3:** Loading/failure/empty states shall
  distinguish no matching work from data unavailable. Users shall be able to
  retry reads without replaying agent actions.

### REQ-ORCHESTRATION-COORDINATOR-VIEW-005: Mobile and accessible parity

**Intent:** Make the same task observations and conversation usable on phones.

#### Acceptance criteria

- **AC-ORCHESTRATION-COORDINATOR-VIEW-005.1:** At a 390-pixel-wide viewport,
  Tasks/Chat controls shall expose the same workspace and selected coordinator
  without clipped essential controls or page-level horizontal scrolling.
  Switching views shall preserve task filters, scroll context and the chat draft.
- **AC-ORCHESTRATION-COORDINATOR-VIEW-005.2:** Keyboard and assistive technology
  users shall be able to select the coordinator, change filters, expand groups,
  open tasks and use chat. Counts, active views, loading and errors shall have
  textual labels; status shall not rely on color alone.

### REQ-ORCHESTRATION-COORDINATOR-VIEW-006: Existing authority and privacy

**Intent:** Observation must not expand the coordinator's or viewer's access.

#### Acceptance criteria

- **AC-ORCHESTRATION-COORDINATOR-VIEW-006.1:** Task/conversation reads shall
  enforce current workspace and retained-owner access. Conversation tasks and
  transcripts shall not enter task rows, search results, counts or exports.
- **AC-ORCHESTRATION-COORDINATOR-VIEW-006.2:** A pending input or recovery link
  shall open the authoritative native task interface. This delivery shall not
  answer questions, grant permissions or resolve requests automatically.
- **AC-ORCHESTRATION-COORDINATOR-VIEW-006.3:** Disabling Orchestration shall
  remove/reject its entry points and new execution while retaining saved
  configuration and ownership protections. Existing Kanban/Office access and
  behavior shall retain their independent controls.

## Out of scope

New task execution or scheduling engines; a personal assistant attention ledger;
automatic task adoption, permission resolution or cross-workspace grants;
guaranteed read-only provider execution; typed report persistence; workflow-step
monitoring policy; plugin host API additions; dependency graphs, editable phase
plans, build buttons and replacement board/diff editors from the example images.
Existing native task/board/PR destinations remain available through links.

Published evidence shall use fictional records and generic prompts in disposable
fixtures. Private conversation history is not acceptance-test or demo material.
