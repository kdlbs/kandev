---
id: "04-daily-projection"
title: "Maintain canonical daily summaries"
status: pending
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

Pending.
