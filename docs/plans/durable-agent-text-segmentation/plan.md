---
created: 2026-10-26
status: done
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
- Persist each resolved canonical ID in the authoritative
  `agent_delivery_inbox.canonical_message_id` column and in reconstructable
  payload metadata in the projection transaction. Derive it from a stable base
  identity plus the committed boundary, never from an already-resolved ID. Do
  not mutate caller-owned event payloads before commit. Return the exact IDs
  committed by SQL, including from already-projected duplicate paths. Payload
  compaction preserves the dedicated column; it is the only duplicate identity
  source after pruning.
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
- Keep canonical identity out of transport event identity and payload-hash
  checks, as it is backend-reconstructed projection metadata. Duplicate
  admission preserves the stored column value even when incoming canonical
  metadata differs.
- For an older projected row with an empty column, use the stored payload ID
  only while that payload is retained. A pruned payload and empty column return
  no ID; do not derive one from incoming metadata or the current boundary, and
  do not notify or rewrite a transcript row without a committed ID. Whole-row
  receipt retirement deletes the ID with the receipt and keeps existing
  expired-replay rejection semantics.

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
| AC-PLATFORM-DURABLE-AGENT-SEGMENTATION-001.2 | SQLite `TestCanonicalDeliveryTextSegmentRollbackRetryAndDuplicate` injects projection failure and verifies retry/duplicate identity. `TestCanonicalDeliveryRetentionPruneRestartAndDuplicatePreserveCommittedID` uses the retention worker to prune, closes and reopens SQLite, advances a later boundary, then verifies duplicate admission and projection return the exact stored ID. `TestCanonicalDeliveryPreUpgradePrunedReceiptDoesNotInventIdentity` verifies an empty migrated column and pruned payload do not infer an ID or create a transcript row. `TestCanonicalProjectionDoesNotNotifyWithoutCommittedMessageID` and `TestCanonicalProjectionBatchSkipsRowsWithoutCommittedMessageID` verify single and batch lifecycle routes suppress notifications without a committed ID. Upgrade coverage asserts the additive column in SQLite and PostgreSQL; PostgreSQL tests run when `KANDEV_TEST_POSTGRES_DSN` is configured. Whole-row expiry remains covered by `TestDeliveryRetentionIntegration`. The browser test `durable-agent-message-segments.spec.ts` disconnects a live durable-v1 stream, checks text/tool/text ordering and chunk coalescing, then reloads and checks again. |

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

- [x] [Task 01: Persist and project durable text segment boundaries](task-01-persist-text-segment-boundaries.md)

## Verification results

Task 01 implementation is complete. The parent takeover verification below
records full SQLite CI coverage and the remaining CI corrections.
Change-aware lifecycle and SQLite race tests passed, including real retention
pruning and reopen, duplicate replay after a later boundary, old pruned-receipt
fallback, schema upgrade, rollback/retry, and whole-row expiry. The
migration-order regression proves the columns are absent after schema
initialization and added by `runMigrations()`; it failed before and passed
after this fix. The implicated broad-timeout test passed alone again after the
migration change (2.103s). The Chromium durable-v1 E2E passed 1/1 after
rebuilding backend and web assets. SQL guard, store conformance (84.7s after
the migration change), changed-Go lint, documentation validation, spec lint,
and `git diff --check` passed.
The child initially skipped PostgreSQL migration and fallback subtests. The
parent takeover section records their later execution against PostgreSQL 16.

Both broad race attempts failed overall. The default-timeout run timed out in
lifecycle, SQLite, and orchestrator while agentctl passed. The controlled
`-timeout 20m` run passed lifecycle, agentctl, and orchestrator but timed out in
SQLite during core schema initialization. Available timeout output identifies
`TestCompleteActiveClarificationBundlePersistsRecoverableDeliveryIntent` in
the `newRepoForSessionTests` initialization path. The full goroutine stacks and
lock/connection owners are unavailable because the original output was
truncated and not saved. A recovery audit searched all 2,534 available session
records and task-specific `/tmp` artifacts; no broad-run stack was present. The
only retained `/tmp/kandev-4788-base-test.log` and
`/tmp/kandev-4788-implementation-test.log` files are 69-byte isolated comparison
outputs (2.502s on base, 2.558s on the initial implementation). The blocked
goroutine chain and lock/connection owners therefore cannot be named. The exact
test was run sequentially, with no concurrent
Go tests, using matching race, package, parallelism, and timeout settings. It
passed on unchanged base `7fb28fc022d5320922608bf57cc9fd9351a8b8da` (2.841s)
and implementation `8bafa71d33be8d20029699202605f136dae41bc3` (2.103s). The
isolated test does not reproduce or explain the broad timeout.
The parent takeover section supersedes those earlier gaps with complete SQLite
CI cohorts and focused PostgreSQL verification. See the
[Task 01 results](task-01-persist-text-segment-boundaries.md#results) for exact
commands and outcomes.

## Risks

- Existing projected history may have pruned the tool event that established
  the current segment. The migration's projected-cursor baseline prevents
  appending to those rows but can start one fresh segment after upgrade.
- Canonical message IDs must be resolved and stored atomically with message
  projection and cursor advancement. Returning the stored resolved ID is
  necessary so notification cannot refer to a different row.
- Both SQL dialects and schema-upgrade fixtures must preserve the same cursor
  checkpoint behavior.


## Parent takeover verification (2026-10-26)

The parent session owns PR #4819 from head `e9a20fbcb650` onward.
Backend CI run `40385254227` passed all three complete SQLite repository
cohorts and all four ordinary backend shards on that head. The earlier local
SQLite timeout remains historical evidence with an unknown cause. Complete
repository CI coverage now supplements those local attempts.

The PostgreSQL CI cohort exposed two migration-regression queries that used
SQLite placeholders without `Rebind`. Both queries now use the database dialect.
The failure reproduced locally before correction with SQLSTATE 42601.
A disposable local PostgreSQL 16 instance then ran the segment and migration
regressions, including previously skipped PostgreSQL cases:

```bash
(cd apps/backend && go test -trimpath -race -parallel 4 ./internal/task/repository/sqlite -run 'TestCanonicalDelivery(TextSegments|TextSegment|Segment)|TestAgentDeliverySegmentationColumnsAreAddedByRunMigrations|TestAgentDeliveryRetentionMigratesPreRetentionSchema' -count=1 -timeout 5m)
```

With `KANDEV_TEST_POSTGRES_DSN` set to the local instance, this passed in 15.068s.
The DSN is runtime-only and contains no production credentials.

The Windows native CI job also stalled in the supervised-descriptor test while
closing a synchronous pipe reader with its writer still open. The fixture now
releases the writer first, matching supervised child exit. Cleanup uses the
same order so a failed assertion cannot hang cleanup. No launcher behavior changes.

```bash
(cd apps/backend && go test -trimpath -race ./internal/agent/runtime/agentctl/launcher -run 'TestPipeOutput' -count=1 -timeout 2m)
```

This passed locally in 3.174s. The Windows rerun remains a CI check.
The PR was marked ready after pushing these corrections, as the user requested.


## Post-hold PR fixup (2026-10-26)

After the requested 15-minute hold, the current-head CodeRabbit review identified
an E2E scheduling gap and stale plan wording. The browser fixture now blocks
its tool on an `e2e:await_file` marker. It asserts no tool row exists before the
forced disconnect, then releases the marker after the disconnect call. Backend
transcript polling precedes rendered-row waits and chronological DOM checks.
The updated managed Chromium test rebuilt production assets and passed 1/1
in 10.9s. Targeted ESLint passed. No product behavior changed in this fixup.

## CI fixture follow-up (2026-10-26)

Backend CI run `40401492531` passed at head `6d3eb21a9624`, including
PostgreSQL, Windows native execution, and all three SQLite cohorts.
E2E run `40401492777` first exposed three fixture failures on that head:

- The mobile sidebar recovery fixture now supplies `Retry-After` so automatic
  retry cannot replace the manual retry control. It verifies retained task rows
  while the manual refresh response is held.
- Concurrent PR-watcher agents now use isolated worktree executors. Their
  offline remote publishes branch and PR-head refs before the watcher starts.
- The selected-filter fixture removes repositories left by earlier tests.
  Replaying repository-set creation followed by filter ordering reproduced
  the duplicate-label failure before this correction.

With retries disabled, the mobile recovery spec passed six checks over three
runs. The watcher and repository-set/filter sequence passed all seven checks.
Two more runs each of the watcher and phone filter tests passed four checks.
Targeted ESLint, Prettier, and diff checks passed. These changes affect test
fixtures only.

The final shard exposed a fourth fixture race: touch-target geometry was
measured during the navigation drawer animation. The test now waits for finite
animations before measuring. The original 44px minimum remains unchanged.
The three-spec sequence passed all ten checks with retries disabled, two CPUs,
and 4 GiB of memory. See the work order for diagnostic limits and results.

## CI follow-up: live workflow provenance (2026-10-27)

CI at `e1d3d6f120fb` exposed a separate frontend defect: workflow sync provenance
was discarded during HTTP, boot, and WebSocket mapping. Settings could therefore
keep a synced workflow editable when a no-change sync did not reload the page.
The frontend now carries the existing `source` and `source_path` fields into
settings, preserves local field drafts, and releases ownership on sync removal.

Six focused regressions failed before correction; all 53 focused tests passed
afterward. Four GitLab browser cases and 22 lifecycle/approval/pagination cases
passed with retries disabled. The work order records the state-boundary fixture
corrections and the separate walkthrough-generation timeout evidence.

## CI follow-up: clock, agent, and picker fixtures (2026-10-27)

CI run `40428275527` at `0452fcdf57de` reported three more browser flakes.
Search controls now install an explicit clock origin and pause at a later time.
Mobile deletion selects the mock agent rather than the first registry family.
Branch creation waits for repository-picker focus restoration before opening
the executor picker. Existing product assertions and retry policy are unchanged.

Local context replays passed 48 checks before correction, so they did not
reproduce those CI failures. After correction, the same project contexts plus
the desktop search controls passed 51 checks with retries disabled, one worker,
two CPUs, and 4 GiB memory. Helper unit tests, targeted lint, and formatting
passed. The work order records the CI evidence and diagnostic limits.

A late fourth failure exposed a product race in task-route enrichment.
Optional hydration replayed an older task/session selection and session list
after the task shell had accepted a prepared session. The correction restores
`AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.7` and `.9`: optional enrichment does
not republish navigation selection or an already-resolved session collection.
The existing failed-list recovery and full SSR hydration paths remain intact.
Two regressions failed before the correction. All 72 focused navigation tests,
11 rebuilt desktop browser checks, and two phone checks passed afterward.
The replacement head still requires CI and review verification.

## Conflict resolution and browser synchronization (2026-10-27)

Merge commit `31b9bdbc35ab` resolves the profile-deletion fixture conflict using
main's exact seeded profile. The resolved merge and newer main `d549fa694b7d`
passed focused compatibility checks recorded in the work order.

Its CI run exposed two browser flakes during terminal creation and delayed
capacity recovery. The test setup now waits for picker animation completion,
the correlated terminal-create reply, and the sixth capacity waiting notice.
The latter observer is armed immediately before the final clock advance.
Existing assertions and retry policy remain unchanged.

A temporary backend delay reproduced the old capacity assertion and passed
with the corrected observer. Clean final browser runs passed 50/50 cases in
the CI runtime image with one worker, two CPUs, 4 GiB memory, and retries
disabled. Synchronization-helper tests passed 32/32; typecheck, targeted lint,
and formatting passed. Replacement-head CI and review verification remains a
delivery gate. The work order records the reproduction limits and full audit.
