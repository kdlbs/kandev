---
status: shipped
created: 2026-07-19
owner: cfl
---

# Parent-Child Task Stop

## Why

A coordinating task can already stop and steer a busy direct child with
`message_task_kandev(delivery_mode="interrupt")`. That is the preferred control
when the child should abandon its current approach and immediately act on a new
instruction. Agents rarely choose it today because the short injected MCP context
does not mention `delivery_mode`, while queued delivery is the default.

A separate case remains: the coordinator no longer needs any work from the child.
Sending an interrupting message would cancel the current turn but immediately
start a replacement turn just to process that message. Coordinators need a
halt-only operation that terminates unnecessary child execution without waiting
for a turn boundary, queuing a message, or starting another agent turn.

## What

- Make the existing interrupt choice prominent in both the
  `message_task_kandev` tool description and injected Kandev MCP context. The
  guidance says to prefer `delivery_mode="interrupt"` for urgent stop-and-steer
  instructions to a running direct child.
- Task-mode MCP exposes `stop_task_kandev` for stopping a current task's direct
  child.
- Stopping is distinct from
  `message_task_kandev(delivery_mode="interrupt")`: it sends no prompt and starts
  no replacement turn.
- One call requests a graceful stop for every execution Kandev observes as live
  while inspecting the target task's active-session candidates, including
  non-primary sibling sessions. There is no session-specific option.
- Kandev marks each accepted session stop `CANCELLED`, initiates graceful runtime
  teardown, and attempts the existing guarded task-level transition to `REVIEW`
  for an active, unarchived, non-Office task.
- Runtime teardown starts immediately but remains asynchronous. A successful tool
  response confirms Kandev accepted the stop and changed logical session state;
  it does not claim the operating-system process has already exited.
- Worktrees, task environments, commits, task records, descendants, and existing
  queued messages are preserved. The stopped task can be started again later.
- Stopping a child with no live execution is an idempotent success and does not
  change task or session state.

### Choosing the control

| Coordinator intent | Operation | Result |
|---|---|---|
| Send information that can wait | `message_task_kandev` with `delivery_mode="queued"` or omitted | Current turn continues; message waits FIFO. |
| Stop the current approach and give replacement work now | `message_task_kandev` with `delivery_mode="interrupt"` | Requests immediate cancel-and-redispatch. If immediate dispatch cannot land safely, the message remains queued. |
| Stop all current work because nothing should replace it | `stop_task_kandev` | Logical cancellation is accepted and runtime teardown begins; no message or replacement turn is created. |

## API surface

Decision:
[ADR-2026-07-19-reject-mcp-actions-on-raw-websocket](../../decisions/2026-07-19-reject-mcp-actions-on-raw-websocket.md)

Task-mode MCP tool request:

```json
{
  "task_id": "<full child task UUID>"
}
```

The MCP server injects the calling task identity. Callers cannot supply or
override it. The backend uses a fixed, non-destructive semantic stop reason for
this operation; caller-controlled text is never passed into executor cleanup
logic. The tool does not expose the runtime `force` option and uses graceful
stop.

The existing message API does not change. Its description and injected context
now explicitly advertise optional
`delivery_mode: "queued" | "interrupt"` and the stop-and-steer selection rule.

Internal `mcp.stop_task` action payload:

```json
{
  "task_id": "<full child task UUID>",
  "sender_task_id": "<injected calling task UUID>"
}
```

This action is an internal task-MCP transport surface, not a public raw WebSocket
authorization API. The raw gateway rejects every `mcp.*` action before shared
dispatcher invocation; agent-stream and mode-scoped MCP adapters bypass that raw
path. The task-mode MCP server supplies `sender_task_id`; unknown payload fields
are ignored and cannot alter the fixed reason or graceful-stop policy. External
MCP does not register the stop tool.

Accepted stop response:

```json
{
  "task_id": "<child task UUID>",
  "status": "stopped"
}
```

Idempotent no-work response:

```json
{
  "task_id": "<child task UUID>",
  "status": "not_running"
}
```

The backend action is `mcp.stop_task`.

`status="stopped"` means Kandev persisted logical cancellation for at least one
execution and scheduled graceful teardown for every execution still live in the
candidate set when inspected. It does not confirm process exit.

### Errors

| Condition | Code | Side effects |
|---|---|---|
| Malformed JSON | `BAD_REQUEST` | None |
| Raw `/ws` caller attempts any `mcp.*` action | `FORBIDDEN` | Handler is not invoked |
| Missing `task_id` or trusted caller identity | `VALIDATION_ERROR` | None |
| Caller task or target task does not exist | `NOT_FOUND` | None |
| Target is not the caller's direct child or is in another workspace | `FORBIDDEN` | None |
| Repository/lifecycle lookup, session `CANCELLED` write, or one or more synchronous stop attempts fails | `INTERNAL_ERROR` | Sessions already cancelled remain cancelled; untouched or failed candidates can be retried |

## Permissions

Only the target task's direct parent may call `stop_task_kandev`. Self, sibling,
child-to-parent, grandparent, unrelated, and cross-workspace callers are rejected
before any session or runtime mutation. The check uses the same direct-parent
predicate as parent interrupt messages, plus a defense-in-depth same-workspace
check. See
[Parent-Child Message Interrupt](parent-child-message-interrupt.md).

The tool is absent from Config, Office, and External MCP modes. Those modes either
lack trusted current-task identity or use a different mutation surface.
The raw browser WebSocket rejects the entire `mcp.*` action namespace before
dispatcher routing, so it cannot forge the task-mode server's injected identity.

## State and persistence

- Each target execution found live when its candidate session is inspected
  transitions its session to `CANCELLED` before teardown is scheduled for that
  session. If persistence
  fails, that session is not torn down by this attempt and the call returns an
  error, so a retry can make progress without leaving a live-looking orphan row.
- Stop uses the existing per-session `cancelInFlightGuard` while rechecking and
  accepting logical cancellation. This serializes it with interrupt, ready-event,
  pending-move, and queued-message drain decisions; the guard is released before
  asynchronous process teardown.
- After accepted stops, Kandev attempts to transition a non-Office, unarchived
  task from `IN_PROGRESS` or `SCHEDULING` to `REVIEW` through the normal
  guarded task update and event path. Office-managed, archived, and
  already-terminal/non-active task states remain unchanged. A failure in this
  secondary board-state reconciliation is logged but does not negate accepted
  execution stops.
- Runtime process and executor cleanup continue asynchronously after the response.
- Existing message-queue entries are not removed or dispatched by this tool. They
  retain their normal behavior if the task is later restarted.
- Worktrees and task environments remain available for review or resume.
- No durable pause or cancellation gate is created; later user or workflow actions
  may start the task again.
- Candidate sessions are inspected sequentially. A launch that registers its
  lifecycle execution after its candidate was inspected can escape the call,
  whether that launch began before or after stopping started. The first version
  does not add a task-wide launch fence.

## Failure modes

- If every candidate completes naturally before stop reaches it, the orchestrator
  returns `status="not_running"` after each candidate is found absent.
- Repeating a successful stop returns `status="not_running"` without another
  state transition.
- Only `lifecycle.ErrNoExecutionForSession` means a candidate naturally has no
  live execution. Repository errors, other lifecycle lookup errors, and an empty
  execution ID without that sentinel remain internal errors; they are not
  collapsed into `not_running`.
- Authorization and existence checks complete before stop side effects.
- A synchronous partial failure is reported as an error rather than a false full
  success. Sessions whose stop already succeeded remain cancelled, so retrying
  targets only work that is still live.
- If zero stops are accepted and every candidate has naturally
  vanished, the result is `not_running`. If one or more stops are accepted and
  the rest have naturally vanished, the result is `stopped`. Any genuine lookup
  or persistence failure returns `INTERNAL_ERROR`.
- Failure during asynchronous process teardown is logged after the tool response.
  The first version does not add teardown retry/reconciliation, so a logical
  `CANCELLED` state may disagree with a lingering process until later manual or
  system cleanup.

## Scenarios

- **GIVEN** a direct child is pursuing the wrong approach but still has replacement
  work, **WHEN** its parent needs to steer it now, **THEN** the tool guidance tells
  the parent to use
  `message_task_kandev(delivery_mode="interrupt")`, which requests immediate
  cancel-and-redispatch and truthfully reports `sent` or safe `queued`
  fallback.
- **GIVEN** a direct child has one running session and no replacement work,
  **WHEN** its parent calls
  `stop_task_kandev`, **THEN** the session becomes `CANCELLED`, runtime teardown
  begins, an eligible active Kanban task moves to `REVIEW`, no prompt is
  created, and the response is `status="stopped"`.
- **GIVEN** a direct child has multiple live sessions, **WHEN** its parent stops
  the task, **THEN** every still-live session accepts a stop before the call
  reports `status="stopped"`; a session that completes naturally during the
  operation counts as no longer live.
- **GIVEN** a sibling, grandparent, child, self, unrelated task, or task in another
  workspace, **WHEN** it calls `stop_task_kandev`, **THEN** the call returns
  `FORBIDDEN` and no target state changes.
- **GIVEN** a direct child has no live execution, **WHEN** its parent calls the tool
  once or repeatedly, **THEN** each call returns `status="not_running"` and leaves
  task and session state unchanged.
- **GIVEN** a direct child has descendants, **WHEN** its parent stops that child,
  **THEN** descendant sessions and task states are unchanged.
- **GIVEN** a target has existing queued messages, **WHEN** its parent stops the
  task, **THEN** the stop tool creates and dispatches no message and the existing
  queue entries remain stored for normal later handling.

## Out of scope

- Cancelling only one session or only the current turn.
- Sending a replacement prompt; use
  `message_task_kandev(delivery_mode="interrupt")` for stop-and-steer behavior.
- Changing queued delivery to implicit interruption. Interrupt remains an explicit
  per-call choice.
- Recursively pausing or cancelling a subtree.
- Setting the task state to `CANCELLED`, archiving, deleting, or cleaning up its
  worktree or task environment.
- Stopping self, siblings, ancestors, grandchildren, or unrelated tasks.
- A force/SIGKILL option or waiting synchronously for process exit.
- A durable pause flag, task-wide launch fence, or guarantee against a launch
  that registers its lifecycle execution after its candidate session was
  inspected.
- Clearing or delivering existing queued messages.
- External/admin MCP stop access.
