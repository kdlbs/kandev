---
id: "06-task-controller"
title: "Authorized task controller"
status: completed
wave: 3
depends_on: ["02-state-contracts", "04-attachment-hub", "05-language-discovery"]
plan: "plan.md"
spec: "../../specs/lsp-file-intelligence/spec.md"
---

# Task 06: Authorized Task Controller

## Acceptance

- Snapshot, policy, Start/Stop/Restart, and attachment routes are keyed only by task/language and
  authorize the task before any state/runtime/capacity access. Request data cannot choose session,
  environment, execution, generation, initiator, or reason.
- Effective policy, executor support, task/language capacity, deterministic queueing, and
  per-key command serialization produce at most one task-host generation under concurrent controls.
- Unsupported and capacity-blocked Start requests do not create/resume task resources; explicit
  Stop durably disables reacquisition; Restart creates one backend generation and preserves policy.

## TDD sequence

1. Add failing controller tests with call-order fakes proving authorization is first and hidden
   tasks never reach store/environment/capacity/agentctl.
2. Add policy tests for Inherit/Keep warm/Disabled, global auto-start/config updates, explicit
   Start/Stop semantics, registered-language validation, and archived/missing tasks.
3. Add executor/capacity tests: support is resolved from canonical task environment; unsupported
   fails before slot/ensure; queued starts fail before ensure; real slots count task/language
   processes and release only after reaping; legacy env parsing is fallback-only.
4. Add synchronized concurrency tests for duplicate and unlike Start/Stop/Restart calls, detached
   accepted work, monotonic generation/revision, and task-host command retry idempotency.
5. Implement task-scoped REST/WS handlers and backend→agentctl client methods. Remove the
   session route, browser-stream capacity limiter, and any browser-derived auto-install decision.

## Verification

```bash
cd apps/backend && go test ./internal/lsp/... ./internal/gateway/websocket ./internal/agent/runtime/agentctl ./internal/backendapp -run 'Test(LSP|Lsp)'
cd apps/backend && go test -race ./internal/lsp/... ./internal/gateway/websocket -run 'Test(LSP|Lsp)'
cd apps/backend && go test ./internal/lsp/... -run 'Test(Concurrent|Capacity|Authorization|Policy)' -count=20
```

## Files likely touched

- `apps/backend/internal/lsp/controller.go`
- `apps/backend/internal/lsp/controller_test.go`
- `apps/backend/internal/lsp/capacity.go`
- `apps/backend/internal/lsp/capacity_test.go`
- `apps/backend/internal/lsp/capabilities.go`
- `apps/backend/internal/lsp/capabilities_test.go`
- `apps/backend/internal/lsp/handlers.go`
- `apps/backend/internal/lsp/handlers_test.go`
- `apps/backend/internal/agent/runtime/agentctl/client_lsp.go`
- `apps/backend/internal/gateway/websocket/lsp_handler.go`
- `apps/backend/internal/gateway/websocket/lsp_handler_test.go`
- `apps/backend/internal/gateway/websocket/lsp_capacity.go` (removed)
- `apps/backend/internal/gateway/websocket/lsp_capacity_test.go` (replaced by controller capacity tests)
- `apps/backend/internal/gateway/websocket/setup.go`
- `apps/backend/internal/backendapp/gateway.go`
- `apps/backend/internal/backendapp/helpers.go`
- `apps/backend/internal/backendapp/types.go`

## Dependencies

Task 02 provides durable state; Tasks 04–05 provide task-host attach/control/discovery contracts.

## Parallelism

Sequential. This composes every prior backend contract and is the base for Task 07 recovery.

## Inputs

- Spec: API surface, State machine, Permissions, executor/capacity scenarios.
- `task.Service.AuthorizeTaskAccess`, `GetOrEnsureExecutionForEnvironment`, current LSP gateway,
  executor runtime types, and user LSP defaults/configuration.
- ADR security rule: future agent origin derives task from execution; no MCP adapter in this task.

## Output contract

Report public/internal route shapes, authorization call-order proof, capacity/resource side-effect
proof, concurrency/race results, removed session contracts, and exact files. Update task/plan status.

## Results

Completed 2026-08-05.

- Added `internal/lsp.Controller` as the authorized task/language seam for aggregate snapshots,
  effective policy, policy mutation, Start/Stop/Restart, capacity, and attachment resolution.
  Every human operation calls `AuthorizeTaskAccess` before language validation, persistence,
  settings, environment, capacity, execution, or task-host access; synchronized fakes cover every
  exported operation and prove hidden tasks stop at authorization.
- Added a fail-closed executor resolver and a process-capacity controller keyed by
  `(task_id, language)`. It parses `KANDEV_LSP_MAX_SERVERS`, uses the legacy connections variable
  only when the new variable is absent, queues by accepted time/task/language, and never counts or
  releases browser attachments. Unsupported and queued starts do not ensure task resources.
- Added per-key FIFO command serialization with duplicate in-flight coalescing and detached
  accepted work. Explicit Start is idempotent while already desired/live; Stop persists Disabled;
  Restart preserves policy and allocates exactly one monotonic replacement generation.
- Added task-scoped REST routes and strict bodies, backend-to-task-host control/snapshot/attach
  client methods, a lifecycle runtime adapter, and the non-owning
  `/lsp/tasks/:taskId/:language/attach` proxy. Removed `/lsp/:sessionId`, the browser connection
  limiter, browser-derived installation choice, and the legacy task-host browser-owned stream.
- Focused verification passed:
  - `go test ./internal/lsp/... ./internal/gateway/websocket ./internal/agent/runtime/agentctl ./internal/backendapp -run 'Test(LSP|Lsp)'`
  - `go test -race ./internal/lsp/... ./internal/gateway/websocket -run 'Test(LSP|Lsp)'`
  - `go test ./internal/lsp/... -run 'Test(Concurrent|Capacity|Authorization|Policy)' -count=20`
  - `go test ./internal/lsp/... ./internal/agentctl/server/... ./internal/agent/runtime/agentctl ./internal/agent/runtime/lifecycle ./internal/gateway/websocket ./internal/backendapp`

PR remediation on 2026-08-06 added task-host failure-snapshot propagation and releases capacity
only when generation-scoped evidence proves no process was created. A successful Stop releases its
slot and watch even if the following durable write fails. Capacity snapshots now include a backend
epoch and monotonic revision in both REST and language events. Workspace-source reconciliation has
an explicit authorization guard; HTTP and attachment 404s require the typed task-not-found error;
the attachment proxy uses joined ping/read-deadline keepalive. Focused controller, client, gateway,
SQLite, lint, and race checks pass.
