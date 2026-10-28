---
id: "01-persist-text-segment-boundaries"
title: "Persist and project durable text segment boundaries"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005
  - REQ-PLATFORM-DURABLE-AGENT-SEGMENTATION-001
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.2
  - AC-PLATFORM-DURABLE-AGENT-SEGMENTATION-001.1
  - AC-PLATFORM-DURABLE-AGENT-SEGMENTATION-001.2
system_design:
  - ../../specs/platform/system-design/durable-agent-delivery.md
  - ../../specs/platform/system-design/durable-agent-stream-processing.md
---

# Task 01: Persist and project durable text segment boundaries

## Summary

Make top-level tool starts durable message boundaries for ID-less assistant text. Store the boundary with the SQL projected cursor, persist the authoritative committed canonical ID in the inbox atomically, and preserve existing projected history and legacy behavior.

## In scope

- Add and migrate the SQL cursor boundary and additive inbox canonical-ID column in SQLite and PostgreSQL.
- Advance it only when a top-level tool call projects.
- Resolve durable text segment IDs in canonical projection and return committed IDs to lifecycle notifications.
- Add lifecycle, repository, migration, restart, duplicate, and browser regressions.
- Resolve IDs from a stable base identity plus the committed boundary. Never re-hash an already-resolved ID or mutate caller-owned events before commit; duplicate projection returns the exact stored committed ID.
- Update every top-level tool projection route, including normal projection, retained replay, and canonical shell-start projection. Persist boundaries with the projected cursor in event sequence order, including mixed batches. Nested tools and tool updates remain non-boundaries.
- Initialize legacy cursor boundaries once without resetting valid values on reopen; cover nonzero projected cursors, pruned payloads, and owner/generation/cursor updates.
- Treat `canonical_message_id` as the authoritative committed projection ID. Payload compaction preserves it; whole-row retirement deletes it with the receipt and keeps expired-replay rejection.
- For old projected rows with an empty column, use a retained payload ID only when its payload is still available. A pruned payload yields no ID and no notification; do not infer one from incoming metadata or the current boundary.
- Keep projection-only canonical IDs out of transport identity/hash checks, and ensure duplicate admission preserves the stored column.
- Update plan results after implementation and synchronize the plan status.

## Out of scope

- Agentctl journal or wire protocol changes.
- Changes to native message IDs, reasoning grouping, nested tools, or legacy delivery.
- Rewriting existing projected transcript rows or changing chat layout.
- Unrelated durable delivery cleanup.

## Acceptance

- Text chunks before and after top-level tool starts project to separate assistant rows; multiple chunks within each segment coalesce. Nested tool calls and tool updates do not split rows.
- Duplicate delivery, batching, partial projection, replay, reconnect, and backend or agentctl restart retain the same committed message IDs. Existing projected messages remain unchanged, and legacy delivery remains unchanged.
- Projection results expose only committed IDs. When an old pruned receipt has no stored ID, duplicate projection leaves it unchanged and sends no notification. Payload pruning preserves the ID; whole-row expiration retains the existing typed replay rejection.
- SQLite and PostgreSQL schema upgrades initialize existing cursors once, including nonzero projected cursors and pruned payloads, and preserve the projection transaction invariant.
- Failure injection proves rollback leaves the stored base identity, boundary, message effect, and cursor unchanged; retry commits the same identity. A duplicate after a later boundary returns the original committed ID.

## Verification

Run from the repository root after implementation.

```bash
(cd apps/backend && go test -race -trimpath -p 1 -parallel 4 ./internal/agent/runtime/lifecycle -run 'TestCanonicalAgentMessageIDPreservesNativeAndReasoningIdentity|TestCanonicalProjectionDoesNotNotifyWithoutCommittedMessageID|TestCanonicalProjectionBatchSkipsRowsWithoutCommittedMessageID' -count=1)
(cd apps/backend && go test -race -trimpath -p 1 -parallel 4 ./internal/task/repository/sqlite -run 'TestCanonicalDeliveryTextSegments|TestCanonicalDeliveryTextSegment|TestCanonicalDeliverySegment|TestCanonicalDeliveryRetentionPruneRestartAndDuplicatePreserveCommittedID|TestCanonicalDeliveryPreUpgradePrunedReceiptDoesNotInventIdentity|TestDeliveryRetentionIntegration$|TestDeliveryRetentionSchemaUpgradePreservesPreRetentionEvidence' -count=1)
(cd apps/backend && go test -race -trimpath -p 1 -parallel 4 -v ./internal/task/repository/sqlite -run '^(TestDeliveryRetentionSchemaUpgradePreservesPreRetentionEvidence|TestCanonicalDeliveryPreUpgradePrunedReceiptDoesNotInventIdentity)$/^Postgres$' -count=1)
(cd apps/backend && go run -trimpath ./cmd/sqlguard ./internal)
(cd apps/backend && go test -race -trimpath -p 1 -parallel 4 ./internal/persistence/storeconformance -count=1)
(cd apps/web && pnpm e2e:run tests/chat/durable-agent-message-segments.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The focused E2E command must report discovery and execution of the intended Chromium test. It must ingest events through durable-v1 using the mock/retained-delivery fixture, assert durable mode by disconnecting the live stream successfully, verify multi-chunk text/tool/text row ordering, then reload and verify again. A single tool call renders as a standalone row; do not require a tool group. Record unavailable PostgreSQL-backed cases as an evidence gap; do not report them as passed. Run `bash scripts/lint-go-changed <changed-go-files>` for changed Go files. Run `pnpm install --frozen-lockfile` from `apps/` once if this worktree lacks dependencies before invoking pnpm.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/durable_delivery_stream.go`
- `apps/backend/internal/agent/runtime/lifecycle/streams.go`
- `apps/backend/internal/agent/runtime/lifecycle/stream_event_processor.go`
- `apps/backend/internal/agent/runtime/lifecycle/durable_delivery_stream_test.go`
- `apps/backend/internal/task/models/agent_delivery.go`
- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/task/repository/sqlite/agent_delivery.go`
- `apps/backend/internal/task/repository/sqlite/agent_delivery_projection.go`
- `apps/backend/internal/task/repository/sqlite/agent_delivery_retention_schema.go`
- `apps/backend/internal/task/repository/sqlite/agent_delivery_segments_test.go`
- `apps/web/e2e/tests/chat/durable-agent-message-segments.spec.ts`

## Dependencies

None.

## Risks

- Updating the SQL boundary without the message and projected cursor in the same transaction can make replay assign a different segment.
- Existing projected cursor values are a conservative upgrade boundary when older tool payloads are pruned; this can start one new message after upgrade but must not modify existing rows.
- Notification must use the canonical ID persisted by projection, including when an unprojected retained event predates this change.

## Parallelism

`sequential`

## Inputs

- [Plan and source evidence](plan.md).
- [Durable delivery requirements](../../specs/platform/requirements/durable-agent-delivery.md).
- [Durable text segmentation requirements](../../specs/platform/requirements/durable-agent-text-segmentation.md).
- [Durable delivery design](../../specs/platform/system-design/durable-agent-delivery.md).
- [Durable stream processing design](../../specs/platform/system-design/durable-agent-stream-processing.md).
- [Text segment identity ADR](../../decisions/2026-10-26-durable-agent-text-segment-identity.md).
- Existing lifecycle and SQLite projection tests.

## Results

At child delivery, implementation was complete but verification remained in
progress. The parent takeover evidence below supersedes those earlier gaps.
SQLite stores the top-level tool boundary beside the projected cursor and
commits it atomically with message effects. Canonical text IDs derive from a
stable base and committed boundary. The inbox `canonical_message_id` column
is the authoritative committed ID; payload compaction retains it, and
duplicates return the exact stored value. Fresh schemas include the columns;
idempotent SQLite/PostgreSQL `ADD COLUMN` migrations run in `runMigrations()`
after schema initialization. Existing transcript rows remain untouched.
Normal projection, retained replay, and canonical shell-start routes preserve
ordered boundaries.

Verification results:

- Change-aware race command `go test -race -trimpath -p 1 -parallel 4 ./internal/agent/runtime/lifecycle -run 'TestCanonicalAgentMessageIDPreservesNativeAndReasoningIdentity|TestCanonicalProjection' -count=1` passed (1.26s).
- The migration-order test `TestAgentDeliverySegmentationColumnsAreAddedByRunMigrations` failed before the implementation change because schema initialization added the columns early, then passed after moving the additive migrations to `runMigrations()`.
- After that fix, the focused race command `go test -race -trimpath -p 1 -parallel 4 -v ./internal/task/repository/sqlite -run 'TestCanonicalDeliveryTextSegments|TestCanonicalDeliveryTextSegment|TestCanonicalDeliverySegmentBoundaryMigration|TestCanonicalDeliverySegmentBoundaryPostgres|TestCanonicalDeliveryRetentionPruneRestartAndDuplicatePreserveCommittedID|TestCanonicalDeliveryPreUpgradePrunedReceiptDoesNotInventIdentity|TestDeliveryRetentionIntegration$|TestAgentDeliverySegmentationColumnsAreAddedByRunMigrations|TestDeliveryRetentionSchemaUpgradePreservesPreRetentionEvidence|TestDeliveryRetentionInterruptedMigrationResumes' -count=1` passed (15.0s). It covers SQLite segmentation, rollback/retry, real payload pruning/reopen/duplicate, pre-upgrade fallback, interrupted and schema upgrades, and whole-row expiry. PostgreSQL subtests skipped because `KANDEV_TEST_POSTGRES_DSN` is unset.
- The implicated test `go test -race -p 1 -parallel 4 -timeout 20m ./internal/task/repository/sqlite -run '^TestCompleteActiveClarificationBundlePersistsRecoverableDeliveryIntent$' -count=1` passed again after the migration-path change (2.103s).
- `pnpm e2e:run tests/chat/durable-agent-message-segments.spec.ts` passed 1/1 Chromium test (10.6s) after rebuilding backend and Vite assets. The test ingested through durable-v1, verified disconnect, text/tool/text order and multi-chunk coalescing, then reloaded and checked the transcript.
- `go run -trimpath ./cmd/sqlguard ./internal` passed. After the migration-path change, `go test -race -trimpath -p 1 -parallel 4 ./internal/persistence/storeconformance -count=1` passed (84.7s). Changed-Go lint reported 0 issues. `python3 scripts/list-docs.py validate` validated 404 decisions and 1741 specifications; `python3 scripts/lint-spec-files.py --all` and `git diff --check` passed.
- PostgreSQL schema-upgrade and fallback tests were attempted through the `Postgres` subtests and skipped because `KANDEV_TEST_POSTGRES_DSN` is unset. PostgreSQL behavior remains unverified locally.

Broad SQLite diagnostic, recorded separately from the focused passes:

- Default-timeout command `go test -race -p 1 -parallel 4 ./internal/agent/runtime/lifecycle ./internal/agent/runtime/agentctl ./internal/task/repository/sqlite ./internal/orchestrator -count=1` timed out in lifecycle, SQLite, and orchestrator; agentctl passed. This command failed overall.
- Controlled command `go test -race -p 1 -parallel 4 -timeout 20m ./internal/agent/runtime/lifecycle ./internal/agent/runtime/agentctl ./internal/task/repository/sqlite ./internal/orchestrator -count=1` passed lifecycle (402.4s), agentctl (6.9s), and orchestrator (561.1s), but timed out in SQLite at 20m during core schema initialization. This command failed overall.
- Available timeout output identifies `TestCompleteActiveClarificationBundlePersistsRecoverableDeliveryIntent` in the `newRepoForSessionTests` core schema initialization path. The full goroutine stacks and lock/connection owners are not available because the original output was truncated and not saved; the cause is unresolved.
- A recovery audit searched all 2,534 available session records and task-specific `/tmp` artifacts; no broad-run stack was present. The only retained `/tmp/kandev-4788-base-test.log` and `/tmp/kandev-4788-implementation-test.log` files are 69-byte outputs from the earlier isolated comparison (2.502s on base, 2.558s on the initial implementation). The blocked goroutine chain and lock/connection owners therefore cannot be named.
- The isolated comparison command `go test -race -p 1 -parallel 4 -timeout 20m ./internal/task/repository/sqlite -run '^TestCompleteActiveClarificationBundlePersistsRecoverableDeliveryIntent$' -count=1` passed sequentially with no concurrent Go tests on unchanged base `7fb28fc022d5320922608bf57cc9fd9351a8b8da` (2.841s) and implementation `8bafa71d33be8d20029699202605f136dae41bc3` (2.103s), using matching race, package, parallelism, and timeout settings. This exact test passes in isolation on both revisions; it does not reproduce or explain the broad SQLite timeout. Earlier comparisons on pre-migration-path implementation revisions also passed (2.502s base, 2.558s and 2.170s implementation).

The earlier incomplete verification is superseded by the parent takeover
evidence below: complete SQLite CI cohorts passed, and PostgreSQL cases now
execute locally with the corrected portable queries.


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

The final shard exposed a fourth fixture race in `mobile-quick-terminal.spec.ts`:
a launcher measured 43.999969px during navigation drawer entry. The test now
waits for finite drawer animations before measuring touch targets. The strict
44px minimum remains unchanged.

Five isolated runs and the ten-test preceding sequence passed before this
correction, so they did not reproduce the exact CI failure. A temporary geometry
probe observed 44.000030px while the drawer transform was running and exactly
44px after it settled. This supports the animation measurement diagnosis; the
probe was removed. After correction, the same three-spec sequence passed all
ten checks in 56.8s with retries disabled, one worker, two CPUs, and 4 GiB of
memory in the CI runtime image. Targeted ESLint and Prettier checks passed.

The complete old-head blob audit covered 118 reports: four failed attempts,
four retry attempts, 4,266 passed attempts, and 40 skipped attempts, with no
parse errors. These are attempt counts, not distinct-test verdicts. All four
leaf failures are covered by the fixture corrections above. The old-head
workflow rollup was still queued after the bounded wait.

## CI follow-up: live workflow provenance (2026-10-27)

The next CI run at `e1d3d6f120fb` exposed four more E2E failures. The GitLab sync
case revealed an implementation defect: the backend already emits `source`
and `source_path`, but frontend HTTP, boot, and WebSocket mappings dropped them.
Settings then reconstructed newly observed workflows without sync ownership.
A no-change manual sync does not reload the page, so that workflow could stay
editable until a later navigation.

The frontend now retains provenance through those mappings and both settings
snapshots. Provenance updates apply while local editable-field drafts remain
intact. Missing metadata in partial events preserves known ownership; explicit
manual ownership and an empty path release it. The browser regression runs an
initial sync outside the view's handler, observes read-only controls through
live updates, and confirms a subsequent no-change manual sync preserves them.

Validation:

- Six new runtime regressions failed before the correction. After correction,
  the three focused Vitest files passed all 53 tests.
- The rebuilt GitLab workflow-sync browser suite passed all four cases with
  retries disabled.
- The state-boundary fixture suite passed all 22 checks with retries disabled:
  Office approval waits for the committed status, profile reuse inspects one
  snapshot with both sessions settled, and desktop/phone pagination waits for
  archive cleanup before restoring a task. It retains the identity, state,
  pagination, and conversation assertions.
- Type checking passed. Changed-file ESLint passed without warnings after
  extracting the workflow mapping callback. Prettier and diff checks passed.

Public documentation already defines read-only synced workflows and release
to manual editing in `docs/public/workflow-sync.md`. This correction restores
that contract. The metadata change uses the shared settings state and leaves
the phone and desktop interaction structure intact.

The separate walkthrough generation job reached its 600-second deadline on
both the original run and one fresh generation run. Both diagnostic artifacts
contain an empty draft and no agent stdout; verification never ran. The cause
is unknown. These jobs execute the trusted base workflow, independently of the
feature code under review. Both failed artifacts were retained.

## CI follow-up: clock, agent, and picker fixtures (2026-10-27)

CI run `40428275527` at `0452fcdf57de` reported these failed first attempts,
each followed by a passing retry:

- Shard 9, job `121951085922`: the phone search cancellation test failed in
  `clock.pauseAt(new Date())` with `Cannot fast-forward to the past`, before
  checking search behavior. Both phone and desktop search fixtures now install
  an explicit origin and pause at that origin plus 60 seconds. Their existing
  timer advancement, cancellation, input, and result assertions remain intact.
- Shard 11, job `121951085938`: phone profile deletion selected `agents[0]`,
  whose profile was undefined. It now uses the existing `getMockAgent` fixture
  helper for all three cases. Virtual or disabled families can precede the
  mock agent and need not own executable profiles.
- Shard 23, job `121951085960`: the worktree-to-local branch case timed out
  after its executor option detached. The trace shows the executor picker
  opened immediately after repository selection, without waiting for the
  repository picker's focus restoration. This supports a focus-restoration
  race diagnosis. The shared dialog helper and this case now wait for restored
  repository-chip focus before opening the next picker.

The original phone search spec passed 4/4 in isolation, and the original branch
case passed 1/1. Project-specific context replay from the downloaded four-part
shard manifests passed 10, 7, and 31 checks before changes. Thus local runs did
not reproduce the CI failures; no deterministic local red result is claimed.

After correction, those same contexts passed 10, 7, and 34 checks. The final
context includes all three desktop search debounce cases. All replays used the
CI runtime image, one worker, `--cpus=2 --memory=4g`, and `--retries=0`.
The existing mock-agent helper unit tests passed 2/2. Changed-file ESLint,
Prettier, and diff checks passed. These corrections change test fixtures only.

The current-head walkthrough generation job passed on this run. Its preceding
two timeouts remain unexplained; no generator policy or timeout was changed.
Full CI and review fixup remains in progress until the replacement head is
verified.

## CI follow-up: late task-route enrichment (2026-10-27)

The late fourth failure in run `40428275527`, shard 15/job `121951086081`,
timed out in `workflow-automation.spec.ts` while waiting for a prepared session
to become the active task/session. Its isolated browser case passed 1/1.
The CI snapshot showed the task header but no selected task in its inspector.

Two focused unit regressions reproduced the defect deterministically.
`fetchTaskNavigationEnrichment` captured an earlier sessionless or existing
identity, waited for optional resources, and returned navigation selection
from `taskToState` plus the older session list. Hydrating this result cleared
or replaced a session accepted during the wait. Correcting selection alone
still failed the collection assertions: the old list removed the new session.

The correction restores the existing requirements in
`docs/specs/ui/requirements/task-navigation-responsiveness.md`, specifically
`AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.7` and `.9`, and the essential-hydration
boundary in its system design. Client enrichment no longer publishes task
selection or a session collection that essential hydration already resolved.
An initially unavailable session list retains its existing enrichment recovery.
Full SSR entry points retain their initial selection and hydration behavior.

Validation:

- Both new selection/collection regressions failed before the correction.
  They cover sessionless and existing-session identities, a newly prepared
  session, the selected/pinned conversation, remembered selection, and the
  current session collection while optional agents are delayed.
- All 72 tests passed across the session-page state, bounded navigation,
  task-route, route-recovery/workspace, and task-page-content suites.
- Freshly built desktop browser checks passed 11/11: workflow automation,
  automatic session creation, durable transcript ordering, and GitLab sync.
- Phone prevent-auto-start and start-step routing checks passed 2/2 with
  one worker, two CPUs, 4 GiB memory, and retries disabled.
- Full web typecheck, changed-file lint, formatting, and diff checks passed.

This restores existing behavior without changing navigation composition,
backend APIs, or public documentation. No temporary logging or diagnostic
source is included. The completed prior-head blob audit contains 118 reports,
4,322 attempts, four retry/error attempts, 4,278 passed attempts, and 40 skipped
attempts, with no parse errors. All four failed leaf tests have corrections;
the replacement head still needs CI/review verification.

## Conflict resolution and browser synchronization (2026-10-27)

Merge commit `31b9bdbc35ab` integrates main `b66f9e3b9d4d`. The sole conflict
was `mobile-agent-profile-delete.spec.ts`. The resolution keeps main's exact
seeded-profile fixture and all assertions; the file leaves the PR delta.
The merge passed 127 focused web tests, full web typecheck, and 14 browser
checks. Compatibility with newer main `d549fa694b7d` passed 175 focused web
tests and the overnight analytics race test. CI checked out that same merged
tree, `24bc1a3ed995351a6c04593743a7e96253361ae9`.

CI run `40481986876` then reported two flaky browser cases: mobile terminal
close setup in shard 3 and delayed capacity cancellation in shard 21.
The aggregate failed from those leaf failures. The complete report contains
118 blobs, 4,282 passing attempts, 40 skipped attempts, two failed attempts,
and two retries. The retry summary confirms exactly two flaky verdicts.
There are no unresolved review threads at this head.

The corrections synchronize test setup with the existing transport boundary:

- Mobile terminal creation waits for the picker animation and the correlated
  `user_shell.create` reply before checking the two rows.
- Delayed capacity checks arm a session-scoped waiting-notice observer before
  the final clock advance. They await attempt 6 before asserting its UI.
  The observer budget starts at the final advance, after earlier retries.

Unmodified local replays passed 32 shard-partition cases and 12 desktop
capacity cases, so they did not reproduce the CI failures. A temporary
6.5-second backend delay before writing the sixth waiting notice reproduced
the old capacity assertion deterministically. The corrected test passed with
that same delay and unchanged UI assertion. The diagnostic change was removed
before rebuilding all backend fixtures for final verification.

After correction, the same shard partition passed 32/32 cases. Two repeats of
the desktop/phone capacity flows and phone terminal close passed 18/18 cases.
These clean runs used the CI runtime image, one worker, two CPUs, 4 GiB memory,
and retries disabled. The temporary per-test retry override was restored in
the committed source. All 32 synchronization-helper unit tests, full web
typecheck, targeted ESLint, formatting, and diff checks passed.

These changes affect test synchronization only. Product behavior, requirements,
public documentation, assertions, and the committed retry policy are unchanged.
Replacement-head CI and review verification remains a delivery gate.
