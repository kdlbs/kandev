# ADR-2026-08-08-task-autopilot-contract: Make Task Autopilot a Creation-Time Runtime Contract

**Status:** accepted
**Date:** 2026-08-08
**Area:** backend, agentctl, frontend, protocol, workflow
**Related issue:** [#2425](https://github.com/kdlbs/kandev/issues/2425)
**Related spec:** [Task Autopilot Mode](../specs/tasks/autopilot-mode.md)

## Context

Kandev task agents currently receive a task-mode system prompt and an MCP tool
inventory when their runtime session is launched. An agent can ask the operator a
clarification through `ask_user_question_kandev`, but that call waits for the
operator's answer. This is unsuitable for delegated autonomous work: a child can
block while its parent is busy, the question can time out, and the operator can be
shown a question that the parent agent could have answered.

Autopilot changes both behavioral guidance and the capabilities an agent should
discover. Allowing the setting to change during a running task would require an
atomic prompt and tool-inventory replacement across active, waiting, resumed, and
restarted sessions. A partially applied change would create a split-brain session
whose prompt and available tools disagree.

## Decision

1. Autopilot is an immutable, persisted property of a task in its first release.
   It is selected at creation, defaults to `false`, and is exposed in task DTOs.
   Editing an existing task cannot change it.
2. Creation APIs, including `create_task_kandev`, accept an explicit `autopilot`
   boolean. The value does not implicitly inherit from the parent when omitted.
   An autopilot agent that wants autonomous delegation must create its child with
   `autopilot: true` deliberately.
3. The backend derives the session's behavioral prompt and MCP capability set from
   the persisted task property. Agentctl enforces the supplied capability set; it
   does not infer autopilot from prompts or parentage.
4. An autopilot child with a direct parent does not discover
   `ask_user_question_kandev`. It instead discovers `ask_parent_question_kandev`,
   which records a structured question and sends it to the task's current direct
   parent without waiting for an answer. The parent target is resolved by the
   backend and is never caller supplied. An autopilot root receives no question
   tool.
5. `ask_parent_question_kandev` is a terminal action for the current logical turn.
   The tool response tells the agent to end its turn, and the autopilot system
   prompt requires the tool to be its last action. At turn completion the child
   remains waiting on the recorded parent question rather than completing its
   workflow transition or draining unrelated queued prompts.
6. A parent answers through `message_task_kandev` with a correlated
   `reply_to_question_id`. The backend accepts the answer only from the recorded
   direct parent and routes it to the recorded child session. Duplicate replies
   are idempotent.
7. Pending parent questions use durable message metadata as their source of truth.
   They survive restart, project to the existing clarification pending-action
   category, and therefore reuse the sidebar question indicator without exposing
   an operator clarification card. They leave the pending state when answered,
   superseded by explicit new child input, or made stale by terminal task/session
   state.
8. A top-level autopilot task has no human-question escape hatch. It proceeds with
   best judgment; calling the parent-question tool fails closed with a clear
   no-parent result. Nested autopilot hierarchies may relay an essential question
   one direct-parent hop at a time.
9. Autopilot is initially supported only for persistent task-mode sessions whose
   configured agent runtime supports Kandev MCP injection. Creation with an
   incompatible profile is rejected instead of silently providing weaker
   semantics. Quick chat, Office-owned sessions, config sessions, and utility
   sessions are outside this contract.

## Consequences

- Creation-time immutability makes every launch and resume deterministic, at the
  cost of requiring a new task to change modes.
- The backend gains a typed parent-question lifecycle and a correlated extension to
  peer messaging, but no request blocks on another agent or on the operator.
- Existing non-autopilot tasks and root clarification behavior remain compatible.
- The UI can show autopilot as task identity and pending questions as transient task
  state using existing shared desktop/mobile task rows.
- Supporting live toggles later requires a separate decision covering versioned
  runtime configuration and in-flight question migration.

## Alternatives Considered

### Allow the setting to change while a task is running

Deferred because prompt guidance is injected at turn/session boundaries while MCP
discovery has its own runtime update path. A correct mutable design needs a
versioned configuration snapshot, atomic capability replacement, and explicit
semantics for an already-pending operator or parent question. The first release
avoids that split-brain state.

### Keep `ask_user_question_kandev` and rely only on prompt wording

Rejected because prompts are advisory. The blocking tool would remain callable and
could still time out or route a delegated question directly to the operator.

### Implement parent questions by calling `message_task_kandev` directly

Rejected because a free-form peer message has no correlation ID or durable waiting
state. A dedicated request tool is needed to enforce direct-parent routing,
idempotent answers, restart recovery, and the sidebar pending indicator.

### Route every question directly to the root task

Rejected for the initial contract because the immediate parent owns the delegated
work and may already know the answer. Direct-parent hops also preserve the existing
authorization boundary for parent-to-child control. An autopilot parent can relay a
question upward if it genuinely cannot decide.

### Add a separate parent-question table

Rejected because task messages already provide durable, ordered, restart-safe
records and pending-action projection. A typed hidden message plus metadata adds the
needed state without establishing a second conversation store.
