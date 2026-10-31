---
id: "04-daily-projection"
title: "Maintain canonical daily summaries"
status: in_progress
wave: 4
depends_on: ["03-collector-contract"]
plan: "plan.md"
requirements:
  - REQ-COSTS-USAGE-HISTORY-004
  - REQ-COSTS-USAGE-HISTORY-006
  - REQ-COSTS-USAGE-HISTORY-007
acceptance_criteria:
  - AC-COSTS-USAGE-HISTORY-004.3
  - AC-COSTS-USAGE-HISTORY-006.1
  - AC-COSTS-USAGE-HISTORY-006.2
  - AC-COSTS-USAGE-HISTORY-006.3
  - AC-COSTS-USAGE-HISTORY-006.4
  - AC-COSTS-USAGE-HISTORY-006.5
  - AC-COSTS-USAGE-HISTORY-006.6
  - AC-COSTS-USAGE-HISTORY-006.7
  - AC-COSTS-USAGE-HISTORY-007.1
  - AC-COSTS-USAGE-HISTORY-007.7
system_design:
  - ../../specs/costs/system-design/usage-history.md
---

# Task 04: Maintain canonical daily summaries

## Summary

Persist selected facts and compact daily summaries from durable dirty-partition work.
Make corrections, crash recovery, and full rebuilds deterministic without rewriting source observations.

## In scope

- Add canonical facts, daily summaries, scope metadata, projection revisions, and restart-safe work checkpoints.
- Resolve only affected identity/coverage partitions through Task 03's selector.
- Keep token and cost provenance, actors, providers, currencies, calendars, and missing/partial counters distinct.
- Cover native per-user tokens beside mixed-user external cost without allocating cost or duplicating token contributions.
- Replace old contributions and rebuild affected day keys for downward corrections and source/model changes.
- Commit facts, summaries, freshness metadata, revision watermarks, and work completion atomically.
- Add shadow-generation rebuild and cutover, with explicit preparation/freshness state.
- Keep internal commit sequences and generation-specific catch-up. Fence the switch against live corrections, deletion, and org erasure.
- Keep undated lifetime totals separate. Preserve saved canonical values during asynchronous projection lag.
- Reconcile source revisions through indexed checkpoints after restart. Add bounded backlog/duration diagnostics without high-cardinality metric labels.

## Out of scope

- HTTP/UI cutover, plugin adaptation, new pricing, and retained observation expiry.

## Acceptance

- Replays, corrections, overlapping collectors, model changes, and partial imports produce the same canonical totals as a clean rebuild.
- Every crash point exposes either the old complete generation or the new complete generation, never a half-updated authoritative summary.
- Work remains durable after restart. Independent database writers cannot publish older revisions over newer summaries.

## Verification

```bash
(cd apps/backend && go test -race -trimpath ./internal/analytics/... ./internal/backendapp -run 'TestUsageHistoryProjection|TestUsageHistorySummary|TestUsageHistoryGeneration|TestUsageHistoryReconcile' -count=1)
(cd apps/backend && : "${KANDEV_TEST_POSTGRES_DSN:?Set an isolated PostgreSQL test DSN}" && go test -trimpath ./internal/analytics/... -run 'TestUsageHistoryPostgres' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- New `apps/backend/internal/analytics/repository/sqlite/usage_history_projection*.go` and summary/rebuild tests
- `apps/backend/internal/analytics/{models,repository,service}`
- `apps/backend/internal/backendapp` service startup/shutdown wiring
- Existing migration metadata and scoped erasure/reset registration from Task 02

## Dependencies

Task 03. Use its generic observations and indexed canonical selector.

## Risks

Unknown amounts cannot be subtracted as zero. Rebuild affected keys when exact contribution arithmetic is unavailable.
A volatile event notification cannot be the only dirty-work record.
Shadow generations must not double retained-source storage or become an unbounded orphan after interruption.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/costs/requirements/usage-history.md), REQ-004/006/007 and listed ACs.
- [Design](../../specs/costs/system-design/usage-history.md), Canonical facts and daily summaries, Migration and recovery.
- Task 03 selector tests and current nullable-cost/effective-rate tests.

## Results

Implementation and test handoff accepted. Code is frozen pending the final semantic review after remaining worker phases.
No shared plan, frontend, or SDK files were changed by this worker.

### Implementation

- Accepted native and external mutations allocate a scope commit sequence while
  holding the transactional admission row lock. Source changes, replayable
  before/after payloads, tombstones, and old/new invalidations commit together.
  Collector revisions remain separate from this internal commit order.
- Rebuilds retain source bounds, scan cursors, per-item sequences, replay cursors,
  selector progress, and publication groups by generation. Shadow work does not
  consume the active generation's replay input. Cutover shares the admission and
  captured-scope erasure fence. Replay pruning waits for every consumer watermark.
- Canonical facts and metric contributions stage behind build IDs. Related work
  invalidations publish atomically, including corrections that change work
  identity. The same transaction replaces affected summaries, updates canonical
  revisions and freshness, and completes durable work.
- Token and cost provenance and actors remain independent. Summaries preserve
  known amounts, measured zeroes, missing/partial counters, currencies, providers,
  models, clients, semantics, and calendars. Mixed-user cost receives no allocated
  token denominator. Lifetime and dated representations remain alternatives.
- The durable oversized-group resolver retains winner and overlap checkpoints
  while reading bounded candidate pages. Restart tests cover 221 equivalent
  candidates and 205 dated facts without permanent preparation or invented zeroes.
- PostgreSQL point identity survives timestamp precision changes. Native points
  remain points despite their source-date metadata. Timed canonical reads use
  occurrence predicates for points and overlap predicates for intervals.
- Private selector/source-bound paths project captured organization scopes after
  workspace transfer. Public and plugin paths retain current-organization access
  checks. Erasure/reset removes all 18 projection tables, including inactive
  generations, publication groups, and checkpoints. Stale work cannot recreate an
  erased generation, including after a workspace reset reuses the scope identity.
- The worker owns cancellation and joining for startup, shutdown, and restore.
  Projection reads and batches use the analytics admission gate and deadline.
  Generation/work records retain progress; worker statistics record bounded
  batch, failure, and duration counters without source identity labels.
- Required-store registration includes retained accounting and projection tables.
  The v0.93.0 upgrade manifest records their absence in that historical fixture;
  the fixture SQL and its source hashes remain unchanged.

### TDD and corrected regressions

Observed RED cases included the missing transactional replay log, missing durable
rebuild operations, oversized-group preparation, loss of measured zeroes during
encoding, stale before-image tombstone state, immediate unprojected canonical
reads, a work-identity correction briefly totaling 140 instead of 100 or 40,
and PostgreSQL point facts being classified as intervals/days. The final tests
exercise the repaired production paths.

The first affected regression run also found a Host fixture that expected
synchronous canonical selection. It now asserts initial preparation without a
publication token, saved values while a correction is pending, and a changed
opaque token after durable publication. Its original amount, deletion,
capability, scope, and integer-precision assertions remain covered. The failure
receipt is `/tmp/usage-history-worker4-native-plugin-before-async-fixture.log`.

Earlier checkpoint passes are historical. The following receipts cover the
current implementation after the cleanup and registration changes.

### Current-tree verification

Commands below run from `apps/backend`, except the scoped lint helper, which runs
from the repository root. PostgreSQL commands silently source
`/tmp/usage-history-pr3660-postgres.env` with `set -a` / `set +a` and require
`KANDEV_TEST_POSTGRES_DSN`; no connection string is recorded here.

1. **PASS, exit 0:** 17 top-level Task 04 tests, plus their subtests; no failures
   or skips. Repository, service worker, and backend startup/restore tests ran.
   Analytics handlers reported no tests selected by this regex; that line is not
   claimed as handler coverage.

   ```bash
   GOMAXPROCS=2 go test -p 1 -race -trimpath -v ./internal/analytics/... ./internal/backendapp -run 'TestUsageHistoryProjection|TestUsageHistorySummary|TestUsageHistoryGeneration|TestUsageHistoryReconcile' -count=1
   ```

   Log: `/tmp/usage-history-worker4-final-race.log`.

2. **PASS, exit 0:** five selected PostgreSQL 18 tests with independent database
   writers, point calendars, captured-scope erasure, and cutover crash recovery.
   No PostgreSQL case skipped. Only the repository package selected cases with
   this regex.

   ```bash
   GOMAXPROCS=2 go test -p 1 -race -trimpath -v ./internal/analytics/... -run 'TestUsageHistoryPostgres' -count=1
   ```

   Log: `/tmp/usage-history-worker4-final-postgres.log`.

3. **PASS, exit 0:** 38 top-level affected native/plugin tests, no failures or
   skips. The usage writer package selected three actual cases. The repository
   cases also exercised PostgreSQL native admission, deletion preservation, and
   captured organization erasure.

   ```bash
   GOMAXPROCS=2 go test -p 1 -race -trimpath -v ./internal/task/repository/sqlite ./internal/task/usage ./internal/plugins -run 'TestUsageHistory|TestSessionUsage|TestCreateTaskUsageEvent|TestTaskUsage|TestWriter_EndToEnd_RealRepoPreservesArrivalOrder|TestBuildRow_CarriesAdmittedUsageBinding|TestDecodePayload_ValidEvent_PopulatesAllFields' -count=1
   ```

   Log: `/tmp/usage-history-worker4-final-native-plugin.log`.

4. **PASS, exit 0:** scoped Go lint, `0 issues`. No full-backend lint scan ran.

   ```bash
   bash scripts/lint-go-changed apps/backend/internal/analytics/models/usage_history_projection.go apps/backend/internal/analytics/repository/sqlite/usage_history_projection_read.go apps/backend/internal/analytics/repository/usage_history.go apps/backend/internal/analytics/service/usage_history_worker.go apps/backend/internal/backendapp/usage_history_projection.go apps/backend/internal/persistence/requiredstores/catalog.go apps/backend/internal/plugins/host_usage_history_test.go
   ```

   Log: `/tmp/usage-history-worker4-final-lint.log`.

5. **PASS, exit 0:** vet for the affected packages.

   ```bash
   GOMAXPROCS=2 go vet -p 1 -trimpath ./internal/analytics/... ./internal/backendapp ./internal/plugins ./internal/task/repository/sqlite ./internal/task/usage ./internal/persistence/requiredstores
   ```

   Log: `/tmp/usage-history-worker4-final-vet.log`.

6. **PASS, exit 0:** SQLguard. This worker changed no guard exemptions.

   ```bash
   GOMAXPROCS=2 go run -p 1 ./cmd/sqlguard ./internal
   ```

   Log: `/tmp/usage-history-worker4-final-sqlguard.log`.

7. **PASS, exit 0:** persistence conformance, including the updated required-store
   catalog, upgrade manifest, both database engines, and prior-stable upgrades.
   All 11 top-level tests passed; no cases skipped.

   ```bash
   GOMAXPROCS=2 go test -p 1 -race -trimpath -v ./internal/persistence/storeconformance -count=1
   ```

   Log: `/tmp/usage-history-worker4-final-conformance.log`.

Changed Go files are formatted. Documentation and diff validation receipts are
recorded in `/tmp/usage-history-worker4-final-docs.log` after this Results update.

### Stable reporting handoff

Repository and service expose:

- `ListUsageCanonicalFacts(ctx, filter)` returns selected facts with independent
  `TokenSource` / `CostSource` evidence and `UsageProjectionMetadata`.
- `ListUsageDailySummaries(ctx, filter)` returns exact-calendar summaries and
  metadata, including `HasMore` / `NextOffset`. Subject-specific and non-calendar
  day-boundary queries use the canonical fact path.
- `ListCanonicalUsagePage(ctx, filter)` preserves the Host compatibility shape
  and returns the saved projection status, opaque publication token, and pending
  flag. A preparing response has no invented publication token.

The public `metadata.Generation` token identifies the committed generation and
canonical revision. Consumers compare equality only and restart mixed-publication
pagination. Internal summary rows retain the storage generation UUID; reporting
DTOs must use the metadata token for `projection_generation`.

Published fact/contribution SQL joins `usage_projection_applied` by scope,
generation, work key, and build ID. This excludes staging and abandoned builds.
`usage_canonical_facts` retains full canonical payloads and coverage indexes;
`usage_projection_contributions` provides SQL amounts and separate metric actors,
provenance, currencies, and calendars. Daily summaries require a matching calendar.
Lifetime summaries remain separate. Arbitrary browser-calendar interval grids,
coarse boundary disclosure, report DTO mapping, UI cutover, and five-year query
benchmarks remain Task 05 work; no capacity claim is made here.
