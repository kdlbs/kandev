---
spec: docs/specs/platform/agent-runtime-availability.md
related_specs:
  - docs/specs/office/costs.md
  - docs/specs/tasks/system-design/runtime-cleanup.md
  - docs/specs/ui/entity-reference-composer.md
created: 2026-08-08
status: completed
---

# Implementation Plan: Backend Failure Containment

## Overview

Contain four failures found in the August 8 incident review:

1. Prevent the fatal `agentctl` concurrent-map crash and publish an explicit
   install-wide runtime outage to the UI.
2. Coalesce `models.dev` refreshes and preserve atomic cache replacement.
3. Make startup runtime cleanup decisions explicit, idempotent, and bounded in
   logs without weakening fail-closed row preservation.
4. Treat canceled mention searches as normal client cancellation and guarantee
   structured diagnostics for every real HTTP 500.

The work uses the existing subsystem boundaries. It does not add automatic
`agentctl` child restart, delete runtime rows with uncertain liveness, or turn
provider-level mention failures into aggregate request failures.

## Confirmed causes and evidence limits

### Agentctl crash and silent UI stall

`convertToolCallResultUpdate` mutates normalized tool payloads under
`Adapter.mu`, but `cloneSubagentPayload` returns the original pointer for every
non-subagent payload. `sendUpdate` queues that pointer and releases the lock.
The HTTP stream writer later marshals it while another update can mutate a
nested map. The fatal stack ends in `NormalizedPayload.MarshalJSON` with
`concurrent map iteration and map write`.

The launcher records an unexpected child exit but publishes no health state.
The backend and browser WebSocket remain connected, so the UI has no signal
that agent and workspace activity is unavailable.

### Models.dev cache race

`warmFromDisk` and `maybeRefresh` can both launch background refreshes, and each
stale lookup can launch another. Direct `Refresh` calls are also uncoordinated.
Every refresh writes the same `<cachePath>.tmp`; one caller can rename or remove
that path before another caller renames it, matching the observed `ENOENT`.

### Startup cleanup warnings

The runtime, lifecycle, and executor layers already preserve typed not-found
identity, and startup cleanup already removes typed-not-found rows only when
local liveness is confirmed dead. The pre-start database shows the warned
legacy standalone rows have `local_pid=0`; liveness therefore classifies them as
Unknown and correctly preserves them. The defect is ambiguous per-row warning
flooding and incomplete decision evidence, not a safe basis for deleting those
rows.

### Mention-search HTTP 500

The handler maps every error other than invalid input and missing workspace to
500 without logging the underlying error. Provider failures normally remain in
HTTP 200 groups. The frontend cancels superseded debounced requests, while the
backend cancellation composition test currently expects that cancellation to
become 500. This deterministic path must become 499. Because the historical 500
had no error record, its exact path cannot be proven after the fact; the fix
also makes every future unexpected 500 observable.

## Backend design

### Immutable ACP event boundary

- Add `NormalizedPayload.Snapshot` in
  `apps/backend/internal/agentctl/types/streams/tool_payload.go`. Copy pointers,
  slices, and recursively copy JSON-compatible map/slice values.
- Snapshot `AgentEvent.Payload` while `Adapter.mu` is held in `sendUpdate` and
  `sendUpdateLocked`, before the event enters `updatesCh`.
- Replace `cloneSubagentPayload` with the common snapshot contract.
- Add deterministic alias tests and concurrent update/marshal race coverage.

### Agent runtime availability

- Add a concurrency-safe tracker under
  `apps/backend/internal/agent/runtime/agentctl/availability.go`. It owns the
  public `available|unavailable` snapshot, stable reason, occurrence time, and
  event publication.
- Extend launcher configuration with an unexpected-exit callback. Invoke it
  only after exit monitoring confirms shutdown was not intentional.
- Mark the runtime available only after child health and authentication succeed.
- Add an internal system event and WebSocket action for status changes. Replay
  the latest full snapshot after each authenticated `user.subscribe`.
- Add the snapshot to authenticated boot/app-state payloads. Keep raw exit text
  and unauthenticated runtime details private.

### Models.dev refresh coordination

- Add one `singleflight.Group` to each models.dev `Client`.
- Route background and explicit refreshes through `Refresh`; move one physical
  fetch/write/swap operation into a private helper.
- Keep stale lookups nonblocking. Concurrent explicit callers wait for and share
  one result.
- Use a unique same-directory temporary file, remove it on every failure, and
  atomically rename it over the cache only after a complete write.
- Preserve current disk and memory data after fetch, cancellation, or write
  failure. Update indexes and `loadedAt` only after the file commit and complete
  in-memory replacement succeed.

### Startup cleanup classification and diagnostics

- Centralize the startup cleanup decision in
  `apps/backend/internal/orchestrator/reconcile_liveness.go`. Record typed runtime
  absence, row liveness, local-PID presence, stop-error class, and disposition.
- Reuse the classifier for missing, terminal, and failed sessions.
- Keep typed-not-found plus confirmed-dead local rows idempotently removable.
  Preserve alive, Unknown, remote, and generic-failure rows.
- Aggregate expected fail-closed outcomes into one structured startup warning
  summary with counts. Keep detailed expected rows at Debug level if useful.
- Keep individual warnings for generic stop failures and other unexpected
  outcomes. Never log resume tokens or credentials.
- Add adapter-boundary tests that confirm typed sentinel normalization and
  repeated-reconciliation tests that distinguish safely removed rows from
  intentionally preserved rows.

### Mention-search cancellation and diagnostics

- Inject a logger into the mention handler and service composition.
- Map `context.Canceled` from a client request to 499, allowing the existing HTTP
  middleware to treat it as routine cancellation.
- Before any unexpected 500, emit exactly one structured Error record with the
  workspace ID and stable failure stage/class.
- Record provider failures with safe source/provider/kind/status context while
  preserving their HTTP 200 group representation.
- Do not log the search query, prompts, credentials, tokens, raw provider error
  bodies, or provider payloads.
- Preserve 400 for invalid requests and 404 for a missing workspace.

## Frontend design

### Authoritative runtime state

- Add the shared wire type and a top-level `agentRuntime` Zustand snapshot.
- Hydrate it from boot state and replace it on
  `system.agent_runtime.status_changed` notifications.
- Do not infer runtime availability from WebSocket connectivity or clear task,
  workspace, session, message, or route state during an outage.

### Persistent recovery alert

- Render `AgentRuntimeUnavailableAlert` as the first in-flow child of the App
  status surface, outside the App status bar feature gate.
- Show one localized, non-dismissible `role="alert"` for `unavailable` without
  raw process diagnostics.
- Reuse the existing Kandev restart capability and progress flow when a
  supervisor supports restart. Otherwise, show manual restart guidance.
- Keep the alert mounted through capability and restart errors; remove it only
  after a new available snapshot.

### Responsive contract

- Desktop/tablet: span the route-content column without covering sidebar or
  bottom status surfaces.
- Phone: stack text and action, keep a 44 px target, preserve route content, and
  add no drawer, toast, overlay, or second bottom bar.

## Test strategy

- ACP snapshot tests mutate nested maps/slices after enqueue and verify queued
  JSON is unchanged; concurrent update/marshal tests run under `-race`.
- Runtime availability tests cover intentional and unexpected exits, sanitized
  state, boot hydration, subscription replay, reconnect, and retained domain
  data.
- Models.dev tests use channel-controlled servers instead of sleeps. They cover
  concurrent homogeneous and mixed lookups, direct refresh calls, failed and
  canceled retry, stale-data retention, and temporary-file cleanup.
- Startup cleanup tests cover missing and terminal/failed sessions with typed
  not-found plus confirmed-dead local rows, alive and Unknown local rows, remote
  rows, generic failures, sentinel normalization, and repeated reconciliation.
- Mention tests cover invalid requests, workspace lookup failure, missing
  workspace, provider failure, cancellation, and unexpected search failure.
  They assert status codes and safe structured log context.
- Desktop and phone E2E tests inject a backend-shaped unavailable runtime
  snapshot. They do not kill the managed E2E `agentctl` process.

## Implementation waves and task order

### Wave 1 — independent backend repairs

- [x] [Task 01: Snapshot mutable ACP event payloads](task-01-snapshot-acp-event-payloads.md)
- [x] [Task 02: Publish agent runtime availability](task-02-publish-agent-runtime-availability.md)
- [x] [Task 05: Coalesce models.dev refreshes](task-05-coalesce-modelsdev-refreshes.md)
- [x] [Task 06: Classify startup runtime cleanup](task-06-classify-startup-runtime-cleanup.md)

These tasks are parallel candidates because they own separate packages and
contracts.

### Wave 2 — consumers and shared route composition

- [x] [Task 03: Surface runtime failure and recovery](task-03-surface-runtime-failure.md) — depends on Task 02.
- [x] [Task 07: Make mention-search failures observable](task-07-observe-mention-search-failures.md) — scheduled after Task 02 because both may edit backend route composition helpers; it has no behavioral dependency on runtime availability.

### Wave 3 — browser coverage

- [x] [Task 04: Cover runtime failure across viewports](task-04-runtime-failure-e2e.md) — depends on Tasks 02 and 03.

Execution remains sequential in the primary conversation unless the user
explicitly authorizes planned implementation sessions.

## Validation commands

- `cd apps/backend && go test -race ./internal/agentctl/types/streams ./internal/agentctl/server/adapter/transport/acp`
- `cd apps/backend && go test -race ./internal/agent/runtime/agentctl/... ./internal/backendapp ./internal/gateway/websocket`
- `cd apps/backend && go test -race ./internal/office/costs/modelsdev`
- `cd apps/backend && go test -race -run 'TestReconcileSessionsOnStartup|TestStopReportsRuntimeAbsent|TestRowProcessLiveness' ./internal/orchestrator ./internal/agent/runtime/lifecycle ./internal/backendapp`
- `cd apps/backend && go test -race ./internal/mentions ./internal/backendapp`
- `cd apps/backend && golangci-lint run ./internal/agentctl/... ./internal/agent/runtime/agentctl/... ./internal/orchestrator ./internal/office/costs/modelsdev ./internal/mentions ./internal/backendapp ./internal/gateway/websocket --timeout=5m`
- `cd apps && pnpm --filter @kandev/web test -- --run lib/state lib/ws/handlers/system-events.test.ts components/app-status-bar`
- `cd apps/web && pnpm run typecheck`
- `cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet`
- `cd apps/web && pnpm e2e:run --host tests/layout/agent-runtime-unavailable.spec.ts`
- `cd apps/web && pnpm e2e:run --project mobile-chrome tests/layout/mobile-agent-runtime-unavailable.spec.ts`

Run `pnpm install --frozen-lockfile` from `apps/` first if this worktree has no
`node_modules`.

## Integrated verification results

All targeted backend race suites passed: ACP (803 tests), agent runtime and
backend composition (562 tests), models.dev (51 tests), startup cleanup (42
tests), and mentions/backend composition (430 tests). The complete narrow
`golangci-lint` command reported no issues.

The frontend slice passed with 758 tests across 88 files, `pnpm run typecheck`
passed, and `i18n:check && i18n:ratchet` passed. The repository’s existing 61
real-locale parity warnings remain advisory and are unrelated to this change.
Desktop and mobile runtime-alert E2E scenarios each passed once; they inject
store state and never stop the managed E2E `agentctl`.

## Risks and boundaries

- Snapshotting must preserve supported JSON scalar types, `json.RawMessage`, and
  typed payload fields without retaining aliases.
- Runtime event publication and user-subscription replay can race; every event
  therefore carries a complete snapshot.
- Singleflight is per client. Unique temporary files also avoid collisions
  across multiple client objects or processes sharing one cache path.
- Startup cleanup must never infer death from typed not-found alone. Historical
  rows with no PID remain intentionally preserved until a separate safe
  migration or operator recovery contract exists.
- The exact cause of the historical mention 500 is unknowable because no error
  was logged. The cancellation path is confirmed by code/tests; structured
  logging closes the remaining diagnostic gap.
- Automatic child restart, health-endpoint changes, historical runtime-row
  deletion, and broad UI action disabling remain outside this package.
