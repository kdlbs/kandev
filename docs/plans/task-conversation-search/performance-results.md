# Conversation search validation

Measured on 2026-10-19. These are development-host observations, not a
certification of the 4-core, 8-GiB reference host. The host runs other work.

## Environment and method

- Linux 6.18.49, Intel Core i7-11700T, 16 logical CPUs, 31 GiB RAM, local NVMe.
- Go 1.26.6; SQLite 3.51.1 with FTS5; PostgreSQL 17.11 with pg_trgm, C locale.
- Disposable generated data: 10,000 tasks, ten workspaces, 100k then 1m messages,
  approximately 1 KiB per message. Ten percent of tasks are archived.
- Each query class has three warmups followed by 30 timed requests. Each repository query
  has a 750 ms deadline. Authorization, HTTP DTO enrichment, and network transit
  are excluded from these latency samples. p95 is the 29th sorted observation. Errors and timeouts
  remain in the samples and are counted separately.
- The SQLite measurements follow bounded candidate materialization. Later changes
  bound each materialized excerpt and add an exact-match predicate; SQLite's
  index and candidate limit are unchanged.
- Projection build times below measure fixture bulk population, not the throttled
  background worker or an end-to-end historical upgrade. They must not be used
  as a historical-index completion estimate.

The original timing harness counted request errors but did not retain page
states or verify expected hits. These historical samples therefore do not
certify successful-result latency. The revised harness validates expected
matches and reports successful latency, limited responses, other page states,
and errors separately. No new full-scale measurement is claimed by this review
fix. Correctness and sample-classification regression tests cover the changes.

## Historical query timing observations

| Database | Messages | Query | p50 (ms) | p95 (ms) | Errors/timeouts |
| --- | --- | --- | --- | --- | --- |
| SQLite | 100000 | common phrase | 26.95 | 35.79 | 0/30 |
| SQLite | 100000 | absentquartzword | 9.63 | 11.41 | 0/30 |
| SQLite | 100000 | selective-needle | 11.25 | 12.60 | 0/30 |
| SQLite | 100000 | café | 9.61 | 10.47 | 0/30 |
| SQLite | 100000 | 中文测试 | 9.54 | 11.10 | 0/30 |
| SQLite | 100000 | archive-needle | 10.84 | 12.61 | 0/30 |
| SQLite | 1000000 | common phrase | 31.09 | 44.82 | 0/30 |
| SQLite | 1000000 | absentquartzword | 19.05 | 27.25 | 0/30 |
| SQLite | 1000000 | selective-needle | 22.66 | 30.80 | 0/30 |
| SQLite | 1000000 | café | 23.36 | 35.22 | 0/30 |
| SQLite | 1000000 | 中文测试 | 27.02 | 33.10 | 0/30 |
| SQLite | 1000000 | archive-needle | 18.84 | 22.91 | 0/30 |
| PostgreSQL | 100000 | common phrase | 35.31 | 37.89 | 0/30 |
| PostgreSQL | 100000 | absentquartzword | 6.83 | 7.21 | 0/30 |
| PostgreSQL | 100000 | selective-needle | 7.16 | 7.85 | 0/30 |
| PostgreSQL | 100000 | café | 4.70 | 5.50 | 0/30 |
| PostgreSQL | 100000 | 中文测试 | 9.73 | 10.46 | 0/30 |
| PostgreSQL | 100000 | archive-needle | 6.57 | 7.16 | 0/30 |
| PostgreSQL | 1000000 | common phrase | 186.95 | 214.05 | 0/30 |
| PostgreSQL | 1000000 | absentquartzword | 3.64 | 3.84 | 0/30 |
| PostgreSQL | 1000000 | selective-needle | 6.45 | 8.01 | 0/30 |
| PostgreSQL | 1000000 | café | 5.71 | 6.08 | 0/30 |
| PostgreSQL | 1000000 | 中文测试 | 6.48 | 8.50 | 0/30 |
| PostgreSQL | 1000000 | archive-needle | 5.82 | 7.11 | 0/30 |

Selective PostgreSQL plans use `Bitmap Index Scan on task_conversation_text`
before canonical joins. The C-locale Unicode case fell from 538.30 ms p95
with direct-text pg_trgm to 8.50 ms with the locale-independent key.

## Storage and construction

SQLite allocated 745,132,032 bytes for the complete 100k fixture database and
7,380,398,080 bytes for the complete 1m fixture. These totals include canonical
records and application indexes; they are not search-index-only sizes. Bulk
projection population took 22.27 seconds at 100k and 170.73 seconds for the
additional 900k messages. Fixture construction plus both measurements took
441 seconds.

PostgreSQL projection plus index allocated 159,162,368 bytes at 100k and
1,535,401,984 bytes at 1m. Bulk projection population took 11.56 seconds at
100k and 60.87 seconds for the additional 900k. Its canonical message tables
are excluded from these sizes.

## Streaming and large messages

The final ten-session run recorded p95 source writes of 21.124 ms with indexing
disabled and 19.329 ms with indexing enabled. This does not establish a speedup:
shared-host I/O varies between phases. It shows no measured p95 regression in
this run. Each phase performs 1,000 writes, ten per round, pacing rounds at
100 ms when writes permit. FULL-synchronous storage can reduce achieved rate
below the nominal 100 writes/second on this shared host.

The initial per-message index transactions raised p95 writes from 19.45 ms to
37.13 ms. Publication now uses one transaction per bounded batch and coalesces
live work once per second. Idle historical work yields between 25-ms batches.

A 10,500,000-byte message took 428.64 ms to index and allocated 31,517,656 Go
bytes. This is cumulative Go allocation, not total process peak RSS; native
SQLite memory is excluded.

## Correctness and integration

- SQLite and real PostgreSQL literal matching, Unicode, canonical session
  ownership, edits, stale-token rejection, rollback, eligibility changes,
  workspace moves, cascade deletion, and replayed backfill pass.
- Missing SQLite FTS5 returns unavailable without preventing repository startup.
- Authorization and admission limits pass. Runtime cleanup joins the worker.
- Frontend tests cover independent sources, metadata fallback, result caps,
  workspace/close cancellation, loading state, and matching-session navigation.
- Concurrent PostgreSQL tests force source-first and worker-first commits under
  real row locks. The final race run passes.
- All 15 desktop browser scenarios pass, including existing command, file,
  metadata, archive, and keyboard behavior plus conversation highlighting,
  session navigation, and edit removal.
- All three phone browser scenarios pass: conversation search, move/archive,
  and change workflow. The closed-drawer regression has a failing-then-passing
  component test. The final targeted frontend run passes 45 tests in seven files.
- Go lint, ESLint, TypeScript, the web build, and localization checks pass.

## Reproduce

From `apps/backend`, set `KANDEV_SEARCH_SCALE=1` and run the tests named
`TestConversationSearchPerformanceBudget`,
`TestConversationSearchPostgresPerformanceBudget`, and
`TestConversationSearchStreamingCost` with `-tags fts5 -v -timeout 30m -count=1`.
PostgreSQL additionally requires `KANDEV_TEST_POSTGRES_DSN` for a disposable
server. Run scale tests sequentially. Ordinary test runs skip scale fixtures.

## Remaining evidence limits

Reference-host qualification, production-sized upgrade elapsed time, and a full
storage-restore rehearsal are not measured by this report. Lifecycle tests cover
restart checkpoints and stop/join wiring. No production data was read or changed.

## Targeted check commands

From `apps/backend`, with a disposable `KANDEV_TEST_POSTGRES_DSN`:

```bash
GOMAXPROCS=2 go test -tags fts5 -race ./internal/task/repository/sqlite ./internal/task/conversationsearch ./internal/backendapp -run ConversationSearch -count=1
go test -tags fts5 ./internal/task/service ./internal/task/handlers -run ConversationSearch -count=1
go test ./internal/task/repository/sqlite -run '^TestConversationSearchUnavailable$' -count=1
```

From `apps/web`:

```bash
pnpm exec vitest run hooks/use-command-panel-task-results.test.ts hooks/use-command-panel-task-navigation.test.ts components/navigation/app-nav-sheet.test.tsx components/navigation/mobile-task-search-action.test.tsx components/command-panel-dialog.test.tsx components/command-panel-confirmation.test.tsx lib/api/domains/conversation-search-api.test.ts
NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck
pnpm run i18n:check
pnpm e2e:run --host --no-build --shards 1 --project chromium tests/task/conversation-search.spec.ts tests/command-panel.spec.ts
pnpm e2e:run --host --no-build --shards 1 --project mobile-chrome tests/task/mobile-conversation-search.spec.ts tests/task/mobile-command-palette-task-actions.spec.ts
```

The Nix host required Chromium shared libraries through a command-scoped
`LD_LIBRARY_PATH` and `KANDEV_SERVER_HOST=127.0.0.1` to override its local bind
configuration. Backend and web artifacts were built before `--no-build` runs.

## PR review remediation

The updated-base validation on 2026-10-20 adds direct API-client checks for
literal query encoding and cancellation through the real HTTP client. Worker
checks now include package-level goroutine leak detection, bounded failure
logging, and cancellation suppression. Traditional Chinese search strings are
regenerated with the repository converter. Failure logs contain only fixed
stage reasons and are limited to one warning per minute across the worker.

```bash
(cd apps/backend && GOMAXPROCS=2 go test -race ./internal/task/conversationsearch ./internal/backendapp -run ConversationSearch -count=1)
(cd apps/web && pnpm exec vitest run lib/api/domains/conversation-search-api.test.ts)
```

CodeRabbit remediation adds a real PostgreSQL reproduction for reverse-order
multi-message publication and session deletion, including messages that changed
eligibility. Publication now skips locked dirty rows for a later pass, while
source deletion fences every projected message before removing it. Batched
publication remains in place. Historical backfill queues eligible conversation
messages only. Setup logs classify capability/permission failures with fixed
reasons, suppress lifecycle cancellation, and exclude raw database details.
