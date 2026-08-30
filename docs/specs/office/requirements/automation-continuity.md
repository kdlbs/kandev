---
status: active
system: office
created: 2026-08-23
owners:
  - Kandev
---

# Automation Continuity Requirements

## Overview

By default, automation firings produce hidden work that people read through the
automation run surface. The separate [automation target mode
requirements](automation-target-modes.md) let a workspace owner opt into
visible normal tasks. For either target, the owner can choose whether each
firing gets an isolated task or continues one conversation and task
environment. The system must keep each firing addressable, preserve the
complete shared transcript, and expose one backend-owned coordinator authority
only to hidden automation agents.

## Terminology

- **New-task policy:** `new_task`, which creates an isolated hidden task,
  primary session, task environment, and conversation for each firing.
- **Reusable policy:** `reuse_thread`, which sends later firings as new turns
  to one hidden task and primary session.
- **Automation run:** The durable record for one firing. A run is distinct
  from the task and is bound to the exact session turn that accepted it.
- **Hidden automation target:** The default target described by this document.
  Its generated task is not ordinary Kanban work.
- **Coordinator authority:** The backend-resolved `SurfaceAutomation` MCP
  profile and its trusted automation principal.

## Requirements

### REQ-OFFICE-AUTOMATION-CONTINUITY-001: Choose context between runs

**Intent:** Let a workspace owner choose independent hidden automation work or
a single continuing hidden conversation without exposing hidden automation
tasks as ordinary Kanban work.

#### Acceptance criteria

- **AC-OFFICE-AUTOMATION-CONTINUITY-001.1:** When an automation is created or
  edited, the settings form shall show a visible Context between runs choice
  with `new_task` and `reuse_thread` options.
- **AC-OFFICE-AUTOMATION-CONTINUITY-001.2:** When no policy is selected, the
  system shall use `new_task`.
- **AC-OFFICE-AUTOMATION-CONTINUITY-001.3:** When the automation uses the
  hidden target and `new_task` is selected, each firing shall create a separate
  conversation, files, and hidden task, and that task shall not appear in
  Kanban or the sidebar.
- **AC-OFFICE-AUTOMATION-CONTINUITY-001.4:** When `reuse_thread` is selected,
  the system shall limit the automation to one open run at a time.
- **AC-OFFICE-AUTOMATION-CONTINUITY-001.5:** When the hidden target is
  selected, the form shall state that new-task runs use separate conversation
  and files and that those tasks do not appear in Kanban or the sidebar.
- **AC-OFFICE-AUTOMATION-CONTINUITY-001.6:** Both policy descriptions shall
  remain visible and accessible on desktop and mobile, and each choice shall
  provide a touch target of at least 44 pixels.

### REQ-OFFICE-AUTOMATION-CONTINUITY-002: Preserve and focus shared turns

**Intent:** Make a reusable automation a readable conversation while keeping
each scheduled firing independently selectable and controllable.

#### Acceptance criteria

- **AC-OFFICE-AUTOMATION-CONTINUITY-002.1:** When a firing is admitted, its run
  shall record the exact task, session, and turn identities before the turn is
  dispatched.
- **AC-OFFICE-AUTOMATION-CONTINUITY-002.2:** When two runs in either target
  mode share a session, selecting either run shall keep the complete session
  transcript mounted and shall scroll or focus the selected turn instead of
  removing other turns.
- **AC-OFFICE-AUTOMATION-CONTINUITY-002.3:** When a person replies from a
  selected run in either target mode, the new turn and its messages shall
  remain visible in the complete transcript even when its turn identity differs
  from the selected scheduled turn.
- **AC-OFFICE-AUTOMATION-CONTINUITY-002.4:** When native session continuation
  is unavailable, fallback resume context shall use the newest 50 non-empty
  user or assistant text messages in chronological order. Tool calls, tool
  results, status events, and unknown entries shall not consume that limit.
- **AC-OFFICE-AUTOMATION-CONTINUITY-002.5:** Reusing a checkout shall not reset
  or rebase it. Replacement shall create a fresh task environment from the
  configured repository base branch.

### REQ-OFFICE-AUTOMATION-CONTINUITY-003: Recover admitted runs

**Intent:** Ensure an admitted firing cannot permanently consume a concurrency
  slot or be confused with another firing.

#### Acceptance criteria

- **AC-OFFICE-AUTOMATION-CONTINUITY-003.1:** The service shall persist an
  admitted run before publishing the dispatch event.
- **AC-OFFICE-AUTOMATION-CONTINUITY-003.2:** If dispatch publication fails,
  the admitted run shall become failed and `last_triggered_at` shall remain
  unchanged. The next firing shall be eligible for admission.
- **AC-OFFICE-AUTOMATION-CONTINUITY-003.3:** A completion or stop action shall
  identify the exact run, session, and turn and shall change no other shared
  run.
- **AC-OFFICE-AUTOMATION-CONTINUITY-003.4:** Startup and dispatch recovery
  shall fail an open bound run only when its exact turn has no live execution,
  open turn, or pending blocker. A live or blocked turn shall remain open.
- **AC-OFFICE-AUTOMATION-CONTINUITY-003.5:** A replacement task shall receive
  the rendered run title, while a resumed shared task shall keep its existing
  title. Exported YAML shall include only the continuation policy and shall
  exclude runtime pointers, bindings, titles, and MCP authority.

### REQ-OFFICE-AUTOMATION-CONTINUITY-004: Constrain coordinator authority

**Intent:** Give hidden automation agents enough workspace coordination access
while preventing owner-wide access, self-targeting, and task-local questions.

#### Acceptance criteria

- **AC-OFFICE-AUTOMATION-CONTINUITY-004.1:** Every hidden automation task
  session shall receive the fixed `SurfaceAutomation` profile resolved by the
  backend before MCP dispatch. A visible normal task shall receive its normal
  task profile instead.
- **AC-OFFICE-AUTOMATION-CONTINUITY-004.2:** The fixed catalog shall include
  workspace inventory, read-only launch catalog, task/session inspection,
  task coordination, and blocker coordination tools, and shall exclude
  `ask_user_question_kandev`, permanent task deletion, configuration writes,
  task-local authoring, diagnostics, plugin tools, and provider PR/MR tools.
- **AC-OFFICE-AUTOMATION-CONTINUITY-004.3:** The trusted principal shall carry
  the automation, workspace, caller task, caller session, and surface
  identities. Handlers shall use it as the workspace boundary and audit the
  source as `automation_mcp`.
- **AC-OFFICE-AUTOMATION-CONTINUITY-004.4:** Mutations, messages, stopping,
  spawning, and blocker discovery or resolution through `SurfaceAutomation`
  shall reject the automation's own hidden task and every session on it, as
  well as foreign workspaces.
- **AC-OFFICE-AUTOMATION-CONTINUITY-004.5:** A task spawned on another task
  shall receive that target task's normal profile and shall not inherit
  `SurfaceAutomation` from its caller.

## Out of scope

- Concurrent turns or additional sessions on a reusable automation.
- Per-automation MCP capability settings or arbitrary tool allowlists.
- Cross-workspace coordinator authority or self-approval.
- Automatic reset or rebase of a reused worktree.
