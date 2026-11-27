# PR #4404 integration with landed session and retention fixes

Integrated authoritative main `e6692746b0b825c543e6c5f5fb3cb183d65c24bd`
into the existing PR head `9b5bee67142a412bd692814d07c42b8dfe0d31d3`.
The REST pull metadata still reported the old base, so integration used the
verified Git branch tip. No additional semantic review was requested.

The boot-state conflict came from moving task-detail helpers. Keep the moved,
bounded implementation and enrich its selected full session with the landed
current-generation recovery blocks. Compact siblings remain summaries. The
extended boot regression failed because the active recovery block was absent,
then passed after restoring enrichment at the new helper owner.

The Resume fixture keeps the landed recovery-card assertions and reads metadata
through session detail rather than the compact session list. Landed database
retention, projected-cursor, settlement and post-purge write protections remain
in the merge. No public contract or operator instructions changed in this
integration; existing public documentation remains applicable.

## Local validation

Commands ran in the attached PR worktree on 2026-10-11, after resolution:

- Backend: `go test -trimpath -tags fts5 -race ./internal/backendapp ./internal/webapp -run 'Test(Boot|.*Boot|HandlerInjects|DevHandler|Journey|NormalizeBootPayloadGraph|TaskDetailSidebarQuery)' -count=1`: both packages passed.
- Backend: `golangci-lint run ./... --new-from-rev=e6692746b0b825c543e6c5f5fb3cb183d65c24bd --timeout=5m`: zero issues.
- Web: `pnpm exec vitest run src/boot-payload.test.ts lib/ssr/session-page-state.test.ts`: 43 tests passed in two files.
- Web: `pnpm run typecheck`: passed.
- Web: `pnpm exec eslint --max-warnings 0 e2e/tests/session/read-only-resume-helpers.ts` and `pnpm exec prettier --check e2e/tests/session/read-only-resume-helpers.ts`: passed.
- Web: `pnpm e2e:run --project chromium tests/session/read-only-resume.spec.ts`: one test passed, with a fresh managed build.
- Web: `pnpm e2e:run --no-build --project mobile-chrome tests/session/mobile-read-only-resume.spec.ts`: one test passed using the unchanged preceding build.
- Root: `python3 scripts/list-docs.py validate`: 374 decisions and 1,527 specifications validated.
- Root: harness and specification lint, their 19 and 36 unit tests, and `pre-commit run harness-lint --files AGENTS.md apps/backend/AGENTS.md`: passed.
- Conflict markers, unmerged index entries and whitespace: clear.

The first root-invoked browser command selected an incompatible global pnpm
version and ran no tests. The successful commands ran from `apps/web`, using
the workspace-pinned package manager. Historical measurements and provider
claims were not refreshed or expanded. Remote CI and actual merge evidence
remain delivery gates and are reported after the integration push.

## SSH restart snapshot CI correction

At integration head `99f2b29a1410314e4f05ccdcc6447fb42c1966c4`, hosted
E2E Containers Shard 6/6 failed in `captureSilentRestartSnapshot`: compact
session membership omits ACP metadata. The blob report from job
`114461796540` (run `38136330680`) showed the same missing-native-ID failure
on all four attempts. The shared restart helper now reads session detail and
checks both session ID and task ID before capturing native identity.

Two focused snapshot regressions reproduced the old mismatch and then passed
with the corrected helper. `pnpm exec vitest run
e2e/helpers/silent-restart-recovery.test.ts`, targeted ESLint and TypeScript
passed. The exact SSH recovery scenario passed with retries disabled under
`KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --host --project containers
tests/ssh/recovery.spec.ts --grep 'remote execution replays completed output
after backend downtime with local survival disabled' --retries=0`. The managed
runner freshly built matching backend/helper artifacts for this passing run.

Earlier local attempts did not reach the hosted assertion: the first lacked
the Linux mock-agent binary, and retained backend/helper build identities then
differed. Debug logs established that setup failure, and the fresh managed
build resolved it. Those failed attempts are not represented as passing tests.
This is a test-fixture correction with no production behavior or public
documentation change. New-head remote CI remains pending after push.

## Durable-record recovery fixture CI correction

At head `b671c1faf1b8cca60db04c16ed72c817488c776e`, hosted E2E Shard
4/14 (job `114467826807`, run `38138234732`) failed both phone durable-record
recovery scenarios on all four attempts. The snapshot read recovery revision
from the compact session list, which intentionally omits that metadata. The
live-block setup also expected an absent metadata key to be `undefined`, while
the cleared key was serialized as `null`.

The shared fixture now reads full session detail, verifies session and task
ownership, and accepts only null or absent cleared metadata. Reconstruction
revision, message count, no-prompt-replay, restart and recovery-block assertions
remain intact. No production behavior, mobile layout, touch interaction or
public contract changed; existing public and mobile design guidance applies.

`pnpm exec vitest run e2e/helpers/delivery-record-recovery.test.ts` first
reproduced the missing revision and unchecked ownership, then passed both
regressions. Targeted ESLint, Prettier and `pnpm typecheck` passed. The exact
phone scenarios passed, two tests with retries disabled, under a fresh managed
build: `pnpm e2e:run --host --project mobile-chrome
tests/session/mobile-durable-record-recovery.spec.ts --retries=0`.
The unchanged managed build also passed both desktop counterparts under
`pnpm e2e:run --host --no-build --project chromium
tests/session/durable-record-recovery.spec.ts --retries=0`. Hosted checks are
still pending; these local passes do not replace required CI evidence.

The subsequent hosted desktop shard 3/14 (job `114467826746`, same run and
head) failed the same two fixture assertions on every attempt. The shared
correction and the passing local desktop scenarios cover those failures too.

## Backend fixture CI corrections

At the same head, Backend Postgres job `114466613029` (run `38138234720`)
exposed two fixture defects. Writer workload events used the session ID as
their incarnation, but the fixture omitted the session's incarnation and
therefore received a generated different ID. Explicitly seeding the matching
fixture incarnation preserves the landed admission and post-purge protections.
`TestWriterWorkloadIsolated` and `TestWriterWorkloadAttribution` reproduced the
identity mismatch, then passed with `-race` after this fixture-only correction.

Million-row retention accounting measured a cursor before a deadline-returning
transaction had finished on PostgreSQL. A subsequent pass then included that
late commit in its own delta and reported 100 retired rows against a 50-row
single-pass budget. `TestDeliveryRetentionStorageDelayedCommitAccounting`
reproduced the exact `before=50 after=150 retries=0` failure using a held
state-row lease. Test-only cursor observations after a deadline or completed
pass now acquire that existing lease before measuring. Worker limits,
transactions, ownership, projected-cursor and canonical preservation rules are
unchanged.

The focused race run of delayed-commit, accounting, retry-lock settlement and
writer workload tests passed on SQLite and an isolated PostgreSQL 16 instance:
`go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite -run
'^Test(DeliveryRetentionStorage(DelayedCommitAccounting|Accounting|RetryLockSettlement)|WriterWorkload(Isolated|Attribution))$' -count=1`.
Backend lint with the integrated base revision reported zero issues.

The exact PostgreSQL protected-gap million-row case passed with `-race`, keeping
the retirement frontier at 499,999 and the protected backlog at 500,001 while
preserving canonical content. The storage 1024-byte case drained all one
million rows with exact removed-byte accounting and preserved canonical content
through reopening, then failed manual VACUUM because the local validation
container's default 64 MiB shared-memory segment was exhausted (SQLSTATE
53100). This partial run is not recorded as a full pass. Its rerun uses a fresh
owned PostgreSQL container with 1 GiB shared memory; no repository or product
settings changed for that local resource limit.

That fresh-container storage rerun passed completely, including exact byte
accounting, canonical preservation, reopening and manual VACUUM:
`go test -trimpath -tags fts5 -race -timeout 45m -v
./internal/task/repository/sqlite -run
'^TestDeliveryRetentionStorage$/^Postgres$/^1024_bytes$' -count=1`.
The preceding protected-gap pass used the same package, race and timeout flags
with `^TestDeliveryRetentionStorageProtectedGap$/^Postgres$` included in the
test selection. Both commands used `KANDEV_TEST_POSTGRES_DSN` for the isolated
PostgreSQL 16 fixture and `GOMAXPROCS=2`. These are repository test results,
not real-provider or transferred benchmark claims.

An initial local accounting implementation synchronized the baseline too early
and stalled the existing intentional lock-settlement fixture. The owned test
process was diagnosed and stopped, the baseline ordering was corrected, and
the successful focused race run above covers the final implementation. No
public behavior, runtime flag or documentation contract changed in these
backend test-fixture corrections. Remote CI remains a delivery gate.
