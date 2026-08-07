---
spec: docs/specs/ui/cancel-turn-progress.md
created: 2026-08-03
status: completed
---

# Implementation Plan: Backend-owned cancel-turn progress

## Overview

Promote the orchestrator's existing per-session cancellation registry into the authoritative
runtime projection described by
[ADR-2026-08-03](../../decisions/2026-08-03-backend-owned-cancellation-progress.md). Publish the
projection through live session events and every session hydration path, then make the shared cancel
control combine that backend value with its existing short-lived optimistic request state. Replace
the request-held browser regression with backend-accepted task-switch and reload coverage on desktop
and mobile.

## Root cause and superseded direction

`SubmitButton` originally owned cancellation progress in component-local state, so a task-route
remount lost it. Tasks 01 and 02 moved that state into the application UI store and proved SPA
navigation, but the store is still scoped to one loaded browser page. It cannot hydrate a reload or
second tab and it treats the request promise, rather than the orchestrator operation, as the source
of truth.

The existing `Service.cancelOperations` registry already tracks actual cancellation intent and the
existing `cancelInFlight` guard already deduplicates lifecycle work. The revised implementation
exposes that fact instead of adding another lifecycle state or a durable database marker. The Task
01 store remains useful only as immediate optimistic feedback before backend acceptance. Runtime
transitions carry a process-local per-session revision so independent REST, boot, and WebSocket
delivery can be ordered without relying on the database timestamp.

## Backend

### Cancellation operation ownership

- `apps/backend/internal/orchestrator/service.go`: document the cancellation registry as the source
  of the public runtime projection; keep its reference-counted, per-session ownership separate from
  the broader `cancelInFlight` mutex.
- `apps/backend/internal/orchestrator/task_operations.go`: expose
  `CancellationPending(sessionID string) bool` and the atomic
  `CancellationPendingSnapshot(sessionID string) (bool, uint64)` form; make
  `beginCancelInFlight` publish only the `0 -> 1` and `1 -> 0` transitions; increment the
  process-local revision and append the transition under the same critical section; drain each
  session's queue outside the lock; retain idempotent release and duplicate-request reference
  counting.
- In `Service.CancelAgent`, authorize with the caller context and then use
  `context.WithoutCancel(ctx)` for the accepted cancellation, reconciliation, message, turn, and
  transition publications. This preserves page-disconnect behavior without removing the lifecycle
  manager's own ten-second cancel wait and escalation bounds.
- Publish `{session_id, cancellation_pending, cancellation_revision}` on
  `events.TaskSessionCancellationChanged`. Event-publication failure is logged but never changes the
  cancellation result; the final deferred release must clear pending on every error path.

### Session wire contract and routing

- `apps/backend/internal/events/types.go` and `apps/backend/pkg/websocket/actions.go`: add the
  semantic event/action pair `task_session.cancellation_changed` /
  `session.cancellation_changed`.
- `apps/backend/internal/gateway/websocket/task_notifications.go`: subscribe to the event and route
  it only through `Hub.BroadcastToSession`. It is semantic session traffic, not replaceable stream
  traffic and not a task-summary update.
- `apps/backend/internal/task/dto/dto.go`: add non-omitempty `CancellationPending bool` and
  `CancellationRevision uint64` fields to both `TaskSessionDTO` and `TaskSessionSummaryDTO`; add a
  narrow atomic snapshot provider alongside `CancellationPendingProvider` and enrichers that always
  write the provider result.
- `apps/backend/internal/task/handlers/task_handlers.go` and
  `task_http_handlers.go`: discover the provider from the orchestrator and enrich list/get session
  responses alongside foreground activity.

### Boot, REST, and subscription reconciliation

- `apps/backend/internal/backendapp/boot_state.go`: enrich every task-detail and quick-chat session
  DTO before inserting it into `taskSessions.items` and `taskSessionsByTask`.
- `apps/backend/internal/backendapp/helpers.go`: pass a cancellation provider into
  `buildSessionDataProvider` and add explicit `cancellation_pending` to the authoritative
  `session.state_changed` subscription snapshot.
- `apps/backend/internal/backendapp/main.go`: wire `orchestratorSvc` into the session data provider.
  A fresh page or a client that missed a live transition must therefore reconcile without waiting
  for another cancel event.
- No repository, schema, migration, or session-metadata change is planned. A backend restart resets
  the runtime projection and leaves existing coarse session/execution recovery authoritative.

## Frontend

### Session contracts and live updates

- `apps/web/lib/types/http.ts`: add the backend-owned `cancellation_pending` and optional
  `cancellation_revision` fields to `TaskSession`; backend wire boundaries emit both explicitly.
- `apps/web/lib/types/backend.ts`: add both cancellation fields to the state-snapshot payload,
  define the revision-bearing `TaskSessionCancellationChangedPayload`, and register
  `session.cancellation_changed` in the backend message map.
- `apps/web/lib/ws/handlers/agent-session.ts`: merge explicit true and false values carried by
  `session.state_changed`; handle revision-bearing `session.cancellation_changed` by updating the
  existing session row and its per-task list without changing coarse state or triggering task/Office
  lifecycle work; reject lower-revision snapshots/events.
- `apps/web/lib/ws/handlers/agent-session.test.ts`: cover explicit-false clearing, session isolation,
  an event arriving for an unloaded session, and state-snapshot/live-event convergence.

### Shared cancel control

- `apps/web/components/task/chat/chat-input-toolbar-primitives.tsx`: derive effective progress from
  `taskSessions.items[sessionId].cancellation_pending ||
  chatInput.cancellingBySessionId[sessionId]`. Keep the current UI-slice value as immediate
  optimistic feedback and a same-page duplicate-click guard; it must not override an authoritative
  backend `true` after the request promise settles.
- `apps/web/components/task/chat/chat-input-toolbar.test.tsx`: retain desktop/mobile remount coverage
  and add a new-store hydration case proving a backend-pending session renders the spinner after a
  page boundary. Prove explicit backend false plus cleared optimistic state returns to idle.
- The existing `chatInput` store action remains transient and is not added to boot state or browser
  persistence.

### Mobile design contract

Desktop and mobile continue using the same inline `SubmitButton`; there is no new surface, copy,
hierarchy, safe-area behavior, scroll owner, or gesture. The backend session field drives both
layouts. Because reload persistence changes user-visible behavior on compact screens, a dedicated
`mobile-chrome` Playwright regression is required in addition to the shared component coverage.

## Tests

- **Backend operation lifecycle:** `apps/backend/internal/orchestrator/task_operations_test.go`
  blocks the mock manager, observes pending true, cancels the initiating context, and proves the
  accepted operation remains pending until release and clears on success/error. Concurrent requests
  must still invoke the manager once and publish one true/false pair.
- **DTO projection:** add focused tests under `apps/backend/internal/task/dto/` for full and summary
  serialization of explicit true and false values.
- **REST and boot hydration:** extend
  `apps/backend/internal/task/handlers/task_http_handlers_test.go` and
  `apps/backend/internal/backendapp/helpers_test.go` to assert current cancellation state in list,
  get, task-detail boot, and initial session-subscription payloads.
- **WebSocket routing:** extend
  `apps/backend/internal/gateway/websocket/task_notifications_test.go` to prove cancellation events
  reach only the subscribed session and preserve both transition values.
- **Frontend reconciliation:** extend
  `apps/web/lib/ws/handlers/agent-session.test.ts` and
  `apps/web/components/task/chat/chat-input-toolbar.test.tsx` for hydration, live updates,
  revision-aware delayed hydration, optimistic/backend union semantics, session isolation, and
  desktop/mobile rendering.

## E2E Tests

- **Task switch and reload:** revise
  `apps/web/e2e/tests/chat/cancel-progress-task-switch.spec.ts`. Start the existing slow mock-agent
  turn, send cancellation through to the backend, wait until the exposed session store reports
  `cancellation_pending=true`, switch away and back, reload the page, and assert the same disabled
  animated control until the backend settles. This replaces the current helper that holds the
  request before backend acceptance.
- **Compact reload parity:** add
  `apps/web/e2e/tests/chat/mobile-cancel-progress-reload.spec.ts`, limited to `mobile-chrome`. Tap
  cancel during the slow mock turn, wait for backend-owned pending state, reload, and assert the
  shared control remains disabled and animated without desktop-only navigation assumptions.
- **Test support:** add a cancellation-pending reader/waiter to
  `apps/web/e2e/helpers/session-store.ts`; remove
  `routeMainWebSocketWithHeldCancelRequest` from `apps/web/e2e/helpers/ws-drop.ts` once no test uses
  it. The existing `/slow` mock scenario is cancellation-insensitive long enough to keep the
  lifecycle operation deterministically pending; no production delay or new test-only endpoint is
  required.

## Verification Results

The backend-owned package is complete. Superseded Tasks 01 and 02 below remain as historical
records; revised Tasks 03-06 are the authoritative implementation evidence.

- Backend lifecycle-focused tests: 3 passed; cancellation regression set: 16 passed; race set: 4
  passed.
- Backend projection packages (`internal/task/dto`, `internal/task/handlers`, `internal/backendapp`,
  `internal/gateway/websocket`): 681 passed.
- Frontend-focused contract/control/store tests: 104 passed; typecheck, lint, and i18n ratchet passed.
- `make -C apps/backend test` and `make -C apps/backend build` completed without failures.
- Desktop backend-owned switch/reload E2E: 1 passed; mobile-chrome reload E2E: 1 passed.
- `git diff --check`: passed.
- No schema migration, durable cancellation marker, cross-workspace broadcast, held WebSocket frame,
  or external side effect was added.

## Review remediation results

- RED: deterministic successor-generation ordering, waiting-state caller-cancellation, and delayed
  REST hydration tests failed against the pre-fix implementation.
- Green backend: `go test ./internal/orchestrator ./internal/task/dto ./internal/task/handlers
  ./internal/backendapp ./internal/gateway/websocket -count=1` — 2,138 passed.
- Race: full `go test -race ./internal/orchestrator -count=1` — 1,453 passed; cancellation-focused
  race set — 5 passed.
- Frontend: the five focused session/control suites — 134 passed; `pnpm run typecheck`, web lint,
  and the i18n ratchet passed.
- `git diff --check` — passed.

The remediation adds a process-local per-session revision to every cancellation snapshot/event,
atomically queues transitions with count changes, rejects lower-revision client merges, and routes
the waiting-state retry through the detached accepted-operation context. No new mobile composition
was introduced; the existing mobile reload E2E covers the shared control, so a new mobile E2E was not
needed for this data-ordering-only change.

## Implementation Waves And Parallel Candidates

Previous implementation record (completed, partially retained as optimistic behavior):

- [x] [task-01-session-scoped-cancel-state](task-01-session-scoped-cancel-state.md) — frontend
  navigation state; superseded as the authoritative owner by Tasks 03-05.
- [x] [task-02-task-switch-regression-e2e](task-02-task-switch-regression-e2e.md) — request-held
  regression; replaced by Task 06's backend-accepted navigation/reload coverage.

Revised implementation, sequential dependency chain:

- [x] [task-03-backend-cancellation-lifecycle](task-03-backend-cancellation-lifecycle.md) — completed;
  establishes runtime ownership and operation transitions.
- [x] [task-04-cancellation-projection-contract](task-04-cancellation-projection-contract.md) —
  completed; depends on Task 03 and exposes hydration/live contracts.
- [x] [task-05-backend-owned-cancel-control](task-05-backend-owned-cancel-control.md) — completed;
  depends on Task 04 and consumes the backend projection.
- [x] [task-06-cancel-reload-regression-e2e](task-06-cancel-reload-regression-e2e.md) — completed;
  depends on Task 05 and proves desktop/mobile navigation and reload behavior.

These tasks are not parallel-safe: each changes or validates the contract established by the
preceding task. The wave list does not authorize subagents.
