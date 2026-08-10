---
status: implemented
created: 2026-08-08
owner: cfl
---

# Task Autopilot Mode

## Why

Delegated tasks should be able to make progress without repeatedly blocking on the
operator. Today, a child agent can call the operator clarification tool and wait
inside that call even when its parent agent owns the delegation and could answer.
That creates avoidable timeouts, sends questions to the wrong owner, and gives no
durable indication that a child is waiting on its parent.

Autopilot makes autonomy an explicit task contract. It is visible to the operator,
changes the agent's instructions and available MCP tools, and gives an exceptional
question a non-blocking, parent-owned lifecycle.

Decision: [ADR-2026-08-08-task-autopilot-contract](../../decisions/2026-08-08-task-autopilot-contract.md)
MCP profile decision: [ADR-2026-08-08-mcp-tool-profiles](../../decisions/2026-08-08-mcp-tool-profiles.md)

## User experience

### Creation and identity

- Top-level task creation through the UI does not expose an **Autopilot** switch;
  set it through `create_task_kandev` for now.
- The subtask creation dialog contains a compact **Autopilot** switch, off by
  default. Its info control explains that the child works independently and
  asks its parent only when a critical decision blocks progress.
- The setting is immutable after creation. Edit-task and session-creation surfaces
  display no control that implies it can be changed.
- An autopilot task shows a yellow **Autopilot** chip in the status row above its
  chat composer.
- Its sidebar row shows a secondary autopilot icon. Pointer and keyboard users can
  reveal the localized description **Autopilot mode**; assistive technology gets
  the same label.
- Desktop and mobile use the same task property and shared row/composer components.
  The mobile task switcher keeps the icon visible, and the chip wraps without
  causing horizontal page scrolling.

### Autonomous behavior

The first-turn Kandev context for an autopilot task says that the task is in
autopilot mode and must:

- make safe, reversible assumptions and continue independently;
- ask only when a decision is genuinely essential and a wrong assumption would
  materially change the requested outcome;
- never ask the operator directly;
- when a direct parent exists, call `ask_parent_question_kandev` only as the last
  action of the turn, then end the turn immediately;
- when no parent exists, proceed with best judgment and disclose material
  assumptions in its eventual result.

Normal tasks retain their current prompt and operator clarification tool.

The MCP question capability is mutually exclusive:

| Task mode | Direct parent | Question tool |
|---|---|---|
| Normal | Any | `ask_user_question_kandev` |
| Autopilot | Present | `ask_parent_question_kandev` |
| Autopilot | Absent | None |

An autopilot task never receives both question tools. Other task-mode tools remain
available when they are required by the task mode or configured provider.

## MCP profiles

Kandev builds the MCP tool list from a backend-owned profile context. The context
has a base surface and additive capability groups. The agent cannot provide an
arbitrary tool list.

The base surfaces are:

| Surface | Tool surface |
|---|---|
| `kanban-task` | Kanban task operations, planning, walkthrough, review, related tasks, workspace/branch actions, step completion, and diagnostics |
| `office-task` | Interaction, planning, related tasks, and task documents; no Kanban task creation tools |
| `configuration` | Workflow, agent, MCP, executor, and configuration-task tools |
| `external` | Configuration tools plus `create_task_kandev`; no live-session tools |

The first additive groups are `user-question`, `parent-question`, `task-title`,
GitHub PR automation, and GitLab MR automation. The backend adds these groups from
session purpose, autopilot state, direct-parent presence, title ownership, and
attached providers.

The profile is runtime state. It is rebuilt on launch and resume. A live profile
replacement atomically rebuilds the MCP list and sends one `tools/list_changed`
notification. The task's `autopilot` value remains the durable source for the
autopilot choice.

## Creation API

Task creation requests gain an optional boolean `autopilot` field:

```json
{
  "title": "Investigate flaky test",
  "parent_id": "<parent task id>",
  "autopilot": true
}
```

- Omitted is equivalent to `false`.
- The value does not inherit from the parent.
- The HTTP task-creation endpoint and `create_task_kandev` use the same validation
  and persistence path.
- The MCP parameter description is: **"Start this task in autopilot mode. Default:
  false. The value is fixed at creation and is not inherited by subtasks. The agent
  does not ask the user directly; it asks its direct parent only for critical
  decisions."**
- `autopilot: true` is rejected when the resolved agent profile cannot receive the
  Kandev task-mode MCP capability set. It is not silently downgraded.
- Task read/list/boot payloads expose the persisted boolean so every frontend entry
  point renders the same state.

## Parent question protocol

### Request tool

Autopilot child sessions with a direct parent discover this tool instead of
`ask_user_question_kandev`:

```json
{
  "questions": [
    {
      "id": "database",
      "prompt": "Which database should I use?",
      "options": [
        {"label": "SQLite", "description": "Use the embedded database."},
        {"label": "Postgres", "description": "Use the hosted database."}
      ]
    }
  ],
  "context": "The migration needs a database choice before implementation."
}
```

`questions` contains one to four question objects. Each question has a required
non-empty `prompt`; `id`, `title`, and `options` are optional. Sender task and
session identity come from the authenticated MCP context; the caller cannot choose
the parent or child session. `context` gives the parent the short reason for the
request.

The backend resolves the task's current direct parent, persists the question, and
queues an attributed, structured prompt to that parent. The call returns immediately:

```json
{
  "question_id": "<id>",
  "parent_task_id": "<direct parent id>",
  "status": "waiting_for_parent"
}
```

Repeated creation attempts with the same tool-call identity return the same
question and do not send duplicate parent prompts. A task without a direct parent
does not receive this tool. A stale client call gets a no-parent error and creates
no pending record.

The parent prompt includes the child task/session identity, exact question,
reason, blocked effect, options, question ID, and the exact correlated reply shape.
It must not reinterpret the request as an operator clarification automatically.

### Reply

`message_task_kandev` gains an optional `reply_to_question_id`:

```json
{
  "task_id": "<child task id>",
  "prompt": "Preserve the column for this release.",
  "reply_to_question_id": "<question id>"
}
```

For a correlated reply, the backend verifies that:

- the question is still pending;
- the target task and selected session match the recorded child;
- the sender is the child's currently recorded direct parent; and
- the reply has not already been accepted.

An accepted reply atomically resolves the question and starts or resumes the child
with an attributed parent answer. Retrying the same accepted reply returns the
resolved outcome without delivering a second prompt. A wrong parent, wrong child,
unknown question, or stale question is rejected with no delivery side effect.
Uncorrelated `message_task_kandev` behavior is unchanged.

## State model

Each parent question is a typed, hidden child-session message with stable metadata:

- `question_id`
- child task and session IDs
- parent task ID at request time
- question, reason, blocked effect, and optional choices
- originating prompt generation or tool-call identity
- lifecycle status and resolution timestamps

Lifecycle states are:

| State | Meaning | Exit |
|---|---|---|
| `pending` | Parent prompt was durably queued or delivered; child is gated. | Answer, explicit superseding child input, or terminal state. |
| `answered` | One correlated parent answer was accepted and delivered once. | Terminal. |
| `superseded` | Explicit new operator/parent input replaced the wait without claiming to answer it. | Terminal. |
| `stale` | The child or session became terminal, was archived/deleted, or the recorded relationship can no longer accept the answer. | Terminal. |

While the latest child turn has a `pending` parent question:

- the child session settles in `WAITING_FOR_INPUT` after its agent turn ends;
- workflow `on_turn_complete` transitions do not run;
- ordinary queued prompts do not drain ahead of the correlated decision;
- the task's pending-action projection is `clarification`, so the existing sidebar
  question indicator is shown;
- no operator clarification card or `ask_user_question_kandev` request is created.

An explicit new child prompt that is not a correlated answer supersedes the pending
question before normal prompt admission. Archive, delete, cancellation, or another
terminal session transition marks it stale. A late answer to a non-pending question
cannot resume the child.

## Persistence and restart

- The task's autopilot boolean is stored in the task row with a default of false so
  existing databases and tasks remain non-autopilot.
- Parent questions and their lifecycle metadata are committed before the parent
  prompt is dispatched.
- Task list, boot-state, and status-summary rebuilding derive the question indicator
  from durable pending metadata after restart.
- Restart recovery preserves a pending child gate and does not replay the parent
  prompt if its durable delivery/queue record already exists.
- Answer resolution and child prompt admission are atomic from the protocol caller's
  perspective: a crash cannot make one answer create two child turns.

## Permissions and boundaries

- Only the authenticated task session may open its own parent question.
- Only the recorded direct parent task may answer it.
- Parent routing is one hop. An autopilot parent that cannot decide may open its own
  question to its parent, producing a separate correlation ID.
- The feature applies to persistent task-mode sessions with Kandev MCP-capable agent
  profiles. Quick chat, Office-owned work, config/utility sessions, passthrough-only
  profiles, and non-task MCP modes are out of scope.
- Autopilot is not an authorization bypass. All existing task, workspace, and MCP
  handler checks still apply.

## Failure behavior

- If durable question creation fails, no parent prompt is sent and the tool returns
  an error; the agent may continue with best judgment.
- If parent prompt dispatch fails after persistence, the pending record remains
  recoverable and retry does not create a second question.
- If no parent exists, no pending indicator is created.
- If the parent is busy, the question uses the existing peer-message queue and the
  child waits without holding an MCP request open.
- If an answer races a superseding prompt or terminal transition, exactly one state
  change wins; the loser has no delivery side effect.
- Multiple independent questions cannot be pending for one child session at once.
  A second request in the same wait returns the existing pending question rather
  than producing a message storm.

## Acceptance scenarios

**GIVEN** task creation omits `autopilot`, **WHEN** the task is launched, **THEN** it
has the normal prompt, discovers `ask_user_question_kandev`, and shows no autopilot
chip or icon.

**GIVEN** a compatible child task is created with `autopilot: true`, **WHEN** it is
listed and launched, **THEN** the persisted property is returned, the yellow chip
and sidebar icon appear, its system prompt contains the autonomy contract, and it
discovers `ask_parent_question_kandev` but not `ask_user_question_kandev`.

**GIVEN** an autopilot child has an essential question, **WHEN** it calls the parent
question tool, **THEN** the call returns immediately, exactly one structured prompt
is queued or delivered to its direct parent, the child ends in waiting state, and
its sidebar row shows the question indicator.

**GIVEN** the direct parent sends a correlated answer, **WHEN** the answer is
accepted, **THEN** the pending indicator clears and exactly one child turn starts
with the attributed answer.

**GIVEN** Kandev restarts while a child awaits its parent, **WHEN** state is rebuilt,
**THEN** the child remains waiting, the question indicator is restored, and the
parent prompt is not duplicated.

**GIVEN** a pending question is superseded or made stale, **WHEN** a late correlated
answer arrives, **THEN** it is rejected and does not resume the child.

**GIVEN** an autopilot task has no parent, **WHEN** its MCP tools are listed,
**THEN** no question tool is present and the task remains responsible for proceeding
with best judgment.

**GIVEN** the task switcher is opened on a narrow mobile viewport, **WHEN** an
autopilot child is waiting on its parent, **THEN** both autopilot identity and the
question state are perceivable without horizontal scrolling or a desktop-only
interaction.

## Out of scope

- Changing autopilot after task creation.
- Automatically inheriting autopilot from a parent.
- Routing directly to the ultimate root instead of the direct parent.
- Asking the operator from a top-level autopilot task.
- Applying autopilot to Office, quick chat, utility/config sessions, or runtimes
  that cannot enforce the Kandev task MCP contract.
- General-purpose approvals, permissions, or multi-party voting.
