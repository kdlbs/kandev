---
id: "04-performance-evidence"
title: "Scale evidence and public guidance"
status: in_progress
wave: 4
depends_on: ['03-palette-results']
plan: "plan.md"
requirements:
  - REQ-TASKS-CONVERSATION-SEARCH-001
  - REQ-TASKS-CONVERSATION-SEARCH-002
  - REQ-TASKS-CONVERSATION-SEARCH-003
acceptance_criteria:
  - AC-TASKS-CONVERSATION-SEARCH-001.5
  - AC-TASKS-CONVERSATION-SEARCH-002.3
  - AC-TASKS-CONVERSATION-SEARCH-002.4
  - AC-TASKS-CONVERSATION-SEARCH-002.5
  - AC-TASKS-CONVERSATION-SEARCH-003.1
  - AC-TASKS-CONVERSATION-SEARCH-003.2
  - AC-TASKS-CONVERSATION-SEARCH-003.3
  - AC-TASKS-CONVERSATION-SEARCH-003.4
system_design:
  - ../../specs/tasks/system-design/conversation-search.md
---

# Task 04: Scale evidence and public guidance

## Summary

Measure actual performance and operational behavior on disposable history at realistic scale. Document supported search and its setup requirements only after the implementation meets its gates.

## In scope

100k/1m-message deterministic corpus, per-query latency samples, database plans/index bytes, streaming-write comparison, large-message cost, upgrade/restart/restore evidence and public docs.

## Out of scope

Broad unrelated verification, new search infrastructure, or production-user data. Do not report a skipped PostgreSQL run as passed.

## Acceptance

- The benchmark report records host/database versions, per-class p50/p95, successful versus limited results, index size/build cost, cancellation, streaming latency and the largest-message memory cost. Each numeric AC has evidence.
- Benchmark queries prove selective index use and expose common-term cost without excluding timeouts from the report. If a gate fails, fix the responsible implementation and rerun only affected checks.
- Public docs explain searchable text, query minimum, indexing status, phone entry and PostgreSQL pg_trgm setup. Promote requirement/design/plan statuses only after every work order and its checks pass.

## Verification

Run from the repository root. PostgreSQL uses a disposable test database.
New test paths and names are deliverables of this work order.

```bash
(cd apps/backend && KANDEV_SEARCH_SCALE=1 go test -tags fts5 ./internal/task/repository/sqlite -run '^TestConversationSearchPerformanceBudget$' -v -timeout 30m -count=1)
(cd apps/backend && test -n "$KANDEV_TEST_POSTGRES_DSN" && KANDEV_SEARCH_SCALE=1 go test -tags fts5 ./internal/task/repository/sqlite -run '^TestConversationSearchPostgresPerformanceBudget$' -v -timeout 30m -count=1)
(cd apps/backend && KANDEV_SEARCH_SCALE=1 go test -tags fts5 ./internal/task/repository/sqlite -run '^TestConversationSearchStreamingCost$' -v -timeout 30m -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node scripts/validate-public-docs.mjs
git diff --check
```

The budget tests must create the two specified corpus sizes and sample each
query class independently, with warmups outside measurement. Ordinary benchmark
ns/op averages are insufficient evidence for p95. Record all samples, timeouts,
and environment details in the report. The write benchmark compares identical
workloads with indexing enabled and disabled. Use stable generated data, not
real conversation exports. Run all database commands sequentially.

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/conversation_search_benchmark_test.go (new)`
- `apps/backend/internal/task/repository/sqlite/conversation_search_write_test.go (new)`
- `docs/plans/task-conversation-search/performance-results.md (new, measured data only)`
- `docs/public/tasks-and-workflows.md`
- `docs/public/configuration.md`
- `docs/plans/task-conversation-search/plan.md`
- `docs/specs/tasks/requirements/conversation-search.md`
- `docs/specs/tasks/system-design/conversation-search.md`

## Dependencies

Task 03, `03-palette-results`.

## Risks

Index storage may exceed the original text size substantially. Common phrases can require sorting many hits despite an index. Performance limits need the specified reference host, not timings from an overloaded development machine.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/conversation-search.md).
- [System design](../../specs/tasks/system-design/conversation-search.md).
- [Plan](plan.md), scoped AGENTS.md, and the exact source/test patterns named there.

## Results

Development-host measurements and public documentation are complete. SQLite and PostgreSQL each ran at 100k and 1m messages. Streaming and large-message measurements are recorded in [performance results](performance-results.md). This work order remains in progress because the specified reference-host server-latency and sustained-write gates, peak RSS, full historical-upgrade timing, and storage-restore rehearsal are not yet certified. The report does not substitute repository timings for complete server latency.
