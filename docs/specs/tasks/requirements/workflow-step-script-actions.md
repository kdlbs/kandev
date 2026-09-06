---
status: draft
system: tasks
created: 2026-09-05
updated: 2026-09-06
owners:
  - Kandev
---

# Workflow Step Script Actions Requirements

## Overview

Workflow authors need a deterministic action that runs a shell command in the
same task environment as the agent assigned to a workflow boundary. The command
must not consume an LLM turn. Its command, live output, final status, and exit
code must remain visible in the bound agent session's chat history.

This capability adds `run_script` to step entry, turn completion, and step exit.
It also improves the existing inline workflow step editor with compact tabs and
focused action editing, without introducing a second workflow-editing layout.
It does not replace repository setup and cleanup scripts.

## Requirements

### REQ-TASKS-WORKFLOW-STEP-SCRIPT-001: Portable action contract

**Intent:** Let workflow authors configure deterministic commands on the
workflow boundaries where an agent session has a defined owner.

#### Acceptance criteria

- **AC-TASKS-WORKFLOW-STEP-SCRIPT-001.1:** A workflow step shall accept one or
  more ordered `run_script` actions under `on_enter`, `on_turn_complete`, and
  `on_exit`.
- **AC-TASKS-WORKFLOW-STEP-SCRIPT-001.2:** Each action shall store a non-empty
  `command`, an optional integer `timeout_seconds`, and an optional
  `failure_policy` whose values are `block` and `continue`.
- **AC-TASKS-WORKFLOW-STEP-SCRIPT-001.3:** An omitted timeout shall resolve to
  600 seconds. An omitted failure policy shall resolve to `block`. Explicit
  timeout values shall be between 1 second and 86,400 seconds, inclusive.
- **AC-TASKS-WORKFLOW-STEP-SCRIPT-001.4:** Workflow create, update, duplicate,
  export, import, and repository sync shall preserve the action and its config
  without installation-specific identifiers.
- **AC-TASKS-WORKFLOW-STEP-SCRIPT-001.5:** Invalid commands, timeout values, or
  failure policies shall produce a validation error instead of becoming an
  inert action or silently using a different value.
- **AC-TASKS-WORKFLOW-STEP-SCRIPT-001.6:** Existing workflow definitions that do
  not declare `run_script` shall retain their current behavior.

### REQ-TASKS-WORKFLOW-STEP-SCRIPT-002: Session and workspace ownership

**Intent:** Run each command where the agent for that workflow boundary sees
the task, including transitions that change agent profiles.

#### Acceptance criteria

- **AC-TASKS-WORKFLOW-STEP-SCRIPT-002.1:** A script shall run through agentctl
  in the bound task session's execution environment, with the execution
  workspace path as its working directory. It shall not run through the
  backend host's repository setup-script runner.
- **AC-TASKS-WORKFLOW-STEP-SCRIPT-002.2:** An `on_enter` script shall bind to
  the destination session after the destination profile and session lifecycle
  policy have been resolved and its execution workspace is ready.
- **AC-TASKS-WORKFLOW-STEP-SCRIPT-002.3:** An `on_turn_complete` or `on_exit`
  script shall bind to the source session that produced the completion or is
  leaving the step. A later destination profile switch shall not move either
  script into the destination session.
- **AC-TASKS-WORKFLOW-STEP-SCRIPT-002.4:** Consecutive steps that retain the
  same effective profile and session shall run their scripts in that reused
  session. A destination policy that selects a new or parked profile session
  shall bind entry output only to the selected destination session.
- **AC-TASKS-WORKFLOW-STEP-SCRIPT-002.5:** Local, Docker, SSH, Kubernetes, and
  Sprites execution shall use the same runtime command contract when the
  executor supports managed processes.
- **AC-TASKS-WORKFLOW-STEP-SCRIPT-002.6:** A missing, terminal, or unavailable
  bound session or execution workspace shall be a script failure governed by
  the action's failure policy.
- **AC-TASKS-WORKFLOW-STEP-SCRIPT-002.7:** The command shall receive the same
  managed process environment available to other agentctl workspace commands.
  The workflow action shall not add a second secret-expansion or template
  language.

### REQ-TASKS-WORKFLOW-STEP-SCRIPT-003: Ordering and failure behavior

**Intent:** Make script effects predictable relative to agent turns and step
transitions.

#### Acceptance criteria

- **AC-TASKS-WORKFLOW-STEP-SCRIPT-003.1:** Scripts on the same trigger shall run
  sequentially in declared order. The system shall wait for one terminal result
  before it starts the next script.
- **AC-TASKS-WORKFLOW-STEP-SCRIPT-003.2:** An entry script shall finish after
  profile routing and entry session configuration but before an automatic step
  prompt or implicit profile-switch prompt starts an agent turn.
- **AC-TASKS-WORKFLOW-STEP-SCRIPT-003.3:** A turn-complete script shall finish
  after the source agent turn and before any transition selected by that
  trigger commits.
- **AC-TASKS-WORKFLOW-STEP-SCRIPT-003.4:** An exit script shall finish before
  the source step transition commits and before source-session retirement or
  destination-session routing.
- **AC-TASKS-WORKFLOW-STEP-SCRIPT-003.5:** On success, later actions shall
  continue normally. On a failure with `continue`, later actions and the
  transition shall continue after the failed result is recorded.
- **AC-TASKS-WORKFLOW-STEP-SCRIPT-003.6:** On a turn-complete or exit failure
  with `block`, later trigger actions and the transition shall stop, and the
  source session shall remain available for user recovery.
- **AC-TASKS-WORKFLOW-STEP-SCRIPT-003.7:** On an entry failure with `block`, the
  already committed destination step shall not roll back. Remaining
  session-bound entry actions and automatic prompting shall stop, and the
  destination session shall remain available for user recovery.
- **AC-TASKS-WORKFLOW-STEP-SCRIPT-003.8:** Existing session-independent
  `on_enter` actions shall keep their durable step-entry dispatch semantics.
  A session-bound script shall not cause one of those actions to execute twice
  or roll back a completed side effect.
- **AC-TASKS-WORKFLOW-STEP-SCRIPT-003.9:** A timeout, process-start rejection,
  output-persistence failure, or non-zero exit code shall be a failed result.
  Timeout handling shall stop the complete process group before the trigger can
  continue.

### REQ-TASKS-WORKFLOW-STEP-SCRIPT-004: Durable at-most-once execution

**Intent:** Avoid repeating non-idempotent commands when events are delivered
again or a service restarts.

#### Acceptance criteria

- **AC-TASKS-WORKFLOW-STEP-SCRIPT-004.1:** Before process admission, the system
  shall persist one script-run record keyed by the trigger occurrence and
  action position, including a snapshot of the command, timeout, failure
  policy, workflow step, bound session, and execution identity.
- **AC-TASKS-WORKFLOW-STEP-SCRIPT-004.2:** Replaying the same trigger occurrence
  shall reuse the existing run record and terminal result. It shall not start a
  second process.
- **AC-TASKS-WORKFLOW-STEP-SCRIPT-004.3:** The process-start boundary shall be
  at most once. The system shall durably record that admission was attempted
  before calling agentctl and shall use a stable process request identity.
- **AC-TASKS-WORKFLOW-STEP-SCRIPT-004.4:** After a backend restart, a run with a
  known live process shall be reconciled from agentctl and its message shall
  continue toward a terminal result without starting a replacement.
- **AC-TASKS-WORKFLOW-STEP-SCRIPT-004.5:** If the system cannot prove whether an
  admitted process completed, it shall mark the run `interrupted`, preserve the
  available output, and apply the configured failure policy. It shall not
  automatically rerun the command.
- **AC-TASKS-WORKFLOW-STEP-SCRIPT-004.6:** Editing or deleting the workflow
  action after admission shall not change the running command or its outcome.
- **AC-TASKS-WORKFLOW-STEP-SCRIPT-004.7:** Task, session, or execution shutdown
  shall request termination of a live script process and persist a terminal or
  interrupted result. Loss of an HTTP or WebSocket request context alone shall
  not cancel an admitted script.

### REQ-TASKS-WORKFLOW-STEP-SCRIPT-005: Chat history and streaming

**Intent:** Make deterministic workflow work as visible and auditable as
repository and environment scripts.

#### Acceptance criteria

- **AC-TASKS-WORKFLOW-STEP-SCRIPT-005.1:** Before process admission, the system
  shall create one persistent `script_execution` message in the bound session.
- **AC-TASKS-WORKFLOW-STEP-SCRIPT-005.2:** The message shall show the command,
  workflow step, trigger, failure policy, timeout, live combined stdout and
  stderr, run status, duration, final exit code when available, and a clear
  timeout or interruption reason.
- **AC-TASKS-WORKFLOW-STEP-SCRIPT-005.3:** Output shall stream into the existing
  message through normal task-message updates. Reloading the task shall show
  the same message and the latest persisted output and status.
- **AC-TASKS-WORKFLOW-STEP-SCRIPT-005.4:** Workflow script messages shall remain
  in chronological agent chat. They shall not be filtered into the environment
  preparation progress surface.
- **AC-TASKS-WORKFLOW-STEP-SCRIPT-005.5:** Stored output shall be UTF-8 safe and
  bounded to the agentctl managed-process buffer limit. When earlier output is
  dropped, the message shall record and display that truncation.
- **AC-TASKS-WORKFLOW-STEP-SCRIPT-005.6:** Output persistence shall coalesce
  rapid chunks so verbose commands do not produce an unbounded rate of database
  writes or WebSocket events.
- **AC-TASKS-WORKFLOW-STEP-SCRIPT-005.7:** Script messages shall not count as
  user prompts, agent replies, completed turns, or workflow completion signals.

### REQ-TASKS-WORKFLOW-STEP-SCRIPT-006: Workflow script action authoring

**Intent:** Let workflow authors configure script actions without adding more
always-visible fields to an already dense step form.

#### Acceptance criteria

- **AC-TASKS-WORKFLOW-STEP-SCRIPT-006.1:** Each editable step shall expose
  `run_script` in the compatible action palette for entry, turn completion,
  and exit. A user shall be able to add, remove, reorder, and edit multiple
  scripts under each trigger.
- **AC-TASKS-WORKFLOW-STEP-SCRIPT-006.2:** A focused script action editor shall
  expose the command, timeout, and failure policy, explain when the selected
  trigger runs, and show inline validation before Save can persist the draft.
- **AC-TASKS-WORKFLOW-STEP-SCRIPT-006.3:** Read-only or synchronized workflow
  restrictions shall apply to script actions through the existing workflow
  mutation guard.
- **AC-TASKS-WORKFLOW-STEP-SCRIPT-006.4:** Desktop and mobile shall support the
  same create, edit, reorder, delete, save, and chat-inspection outcomes.
- **AC-TASKS-WORKFLOW-STEP-SCRIPT-006.5:** All new user-facing text shall be
  localized in the supported catalogs. Commands and output shall remain
  verbatim user/runtime data.

### REQ-TASKS-WORKFLOW-STEP-SCRIPT-007: Trust and observability

**Intent:** Treat workflow scripts as explicit code execution and expose enough
evidence to diagnose their lifecycle.

#### Acceptance criteria

- **AC-TASKS-WORKFLOW-STEP-SCRIPT-007.1:** Only users already authorized to
  mutate a workflow shall be able to add or change a script action.
- **AC-TASKS-WORKFLOW-STEP-SCRIPT-007.2:** Import and sync documentation shall
  state that a workflow can execute commands with the selected task executor's
  permissions and that command output is persisted in task chat.
- **AC-TASKS-WORKFLOW-STEP-SCRIPT-007.3:** Structured logs shall identify the
  task, workflow, step, trigger, action position, run, session, execution,
  process, status, failure policy, duration, and exit code without logging
  environment values.
- **AC-TASKS-WORKFLOW-STEP-SCRIPT-007.4:** The system shall expose counters for
  starts and terminal outcomes labeled by trigger and outcome. Labels shall not
  contain task, command, workflow, or other unbounded values.

### REQ-TASKS-WORKFLOW-STEP-SCRIPT-008: Focused inline step editor

**Intent:** Reduce the density of the existing workflow editor while preserving
its familiar workflow card, step strip, and inline editing model.

#### Acceptance criteria

- **AC-TASKS-WORKFLOW-STEP-SCRIPT-008.1:** Workflows shall remain editable in
  the existing workspace Workflows settings list. Editing an existing or new
  workflow shall not navigate to a dedicated workflow-editor route or replace
  the current workflow card layout.
- **AC-TASKS-WORKFLOW-STEP-SCRIPT-008.2:** Each workflow card shall retain its
  workflow-level fields, compact ordered step strip, and one inline panel for
  the selected step. Selecting another step shall replace the panel contents
  without expanding every step or introducing a freeform graph canvas. The
  selected-step heading shall retain editable step name and color controls.
- **AC-TASKS-WORKFLOW-STEP-SCRIPT-008.3:** The selected-step panel shall contain
  compact **Agent**, **Automation**, and **Policies** tabs near its header.
  Desktop tabs shall use a small segmented-control treatment rather than three
  large full-width buttons. Selecting an action shall replace the Automation
  tab's action list with one focused editor and an explicit way back.
- **AC-TASKS-WORKFLOW-STEP-SCRIPT-008.4:** The Automation tab shall preserve
  every existing event editor, including task entry, turn start, turn complete,
  child-task completion, and task exit, as compact ordered action groups. Its
  add-action palette shall show only action types supported by the selected
  trigger, and transition actions shall update the step strip summary. The
  `run_script` action shall remain limited to entry, turn completion, and exit.
- **AC-TASKS-WORKFLOW-STEP-SCRIPT-008.5:** Workflow-level configuration checks
  shall identify invalid or incomplete steps. Selecting an issue shall select
  the exact step and open the tab, action, or field that can resolve it within
  the same workflow card.
- **AC-TASKS-WORKFLOW-STEP-SCRIPT-008.6:** The redesign shall preserve the
  settings manual-save contract. Workflow, tab, step, and action selection
  shall retain the existing page-local drafts; the shared Save changes surface
  shall save every dirty workflow on the page; and leaving with dirty changes
  shall use the existing save, discard, or continue-editing confirmation.
- **AC-TASKS-WORKFLOW-STEP-SCRIPT-008.7:** On phone viewports, the workflow card,
  compact step strip, and inline selected-step panel shall retain the same
  information hierarchy. The step strip may scroll inside its own horizontal
  region, while tab content stacks below it without a dedicated journey,
  full-height step route, or nested editor navigation.
- **AC-TASKS-WORKFLOW-STEP-SCRIPT-008.8:** Mobile shall provide the same
  authoring capabilities as desktop, use explicit move up/down actions where
  drag is not appropriate, keep interactive targets at least 44 by 44 CSS
  pixels, respect safe-area insets, use one vertical scroll owner per screen,
  expose no hover-only operation, and avoid document-level horizontal
  overflow.
- **AC-TASKS-WORKFLOW-STEP-SCRIPT-008.9:** Read-only synchronized workflows
  shall keep the same inline selection and inspection experience while mutation
  affordances are disabled with a visible reason.
- **AC-TASKS-WORKFLOW-STEP-SCRIPT-008.10:** The editor redesign shall preserve
  existing workflow persistence, import/export, sync, inheritance, transition,
  and execution semantics. Existing fields such as workflow default agent
  profile, workflow description, step color, auto-archive timing, WIP/pull, and
  child-task completion transitions shall remain editable on supported
  viewports. All new user-facing text shall be localized in the supported
  catalogs.

## Scenarios

- **GIVEN** Work uses profile A and Review uses profile B, **WHEN** Work's
  completion script succeeds and the task enters Review, **THEN** completion
  and exit output appears in profile A's session while Review's entry output
  appears in the selected profile B session before its prompt.
- **GIVEN** Review returns to profile A with destination policy `reuse`,
  **WHEN** the entry script runs, **THEN** the output appears in the reused
  profile A session and no new session is created for the script.
- **GIVEN** a turn-complete script exits non-zero with `block`, **WHEN** a move
  action also resolves a destination, **THEN** the task remains in the source
  step and chat shows the failed exit code.
- **GIVEN** the same script uses `continue`, **WHEN** it exits non-zero,
  **THEN** chat shows the failure and the configured transition continues.
- **GIVEN** the backend restarts after process admission, **WHEN** the trigger
  is replayed, **THEN** Kandev reconciles or interrupts the existing run and
  never starts the command again.
- **GIVEN** a workflow with several steps and actions, **WHEN** an author edits
  it on desktop, **THEN** its existing compact step strip remains visible while
  the inline selected-step panel changes between Agent, Automation, and
  Policies.
- **GIVEN** an invalid action in a non-selected step, **WHEN** an author selects
  its configuration issue, **THEN** the editor selects that step and opens the
  action field that caused the issue.
- **GIVEN** a dirty script action, **WHEN** an author selects another step and
  returns, **THEN** the unsaved command remains in the page-local workflow draft
  and no persistence request occurs until Save changes is selected.
- **GIVEN** the same workflow on a phone, **WHEN** an author selects a step and
  edits an action, **THEN** the edit stays inside that workflow card, the tabs
  remain touch-safe, and the page does not gain horizontal overflow.

## Out of scope

- Running scripts on `on_turn_start`, `on_worktree_created`, Office/Phase-2
  triggers, or other triggers that do not yet have a stable session-binding
  contract.
- A separate `script_path` action field, per-repository working-directory
  selectors, interactive TTY input, shell selection, retries, schedules, or
  user-defined environment variables. A checked-in script can be invoked from
  `command`.
- Replacing repository setup, cleanup, copy-files, or agent boot scripts.
- Automatically retrying an interrupted, timed-out, or failed command.
- Redacting secrets that a configured command deliberately writes to stdout or
  stderr.
- A freeform workflow canvas, arbitrary node coordinates, zoom controls, or
  user-authored graph branches beyond the existing transition model.
- Dedicated existing-workflow or new-workflow editor routes, route-encoded step
  selection, desktop side inspectors, and mobile journey/step/action screens.
- New workflow-level defaults or inheritance rules for existing per-step
  booleans and policies. The editor may explain current inheritance but shall
  not change its persisted semantics.
- Live **Test action** execution from the editor, because it would introduce a
  separate side-effect and authorization lifecycle.
- Persisting unfinished editor drafts across reloads or devices.
