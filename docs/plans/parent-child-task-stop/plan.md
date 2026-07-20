---
spec: docs/specs/tasks/parent-child-task-stop.md
created: 2026-07-19
status: complete
---

# Implementation Plan: Parent-Child Task Control

## Overview

Keep both coordinator controls and make their intent obvious:

- prefer `message_task_kandev(delivery_mode="interrupt")` when a busy child
  should stop its current approach and immediately process a replacement
  instruction;
- use new `stop_task_kandev` when the child should halt with no replacement
  prompt or turn.

Improve interrupt discoverability in the tool description and injected MCP
context, then expose Kandev's existing task-level stop through a direct-parent-
only task-mode MCP tool. Give the coordinator stop path a structured, truthful
result, add the trusted backend action and server adapter, and finish with an
integrated long-running child regression and public contract docs. No frontend
UI changes are required.

The user confirmed both controls should exist, with interrupt preferred for the
usual stop-and-steer case. Planning defaults retained from the timed-out
clarification prompt: stop the whole child task, authorize only its direct
parent, and treat an already-idle child as idempotent `not_running` success.

Transport decision:
[ADR-2026-07-19-reject-mcp-actions-on-raw-websocket](../../decisions/2026-07-19-reject-mcp-actions-on-raw-websocket.md).

---

## Backend

### Structured coordinator-stop semantics

Files:

- `apps/backend/internal/orchestrator/executor/executor_interaction.go`
- `apps/backend/internal/orchestrator/executor/executor_interaction_test.go`
- `apps/backend/internal/orchestrator/executor/executor_mocks_test.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/task_operations_test.go`
- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/orchestrator/event_handlers_streaming.go`
- `apps/backend/internal/orchestrator/event_handlers_streaming_test.go`

Changes:

- Add an orchestrator-level coordinator stop operation that accepts only
  `taskID` and returns a structured `stopped` / `not_running` result. Keep
  executor sentinels and force/reason parameters behind that boundary.
- Define one exact semantic reason constant:
  `coordinatorMCPStopReason = "stopped by parent task via MCP"`, with graceful
  `force=false`. Do not pass caller-controlled text into lifecycle cleanup
  because some executor providers interpret semantic reason strings.
- List active candidate sessions, then inspect them sequentially. For each
  candidate, acquire the existing per-session `cancelInFlightGuard`, re-read
  session/lifecycle state, and issue the logical stop before releasing the
  guard. Process candidates in stable ID order and never hold more than one
  guard or hold it across asynchronous runtime teardown.
- Treat only `errors.Is(err, lifecycle.ErrNoExecutionForSession)` as normal
  absence. Any repository error, any other lifecycle error, or
  `executionID == ""` with no absence sentinel is an internal failure.
- Refactor the stop-specific session transition to return an outcome containing
  whether `CANCELLED` was written and the observed final state, plus error.
  A same-state/terminal race must be re-read: already-`CANCELLED` is
  idempotent, natural `COMPLETED`/other terminal state is no longer live, and
  only an actual `CANCELLED` acceptance counts as stopped.
- Persist `CANCELLED` and publish its existing state event before scheduling
  asynchronous teardown. If persistence fails, return the error and do not
  teardown that candidate; earlier successful candidates remain cancelled and
  retry targets the rest.
- Aggregate coordinator outcomes explicitly: zero accepted stops with every
  candidate naturally absent/terminal gives `not_running`; one or more
  accepted stops with the remainder naturally absent gives `stopped`; any
  genuine lookup/persistence failure gives an internal error, even after partial
  success.
- Reuse the detailed per-session stop primitive, but preserve legacy adapter
  contracts. UI `Service.StopTask`, `CompleteTask`, Office/tree/workspace
  `CancelTaskExecution`, and handoff cleanup keep their caller-supplied
  reason/force values and current error-only policy
  (`not_running -> executor.ErrExecutionNotFound`; task-wide legacy stop may
  still return nil when any session stopped). Add regression coverage rather
  than silently making those callers strict.
- On a structured `stopped` result, attempt a guarded `REVIEW` transition
  only for non-Office, unarchived tasks currently in `IN_PROGRESS` or
  `SCHEDULING`, using `UpdateTaskStateIfCurrentIn`. Rely on the production
  task-service adapter to publish the state event; do not publish a second event
  manually. Log a reconciliation write failure without changing the accepted
  stop result. On `not_running`, leave task state untouched.
- Preserve worktrees, task environments, executor resume records, and queued
  messages. Do not expose or use force-stop behavior.
- Scope the first version to executions registered live when each candidate is
  inspected. Do not add a second lock beside `cancelInFlightGuard`, a durable
  pause flag, or a task-wide launch fence; a launch registering after its
  candidate lookup can escape even if it began before the stop call.

Reason:

The handler needs an idempotent product result, not a guess based on
`executor.ErrExecutionNotFound`. The existing lifecycle seam already provides
immediate logical cancellation plus asynchronous teardown, but it currently
collapses repository errors, swallows session-state persistence errors, tolerates
partial success, and writes `REVIEW` unconditionally. The structured boundary
makes the new agent-facing contract truthful without claiming process-exit or
concurrent-launch guarantees.

### MCP backend action and authorization

Files:

- `apps/backend/pkg/websocket/actions.go`
- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/mcp/handlers/stop_task.go`
- `apps/backend/internal/mcp/handlers/stop_task_test.go`
- `apps/backend/internal/backendapp/helpers.go`
- `apps/backend/internal/backendapp/mcp_stop_composition_test.go`
- `apps/backend/internal/gateway/websocket/client.go`
- `apps/backend/internal/gateway/websocket/client_mcp_actions_test.go`

Changes:

- Add `ActionMCPStopTask = "mcp.stop_task"`.
- Add a narrow `TaskStopper` dependency and wire the production orchestrator
  service through `registerMCPAndDebugRoutes`; keep stop semantics out of the
  MCP handler itself.
- Parse `task_id` and trusted injected `sender_task_id`. Reject malformed JSON
  as `BAD_REQUEST`; require both IDs with `VALIDATION_ERROR`.
- Load sender and target tasks and require both same-workspace membership and
  `target.ParentID == sender.ID` before invoking the stopper. Do not reuse the
  broader handoff/document relationship policy.
- Invoke the structured coordinator stop operation and return its
  `status="stopped"` or `status="not_running"` result directly. The handler
  must not map raw executor sentinels or accept reason/force fields.
- Return typed `VALIDATION_ERROR`, `NOT_FOUND`, `FORBIDDEN`, and
  `INTERNAL_ERROR` responses without leaking a caller-controlled sender ID.
- Register the new backend action and update the always-registered MCP handler
  count from 24 to 25.
- Add a production-composition test that dispatches `mcp.stop_task` through the
  backendapp wiring and proves a non-nil orchestrator stopper is installed; unit
  tests with an injected fake alone are insufficient.
- Enforce the ADR transport boundary in the raw gateway client: reject every
  `mcp.*` action before shared-dispatcher invocation. Agent-stream MCP and the
  external MCP server keep their existing direct adapter paths; the latter does
  not register `stop_task_kandev`.
- Add a bypass regression proving a raw gateway request with forged
  `sender_task_id` receives `FORBIDDEN` and never reaches the stop handler,
  while the agent-stream/direct MCP dispatch path remains functional.

Reason:

Authorization must be enforced before runtime mutation, and injected identity is
trustworthy only when raw WebSocket clients cannot reach internal MCP actions. A
dedicated action also keeps user-originated `orchestrator.stop` separate from
agent-originated, direct-parent-authorized requests.

### Task-mode MCP server adapter

Files:

- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/handlers.go`
- `apps/backend/internal/mcp/server/server_test.go`
- `apps/backend/internal/mcp/server/handlers_test.go`
- `apps/backend/internal/mcp/server/sysprompt_sync_test.go`
- `apps/backend/config/prompts/kandev-context.md`

Changes:

- Register `stop_task_kandev` only in `ModeTask`, with required `task_id`
  only. Describe all-session stop, direct-child authority,
  eligible active-Kanban `REVIEW`, idempotency, async teardown, and the
  difference from interrupting `message_task_kandev`.
- Forward to `mcp.stop_task` while injecting only trusted `s.taskID` as
  `sender_task_id`; expose neither identity, reason, nor `force` as tool
  arguments.
- Rewrite the opening guidance for `message_task_kandev` as the reciprocal
  three-way choice: ordinary information stays queued; urgent direct-parent
  replacement work uses `delivery_mode="interrupt"`; halt-only intent uses
  `stop_task_kandev`. Remove the ambiguous current phrase “steering/stop
  messages,” because interrupt always supplies a replacement message.
- Expand `apps/backend/config/prompts/kandev-context.md` to list the optional
  `delivery_mode` values and the three-way choice: queue, interrupt-and-steer,
  or halt-only stop. This fixes the shortened runtime context that currently
  hides the interrupt parameter from agents.
- Update `registerKanbanTools` accounting from 14 to 15 and total task-mode
  registration from 26 to 27. Assert the stop tool remains absent from Config,
  Office, and External modes.
- Extend schema-to-system-prompt sync coverage so the shortened injected context
  cannot again omit the message tool's `delivery_mode` enum/default,
  direct-parent restriction, or companion stop tool.

---

## Frontend

No frontend code changes. This feature adds an agent-only task MCP operation and
reuses existing session/task state events already rendered by the web client.

---

## Tests

- **What:** the per-session primitive distinguishes accepted cancellation,
  exact absence sentinel, terminal race, lookup failure, persistence failure,
  and empty-ID invariant.
  **File:** `apps/backend/internal/orchestrator/executor/executor_interaction_test.go`
  **How:** table-driven unit tests with recording lifecycle/repository fakes.

- **What:** coordinator traversal attempts candidates in stable order and applies
  the accepted/vanished/failure aggregation table, including idempotent repeat.
  **File:** `apps/backend/internal/orchestrator/task_operations_test.go`
  **How:** table-driven orchestrator tests with detailed stop outcomes.

- **What:** repository/lifecycle lookup errors do not masquerade as
  `not_running`; terminal races do not get overwritten; session-state
  persistence failure prevents teardown for that candidate; the fixed stop
  reason and `force=false` reach the lifecycle manager.
  **Files:** `apps/backend/internal/orchestrator/executor/executor_interaction_test.go`,
  `apps/backend/internal/orchestrator/executor/executor_mocks_test.go`,
  `apps/backend/internal/orchestrator/event_handlers_streaming_test.go`
  **How:** injected errors and recording callbacks; no sleeps.

- **What:** stop serializes with ready-event/pending-move/queue-drain decisions
  through the existing `cancelInFlightGuard`; no queued message is popped or
  replacement turn dispatched after cancellation acceptance.
  **File:** `apps/backend/internal/orchestrator/task_operations_test.go`
  **How:** channel-synchronized race tests with no sleeps.

- **What:** a launch that registers after its candidate lookup is explicitly not
  fenced in v1 and can escape; the test documents this boundary instead of
  accidentally claiming stronger semantics.
  **File:** `apps/backend/internal/orchestrator/task_operations_test.go`
  **How:** channel-synchronized pre-registration launch test.

- **What:** runtime teardown is truly asynchronous and detached from caller
  cancellation.
  **File:** `apps/backend/internal/orchestrator/executor/executor_interaction_test.go`
  **How:** block `StopAgentWithReason`, assert structured stop returns after the
  state event, cancel caller context, then unblock and assert teardown completes.

- **What:** successful task stop writes session `CANCELLED`, uses archive-safe
  guarded `REVIEW` through the existing event-producing adapter for active
  Kanban state only, and preserves workspace/queue data; Office, archived,
  terminal, and `not_running` cases change no task state.
  **File:** `apps/backend/internal/orchestrator/task_operations_test.go`
  **How:** SQLite-backed orchestrator test with fake lifecycle manager and event
  subscriber.

- **What:** direct parent succeeds; self, sibling, child, grandparent, unrelated,
  and cross-workspace callers are forbidden before stopper invocation.
  **File:** `apps/backend/internal/mcp/handlers/stop_task_test.go`
  **How:** table-driven handler test with seeded tasks and recording stopper.

- **What:** malformed JSON, missing/unknown identities, `not_running`, and
  stopper errors map to the specified response codes/statuses; unknown payload
  fields cannot influence reason or force.
  **File:** `apps/backend/internal/mcp/handlers/stop_task_test.go`
  **How:** focused unit tests.

- **What:** task-not-found sentinels map to `NOT_FOUND`, while arbitrary sender
  or target repository failures map to `INTERNAL_ERROR`; neither invokes the
  stopper.
  **File:** `apps/backend/internal/mcp/handlers/stop_task_test.go`
  **How:** injected task-service failures for both lookups.

- **What:** task MCP schema registers and forwards trusted sender identity while
  other server modes omit the tool; the message schema description and injected
  context prominently explain `delivery_mode="interrupt"`.
  **Files:** `apps/backend/internal/mcp/server/server_test.go`,
  `apps/backend/internal/mcp/server/handlers_test.go`,
  `apps/backend/internal/mcp/server/sysprompt_sync_test.go`
  **How:** tool-list/schema/description tests plus exact fake-backend payload
  assertions.

- **What:** production backendapp composition installs the orchestrator-backed
  stopper before handler registration.
  **File:** `apps/backend/internal/backendapp/mcp_stop_composition_test.go`
  **How:** construct the production handler graph and dispatch the new action
  through its dispatcher.

- **What:** raw `/ws` dispatch rejects all `mcp.*` actions, including a forged
  direct-parent stop payload, before the shared dispatcher; ordinary public
  actions still dispatch.
  **File:** `apps/backend/internal/gateway/websocket/client_mcp_actions_test.go`
  **How:** gateway client test with a recording dispatcher handler and captured
  error response.

- **What:** a parent stops a long-running child through the real orchestrator and
  MCP backend action without
  queuing a replacement prompt; the call returns promptly, session becomes
  `CANCELLED`, task becomes `REVIEW`, and a repeat returns `not_running`.
  **File:** `apps/backend/internal/integration/mcp_stop_task_test.go`
  **How:** integration test with the mock agent and real
  orchestrator/dispatcher/MCP-handler wiring. The local task-mode MCP
  tool-to-action forwarding remains covered by server unit tests.

---

## E2E Tests

Skipped. No browser-visible control or new frontend behavior is introduced. The
backend integration test covers authorization through lifecycle state changes;
task-mode MCP registration/forwarding is covered by server tests.

---

## Public documentation

Files:

- `docs/public/automation-and-mcp.md`
- `docs/public/coordination.md`
- `docs/public/sessions-and-review.md`
- `docs/public/feature-status.md`
- `docs/public/websocket-api.md`
- `docs/public/coverage.json`

Changes:

- Add `stop_task_kandev` to task MCP lifecycle/coordination references.
- Make `delivery_mode="interrupt"` the documented recommendation when the
  coordinator has replacement instructions for a busy direct child.
- Explain direct-child authorization, all-session scope, task `REVIEW`, preserved
  workspace, idempotent response, and asynchronous teardown; qualify `REVIEW`
  as the normal active-Kanban transition.
- Contrast halt-only stop with `message_task_kandev(delivery_mode="interrupt")`.
- Add internal `mcp.stop_task` to the WebSocket action catalog and state that
  raw `/ws` clients are rejected for every `mcp.*` action; supported access is
  through the agent-stream or mode-scoped MCP adapters.
- Assign `stop_task_kandev` exactly once to the `task-lifecycle` coverage area
  and add its backend handler plus raw-gateway guard source/test files to that
  area's coverage mapping.
- Update External MCP feature-status wording to name stop among the omitted
  live-session tools.

---

## Implementation Waves

Wave 1:

- [x] [task-01-execution-stop-semantics](task-01-execution-stop-semantics.md)

Wave 2:

- [x] [task-02-coordinator-stop-operation](task-02-coordinator-stop-operation.md)

Wave 3:

- [x] [task-03-mcp-stop-handler](task-03-mcp-stop-handler.md)

Wave 4:

- [x] [task-04-task-mcp-tool](task-04-task-mcp-tool.md)

Wave 5 (parallel):

- [x] [task-05-integration-regression](task-05-integration-regression.md)
- [x] [task-06-public-docs](task-06-public-docs.md)

---

## Verification Commands

Format first:

```bash
make fmt
```

Targeted backend tests:

```bash
cd apps/backend && go test ./internal/orchestrator/executor ./internal/orchestrator ./internal/mcp/handlers ./internal/mcp/server ./internal/backendapp ./internal/gateway/websocket
```

Focused integration test:

```bash
cd apps/backend && go test ./internal/integration -run TestMCPStopTask
```

Public docs validation:

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

Full verification:

```bash
make fmt
make typecheck test lint
```

---

## Risks

- Runtime teardown remains asynchronous by design; tests must distinguish
  logical stop acceptance from confirmed process exit.
- Stop inspects candidate sessions sequentially, not behind a task-wide launch
  fence. An execution registering after its candidate lookup can run; closing
  that race requires a separately designed task-level stop fence.
- Async teardown failure is logged after the response; the first version adds no
  retry/reconciliation guarantee for a lingering external process.
- Existing queued messages intentionally survive stop and may be delivered if
  the child is later restarted.
- Multi-session partial failure can leave some sessions already cancelled;
  returning an error and making retries idempotent is required.

## Approved implementation assumptions

On 2026-07-19, the user approved implementation after confirming both controls
should remain, with interrupt preferred for stop-and-steer. The approved
defaults are:

- stop every current session on the direct child, not only its primary session;
- authorize only the direct parent;
- return idempotent `not_running` for an already-idle child;
- attempt `REVIEW` only for active Kanban state; preserve Office, archived, and
  non-active task state;
- preserve existing queued messages and descendants;
- accept the documented pre-registration launch race in v1 instead of adding a
  durable task-level stop fence;
- enforce the accepted ADR by rejecting the entire internal `mcp.*` namespace
  on raw `/ws` rather than adding per-action caller provenance.
