---
created: 2026-10-26
status: in_progress
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005
  - REQ-PLATFORM-DURABLE-AGENT-SEGMENTATION-001
system_design:
  - ../../specs/platform/system-design/durable-agent-delivery.md
  - ../../specs/platform/system-design/durable-agent-stream-processing.md
legacy_specs: []
---

# Implementation Plan: Durable agent text segmentation

## Overview

Separate ID-less assistant text after top-level tool calls while preserving
chunk coalescing and deterministic durable projection. Implement one vertical
slice: persist the boundary in the backend delivery cursor, apply it inside
canonical SQL projection, and verify the transcript outcome through lifecycle,
repository, migration, and browser coverage.

The source baseline is current main at `7fb28fc022d` (PR #4804). The package
addresses issue #4788. Source tracing confirms the failure; no live runtime
reproduction was performed during planning.

## Scope

### In scope

- In durable v1, collect ID-less text chunks into one message per segment and
  begin a new message after each top-level tool call.
- Preserve boundaries across duplicate receipt, batching, projection replay,
  reconnect, partial projection, and backend or agentctl restart.
- Preserve native protocol IDs, reasoning/message type separation, turn and
  submission ownership, and the legacy delivery path.
- Migrate existing cursor rows without rewriting projected transcript rows.

### Out of scope

- Changes to ACP or agentctl wire or journal formats.
- Splitting messages at nested subagent calls or tool updates.
- Changing text chunking, reasoning grouping, chat layout, or mobile controls.
- Rewriting historical projected messages or changing old inbox transport
  identity.
- Broad durable delivery or provider behavior changes unrelated to this
  boundary.

## Source contracts and evidence

The owning contract is Platform durable delivery because SQL projection owns
canonical messages and projected cursors. The active design says projection
commits message changes with cursor advancement. The requirement lacked an
explicit segmentation and replay contract, so this package adds ACs under the
existing idempotent projection requirement.

| Evidence | Consequence |
| --- | --- |
| `durable_delivery_stream.go:canonicalAgentMessageID` scopes ID-less chunks to turn, submission, or prompt identity; the scope does not change at a tool call. | Chunks before and after a tool call reuse the canonical ID. |
| `stream_event_processor.go:handleEvent` flushes a compatible chunk batch before processing a non-batchable tool call. | Batch flushing alone cannot split the message row. |
| `agent_delivery_projection.go:persistCanonicalAgentMessageTx` appends to an existing `CanonicalMessageID`. | The durable projector merges post-tool text into the pre-tool row. |
| `manager_events.go:handleToolCallEvent` and `manager_streaming.go:flushMessageBuffer` split only on a top-level tool start and clear the current message ID; nested tools do not split. | Preserve the legacy boundary rule, including multi-chunk segments. |
| `agent_delivery.go:ProjectAgentDeliveryEvent` commits projection and cursor updates together; `AgentDeliveryCursor` is persisted in SQL. | Store the boundary with the authoritative projected cursor. |
| `git show v0.97.0:apps/backend/internal/agent/runtime/lifecycle/manager_events.go` shows the pre-durable path flushing on top-level tool starts. | The regression is in durable canonical identity, not the intended chat behavior. |

## Technical approach

### Durable boundary and projection

- Add `message_segment_boundary_sequence` to the SQL delivery cursor and its
  typed repository model.
- On projection of a top-level `tool_call` (`ParentToolCallID == ""`), set the
  boundary sequence to that event's durable sequence in the same transaction
  as effects and projected cursor advancement.
- In canonical message projection, derive an ID-less assistant text segment
  identity from the existing canonical ID plus the cursor boundary. Keep the
  identity unchanged when the boundary is zero. Do not apply the suffix to
  native `ProtocolMessageID` values or reasoning events.
- Persist each resolved canonical ID in the inbox event's reconstructable
  lifecycle metadata in the projection transaction. Derive it from a stable
  base identity plus the committed boundary, never from an already-resolved
  ID. Do not mutate caller-owned event payloads before commit. Return the
  exact IDs committed by SQL, including from already-projected duplicate paths.
  Keep the committed ID in a dedicated inbox column so payload pruning does
  not erase duplicate identity.
- Keep tool starts as non-batchable barriers. Compatible text chunks following
  one committed boundary continue to batch and append to one row.
- Keep the normal `ProjectAgentDeliveryEvent` event order and transaction
  idempotency. A rollback must leave the boundary, message effect, stored ID,
  and projected cursor unchanged.

### Upgrade and retained-event compatibility

- Add a nullable boundary cursor column for existing SQLite and PostgreSQL
  stores; new streams start at zero. Initialize a legacy NULL boundary once in
  the first projection transaction from the then-current `projected_sequence`
  (or the current event sequence when that first event is a top-level tool
  call). Leave non-NULL boundaries untouched across reopen, owner/generation
  changes, and cursor updates. This conservative baseline prevents appending to
  historical rows whose boundary evidence may have been pruned.
- Do not mutate existing `task_session_messages` rows. During ordered replay,
  an unprojected event is projected using the committed boundary then in force;
  lifecycle metadata is resolved atomically before the cursor advances.
- Update every top-level tool projection route: normal
  `ProjectAgentDeliveryEvent`, retained replay, and canonical shell-start
  projection. Any mixed batch must advance boundaries in event sequence order.
  Nested tool calls and tool updates do not advance the boundary.
- Keep canonical identity out of transport event identity checks, as it is
  backend-reconstructed lifecycle metadata. A duplicate remains the same
  transport event even when its resolved canonical ID is read from SQL.

### Compatibility matrix

| Event path | Segmentation behavior | Evidence |
| --- | --- | --- |
| Durable v1, ID-less assistant text | New stable row after each top-level tool start; chunks within a segment share a row. | Lifecycle and SQLite projector regression tests; Chromium E2E. |
| Durable v1, native protocol message ID | Keep the native-scoped canonical ID. | Identity regression test. |
| Durable v1, reasoning | Keep the existing separate `thinking` message identity and type. | Reasoning separation regression test. |
| Durable v1, nested tool or tool update | Do not advance the segment boundary. | Parent-tool and tool-update repository regressions. |
| Identified legacy delivery | Keep existing `flushMessageBuffer` behavior and avoid durable cursor lookups. | Existing legacy lifecycle regression suite. |

## Tests

| Acceptance criterion | Regression evidence |
| --- | --- |
| AC-PLATFORM-DURABLE-AGENT-SEGMENTATION-001.1 | `apps/backend/internal/task/repository/sqlite/agent_delivery_segments_test.go`: `TestCanonicalDeliveryTextSegmentsSplitAtTopLevelToolStarts`, `TestCanonicalDeliveryTextSegmentsRespectMixedBatchOrder`, `TestCanonicalDeliveryTextSegmentsIgnoreNestedToolsAndUpdates`, `TestCanonicalDeliveryTextSegmentsPreserveNativeAndReasoningIdentity`, and `TestCanonicalDeliveryTextSegmentsSplitAtCanonicalShellStart`. Lifecycle `TestCanonicalAgentMessageIDPreservesNativeAndReasoningIdentity` verifies stable base, native protocol, and reasoning identity rules. |
| AC-PLATFORM-DURABLE-AGENT-SEGMENTATION-001.2 | SQLite `TestCanonicalDeliveryTextSegmentRollbackRetryAndDuplicate` injects projection failure, retries, then duplicates after a later boundary with a pruned payload. `TestCanonicalDeliveryTextSegmentBoundarySurvivesDatabaseReopen` closes and reopens SQLite, replays the prior event, and continues the same segment. `TestCanonicalDeliverySegmentBoundaryMigration` covers nonzero projected cursor, pruned history, first-projection initialization, and reopen; PostgreSQL runs the same migration path in `TestCanonicalDeliverySegmentBoundaryPostgres` when `KANDEV_TEST_POSTGRES_DSN` is configured. The browser test `durable-agent-message-segments.spec.ts` disconnects a live durable-v1 stream, checks text/tool/text ordering and chunk coalescing, then reloads and checks again. |

## E2E tests

`apps/web/e2e/tests/chat/durable-agent-message-segments.spec.ts` in the
`chromium` project uses the mock agent to ingest through durable-v1, then
disconnects the live stream through the durable-only fixture endpoint and
asserts the disconnect succeeded. Send multiple ID-less text chunks before
and after a top-level tool call/result. Assert chronological text/tool/text
rows, with chunks coalesced on each side; one tool call renders as a standalone
row, so do not require a group. Reload and assert the same rows and content.
This is a transcript data change with no viewport-dependent interaction or
layout change, so no mobile-only test is required; the same stored message
records feed the shared chat transcript on phone.

## Work orders

- [x] [Task 01: Persist and project durable text segment boundaries](task-01-persist-text-segment-boundaries.md) (done)

## Verification results

The final focused race suite passed for the lifecycle identity tests and SQLite
segment, rollback/retry, duplicate, reopen, and migration regressions. The
Chromium E2E passed against durable-v1 ingestion and verified text/tool/text
ordering, chunk coalescing, reconnect, and reload. SQLite migration, SQL guard,
store conformance, Go lint, documentation validation, spec lint, and
`git diff --check` passed.

The PostgreSQL migration test was run with `-v` and skipped because
`KANDEV_TEST_POSTGRES_DSN` is not configured in this environment. A controlled
broad race run passed lifecycle, agentctl, and orchestrator packages; the SQLite
package reached its 20-minute timeout during core schema initialization under
shared-runner I/O contention, without an assertion failure. The default-timeout
broad run also timed out in lifecycle, SQLite, and orchestrator. The focused
SQLite and E2E regressions passed independently.

## Risks

- Existing projected history may have pruned the tool event that established
  the current segment. The migration's projected-cursor baseline prevents
  appending to those rows but can start one fresh segment after upgrade.
- Canonical message IDs must be resolved and stored atomically with message
  projection and cursor advancement. Returning the stored resolved ID is
  necessary so notification cannot refer to a different row.
- Both SQL dialects and schema-upgrade fixtures must preserve the same cursor
  checkpoint behavior.
