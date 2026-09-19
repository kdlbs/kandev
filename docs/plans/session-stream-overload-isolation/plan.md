---
spec: docs/specs/platform/requirements/bounded-task-status-delivery.md
decision: docs/decisions/2026-08-02-isolate-replaceable-session-stream-traffic.md
created: 2026-08-02
status: complete
---

# Implementation Plan: Session Stream Overload Isolation

## Overview

Contain a single high-volume ACP session at three independent boundaries while
keeping the final transcript exact. Coalesce adjacent assistant/reasoning
chunks before the orchestrator performs read/append/write work, replace queued
full-message notifications instead of accumulating stale versions, and batch
browser store updates once per animation frame. Stabilize Office multi-session
membership so task refetches do not replay snapshots. Prove the contract with a
deterministic noisy mock-agent scenario on desktop and mobile.

This extends the implemented bounded task-status delivery design. It does not
change task summaries, message persistence ownership, or the reserved
correlated-response queue.

## Incident baseline

- Task `69637afd-c193-4197-aa49-348ac2b0cfb3`, session
  `89f7b247-cfb3-4a4b-8147-8826e13f1630`.
- The recorded launcher was `opencode-acp` using `opencode-go/kimi-k3`; the
  logs do not identify this execution as Grok.
- 28,967 unique reasoning-stream events and 31,314 total stream events over
  about 41 minutes, peaking at 63 reasoning events per second.
- Every chunk crossed lifecycle, orchestrator, message persistence, gateway,
  JSON, Zustand, and React boundaries.
- Five `session.message.updated` frames were dropped when the 256-frame
  notification queue filled. No control-queue overflow or backend crash was
  recorded.
- Equivalent Office task refetches caused repeated unchanged session
  unsubscribe/subscribe/snapshot cycles, further amplifying traffic.

## Architecture

### Lossless lifecycle coalescing

Add an execution-owned coalescer in
`apps/backend/internal/agent/runtime/lifecycle`. It accepts already-normalized
assistant/reasoning chunks after legacy newline buffering and protocol ID to
Kandev ID mapping. The first non-empty chunk for a record publishes immediately with
`IsAppend=false`. Adjacent later chunks append to one pending segment and flush
at the 100 ms cadence with `IsAppend=true`.

Keep ordering explicit. Before a tool call/result, permission request,
completion, error, stream disconnect, prompt-generation reset, execution
replacement, or manager shutdown, synchronously detach and publish pending
segments before publishing the boundary. Timer callbacks detach under the
execution mutex and publish outside it. Teardown stops timers and makes repeat
flushes idempotent.

The coalescer operates before `handleThinkingStreamingEvent` /
`handleMessageStreamingEvent`, so the existing task service naturally performs
fewer `GetMessage`, concatenation, `UpdateMessage`, and full-message event
publications. It does not change stored schemas or final message shapes.

### Session-isolated notification scheduling

Replace the client's single best-effort notification channel with a scheduler
that retains three classes:

1. the existing control queue for correlated responses/errors;
2. a bounded semantic FIFO for non-replaceable notifications; and
3. bounded per-session queues for replaceable `session.message.updated`
   notifications.

The hub passes action and routing identity to the client instead of only raw
bytes. For a replaceable message update, derive the key from `session_id` and
`message_id`. If the key is already pending, replace its bytes without moving
its position. A later semantic event for the same session therefore remains
behind the newest message state. Drain active session queues round-robin and
interleave them with the semantic FIFO after bounded control priority.

Production capacities are named constants, but tests construct small queues.
On per-session pressure, evict or reject only queued replaceable entries for
that session. Never evict control or semantic notifications to admit a stream
replacement. Record content-free counters/log fields for action, client,
session, replacement, eviction, and queue depth.

### Frontend batching and stable membership

Extract a small replaceable-message scheduler beside
`lib/ws/handlers/messages.ts`. During one animation frame it retains only the
newest full payload per `(session_id, message_id)` and then invokes
`updateMessage` once per changed key. Added/deleted messages and turn-settle
events act as barriers: flush or cancel a pending replacement as appropriate so
wire order cannot revive deleted state. Clear scheduled work when the WS router
is disposed or replaced.

In the Office task detail live-sync hook, derive a stable, sorted session-ID
membership key. Refetching equal session IDs or replacing the task object must
not rerun subscription cleanup/setup. A genuine membership change applies only
the added/removed IDs. The surface remains an intentional multi-session detail
view; this task removes churn rather than changing its product scope.

### Observability and reconciliation

Lifecycle and gateway logs/metrics must identify pressure without including
message content. Existing session snapshot/reconnect and turn-settle behavior
remain the reconciliation path; no new user-facing overload banner or protocol
action is required.

## Tests

### Backend

- Use `testing/synctest` around the lifecycle coalescer. Assert immediate first
  publication, one append per 100 ms window, exact 30,000-chunk concatenation,
  independent assistant/reasoning IDs, boundary order, disconnect/shutdown
  flush, and no timer/goroutine leak.
- Assert the orchestrator/task-service integration sees the reduced event count
  and stores exact final assistant and thinking content.
- Construct gateway schedulers with tiny capacities. Assert replacement in
  place, bounded per-session occupancy, round-robin progress, semantic barrier
  order, control priority, noisy-session-only eviction, metrics, close safety,
  and zero-value test-client compatibility.

### Frontend

- Feed hundreds of `session.message.updated` payloads for one message in a
  single fake animation frame. Assert one store update with the newest complete
  payload, while different message keys each update once.
- Cover add/delete/turn barriers, handler teardown, and separate sessions.
- Rerender Office live sync with new task/session object references but the
  same IDs and assert no subscription churn; then change membership and assert
  only the delta is subscribed/unsubscribed.

## E2E tests

Add a deterministic mock-agent command that emits a configurable reasoning
burst without sleeping or embedding thousands of commands in the prompt. A
new overload-isolation Playwright spec opens the noisy session and captures
gateway frames while interacting with a second session.

- Desktop Chromium: emit a representative burst, navigate to the quiet task,
  submit a message, and assert navigation/model controls/responding message
  remain usable before the noisy run settles.
- Assert server-side burst count was actually produced, the final persisted
  reasoning text is exact, received replacement frames are materially below
  source chunks, no control response is lost, and the quiet session progresses.
- Mobile Chrome: repeat the navigation/quiet-session progress assertion through
  the existing native task-switcher sheet and assert no horizontal overflow.
- Attach source-chunk, lifecycle-publication, gateway replacement/eviction,
  received-frame, byte, and latency totals. Counts are diagnostics except for
  the hard boundedness, exact-content, and cross-session-progress assertions.

## Mobile design contract

- **Desktop outcome / mobile entry:** both remain responsive while another
  session streams. Desktop uses the existing task/sidebar navigation; phone
  uses the existing task-switcher sheet.
- **Nearest exemplar:** keep
  `apps/web/components/task/mobile/session-task-switcher-sheet.tsx` and shared
  task/session stores unchanged in layout and navigation ownership.
- **Hierarchy / interaction:** no new control, gesture, dialog, or route.
- **Presentation / scroll:** the repair changes transport/state cadence only;
  existing sheet scrolling, safe areas, and viewport bounds remain intact.
- **Shared state:** desktop and mobile consume the same batched message and
  connection state.
- **Mobile proof:** run the overload scenario under `mobile-chrome`, switch to a
  quiet task, verify progress, and assert `scrollWidth <= clientWidth`.

## Implementation waves

Implementation is sequential in the user-started session because Tasks 02 and
03 consume the delivery semantics established by the preceding task. These
task files do not authorize subagent work.

- [x] [Task 01: Coalesce agent stream ingress](task-01-coalesce-agent-stream-ingress.md)
- [x] [Task 02: Isolate replaceable session delivery](task-02-isolate-replaceable-session-delivery.md)
- [x] [Task 03: Bound frontend stream work](task-03-bound-frontend-stream-work.md)
- [x] [Task 04: Prove overload containment](task-04-prove-overload-containment.md)

## Verification

All implementation waves are complete. The final verification evidence is:

- `go test -race ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/server/process ./internal/agent/runtime/lifecycle ./internal/gateway/websocket ./cmd/mock-agent` — passed (2,633 tests).
- `make -C apps/backend test` — passed on the final run.
- `rtk proxy golangci-lint run ./...` — passed (0 issues).
- Focused frontend Vitest — passed (7 tests); web typecheck and lint — passed.
- Desktop Chromium overload E2E — passed (1 test, 13.4s).
- Mobile Chrome overload E2E — passed (1 test, 8.3s).

The E2E evidence confirms exact persisted reasoning, bounded noisy-session
frames, quiet-session progress, and desktop/mobile viewport safety. The ACP
and process handoff now uses cancellation-aware per-agent backpressure so
normalized stream events are not silently discarded before lifecycle
coalescing; shutdown cancellation still releases a blocked noisy session.

```bash
cd apps/backend && go test ./internal/agent/runtime/lifecycle/... \
  ./internal/orchestrator/... ./internal/task/service/...
cd apps/backend && go test ./internal/gateway/websocket/...
make -C apps/backend test
make -C apps/backend lint
cd apps/web && pnpm exec vitest run \
  lib/ws/handlers/messages.test.ts \
  'app/office/tasks/[id]/use-session-live-sync.test.tsx'
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run lint
cd apps/web && pnpm e2e:run tests/session/session-stream-overload-isolation.spec.ts \
  -- --project=chromium
cd apps/web && pnpm e2e:run tests/session/session-stream-overload-isolation.spec.ts \
  -- --project=mobile-chrome
```

## Risks

- Timer callbacks racing completion could duplicate or reorder content. Detach
  pending buffers atomically, publish outside locks, and make boundary flushes
  idempotent.
- Protocol message IDs may interleave. Coalesce only adjacent chunks per typed
  Kandev record and retain the first-seen queue position.
- A latest-wins gateway queue can reorder a message update past turn completion
  if replacement positions are not stable. Preserve per-session enqueue order
  and test semantic barriers.
- Strict control/semantic priority could starve replacement streams. Use
  bounded priority bursts plus round-robin session scheduling.
- Animation-frame batching can be heavily throttled in background tabs. Keep
  only complete latest state and rely on selected-session reconciliation when
  the tab resumes.
- A quiet E2E producer would yield a false pass. Expose and assert the mock
  burst's produced count before checking isolation.

## Out of scope

- Truncating stored assistant or reasoning content.
- Automatically stopping an agent because it streams heavily.
- Splitting sessions across multiple WebSocket connections.
- Changing task-summary fields, task-switcher layout, or message schema.
- Adding a user-facing overload state or runtime feature flag.

## Documentation impact

The behavioral contract is added to
`docs/specs/platform/requirements/bounded-task-status-delivery.md` and the shared boundary is
recorded in
`docs/decisions/2026-08-02-isolate-replaceable-session-stream-traffic.md`.
No public docs change is expected because commands, settings, public APIs, and
visible interaction patterns do not change.
