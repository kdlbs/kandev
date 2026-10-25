---
created: 2026-09-09
status: done
requirements:
  - REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001
system_design:
  - ../../specs/platform/system-design/provider-error-recovery.md
  - ../../specs/platform/system-design/provider-response-attempt-recovery.md
legacy_specs: []
---

# Implementation Plan: Provider Retry Output Retraction

## Overview

Translate Codex's structured internal-retry metadata into a provider-neutral
response-attempt reset, map that boundary to the provisional Kandev streaming
records owned by lifecycle, and retire those records through the task service.
The order follows the runtime flow so each work order can prove its contract
before the next layer consumes it. A controlled mock-agent scenario then proves
the durable and live desktop and mobile result end to end.

## Scope

### In scope

- Exact, prompt-generation-scoped detection of Codex response-stream retry
  metadata.
- Provider-neutral reset delivery through agentctl and lifecycle.
- Attempt-local assistant and thinking record ownership and history cleanup.
- Durable task-message deletion and existing WebSocket removal projection.
- Desktop and mobile proof that the abandoned response disappears live and
  stays absent after reload.

### Out of scope

- Similarity-based message deduplication.
- A new Kandev retry loop, provider fallback, or change to replay-safety policy.
- Truncating a message record created before the latest top-level effect
  boundary.
- Message schema changes, new chat components, layout changes, or localized
  copy.
- Repairing transcript rows left by historical incidents.

## Technical approach

### ACP retry-boundary projection

- Add `EventTypeResponseAttemptReset` in
  `apps/backend/internal/agentctl/types/streams/agent.go`.
- Extend `acpDialect` in
  `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect.go` with
  an optional structured reset recognizer.
- Match `codex.error.willRetry=true` plus the nested
  `responseStreamDisconnected` object in `dialect_codex.go`. Keep raw
  diagnostic text out of the normalized event.
- Emit the generation-bearing reset from the ordered notification worker in
  `adapter_updates.go` before later provider output while preserving ordinary
  session-info fields.

### Lifecycle attempt records

- Add an ordered response-attempt record list to `AgentExecution` in
  `types.go`. Register every newly allocated assistant or thinking message ID
  in `manager_streaming.go`.
- Commit the list at a top-level tool boundary while retaining existing
  protocol correlation. Clear it with the existing prompt and execution reset
  paths.
- Add `handleResponseAttemptReset` in `manager_events.go`. Fence it to the
  current prompt generation, flush the coalescer, detach eligible IDs, clear
  pending buffers and fallback history, and publish
  `RetractedMessageIDs` through `event_types.go` and `events.go`.

### Durable transcript cleanup

- Add a narrow `StreamingMessageRetractionService` to
  `internal/orchestrator/service.go` and wire the task service in
  `internal/backendapp/orchestrator.go`.
- Handle `response_attempt_reset` in
  `event_handlers_streaming.go` under the existing per-session stream guard.
  Delete each detached ID through `task/service.Service.DeleteMessage`, continue
  after individual failures, and keep the provider turn running.
- Reuse the existing `session.message.deleted` gateway and frontend handler.
  No frontend production file changes are expected.

### Controlled end-to-end fixture

- Add mock-agent emitters and a `/e2e:response-retry` scenario that stream
  explicit-ID assistant and thinking rows, emit a test-only structured reset,
  and then stream a replacement under new IDs.
- Add a mock-only ACP dialect recognizer. The mock marker remains unavailable
  to production agent dialects.
- Exercise the scenario in desktop and mobile task chat, including live removal,
  durable API state, and reload.

## Tests

- `AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.26`: new
  `dialect_codex_retry_test.go` cases cover the exact captured metadata,
  malformed and false flags, adapter scope, prompt-generation scope, and event
  ordering.
- `AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.27`: new lifecycle and orchestrator
  tests cover ordered assistant/thinking retraction, real task-service deletion,
  and `session.message.deleted` publication.
- `AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.28`: lifecycle tests cover an empty
  second reset, committed rows, protocol-ID reuse, stale generations, pending
  buffers, fallback history, and unchanged prompt replay evidence.
- Existing `apps/web/lib/ws/handlers/messages.test.ts` remains the focused proof
  that deletion flushes queued updates before removing a row.

## E2E tests

- `AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.27`: add
  `apps/web/e2e/tests/session/provider-response-retry.spec.ts` to the `chromium`
  project. It asserts live removal, the durable message list, and absence after
  reload.
- `AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.27`: add
  `apps/web/e2e/tests/session/mobile-provider-response-retry.spec.ts` to the
  `mobile-chrome` project and assert the same task-chat outcome.
- The change reuses the current task-chat composition and shared message store.
  The nearest recovery exemplars are `transient-retry.spec.ts` and
  `mobile-transient-retry.spec.ts`; no navigation, scroll, touch, safe-area, or
  viewport behavior changes.

## Work orders

- [x] [Task 01: Project Response-Attempt Resets](task-01-project-response-attempt-resets.md)
- [x] [Task 02: Map Provisional Stream Records](task-02-map-provisional-stream-records.md)
- [x] [Task 03: Apply Transcript Retractions](task-03-apply-transcript-retractions.md)
- [x] [Task 04: Prove Chat Retry Recovery](task-04-prove-chat-retry-recovery.md)

## Verification results

All task-defined checks passed:

- Codex and controlled-mock ACP reset projection under the race detector.
- Lifecycle attempt ownership, commit boundaries, stale-event fencing, and
  retraction publication under the race detector.
- Orchestrator durable deletion, continued cleanup after individual failures,
  existing deletion-event publication, and production composition under the
  race detector.
- Mock-agent retry sequence and the frontend message-deletion semantic barrier.
- Chromium desktop and mobile Chrome end-to-end flows, including transient-row
  visibility, live removal, durable API readback, and reload.
- Full backend lint and web TypeScript type checking.

## Risks

- A buffered streaming chunk can recreate an abandoned row after deletion if
  lifecycle publishes the reset before flushing the coalescer.
- A stale reset can delete successor output unless execution and prompt
  generation are both fenced.
- Deleting a record that predates a tool boundary can remove valid committed
  content when a provider reuses a protocol message ID.
- An individual task-store failure can leave one abandoned row even though the
  replacement turn continues; structured logs must preserve that diagnosis.
