---
id: "07-operator-docs-load-validation"
title: "Document operations and validate sustained load"
status: done
wave: 3
depends_on: ["06-database-maintenance-command"]
plan: "plan.md"
spec: "../../specs/platform/pr-watch-and-storage-bounds.md"
---

# Task 07: Document operations and validate sustained load

## Intent

Publish the safe upgrade/maintenance procedure and prove the combined change is stable under the production-like duplicate-watch load.

## Acceptance

- Public operations documentation describes backup of kandev.db and master.key, image upgrade, migration/maintenance dry run, execute/compact, rollback, and post-release health checks.
- A deterministic ten-minute multi-task/coordinator load test proves no watch-creation loop, one lookup per canonical identity, no recurring exhausted CAS errors, and stable API/history latency.
- Documentation identifies the maintenance command’s required backup and operator-selected retention, without claiming automatic deletion of conversations.

## Files likely touched

- docs/public/operations.md (new "Upgrading past the canonical PR-watch migration" subsection)
- apps/backend/internal/github/load_validation_test.go (new load-validation fixture)
- apps/backend/internal/task/statussummary/projector_load_test.go (new sustained-contention fixture)

## Dependencies

Task 06.

## Parallelism

Sequential. Documentation must describe the shipped command and load validation exercises the integrated behavior.

## Verification

~~~bash
cd apps/backend && go test ./internal/github ./internal/task/statussummary -run 'Test.*(Canonical|Load|TenMinute|Concurrent|CAS).*' -count=1 -v
cd apps/backend && go test ./internal/github ./internal/task/statussummary -count=1
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
~~~

## Output contract

Report the exact load fixture scale, lookup/watch/CAS/error/latency measurements, public-doc pages and Diátaxis type, validation outcomes, and upgrade/rollback instructions verified. Update task and plan status.

## Results

**Status: done.**

**Operator documentation** (`docs/public/operations.md`, new "Upgrading past
the canonical PR-watch migration" subsection inside the existing "Database
operation details" `<details>` block, alongside the existing PostgreSQL
schema-cutover upgrade pattern): describes the one-time, idempotent,
transactional `github_pr_watches` migration
(ADR-2026-08-31-task-owned-pr-watch-identity), that it runs after the
existing version-change SQLite snapshot and aborts startup before any
destructive change on a failed snapshot or migration, and that it only ever
removes duplicate/orphaned watch rows while preserving Review-task
monitoring and never touching conversation messages/snapshots/plans. Gives
the four-step safe-upgrade procedure (verify a backup, stop all writers
before starting the new binary, start exactly one instance and spot-check
health plus an open-PR task and a completed-session Review task, optionally
run `kandev maintenance database` afterward) and cross-links to the
existing [Database maintenance](../../public/cli.md#database-maintenance)
section (Task 06) for the maintenance command's own dry-run/execute/compact/
rollback semantics, plus a rollback note (restore the pre-migration snapshot
only if an older binary is needed again; the migration is otherwise safe to
retry because it is idempotent). This satisfies the acceptance criterion's
required backup/upgrade/migration-dry-run/execute-compact/rollback/health-check
coverage and explicitly states the maintenance command's own retention is
operator-selected and backup-gated, not automatic conversation deletion.

**Load validation** (new deterministic fixtures, no shared test-scaffold
package needed — both reused existing package test helpers):

- `apps/backend/internal/github/load_validation_test.go` —
  `TestLoadValidation_TenSimulatedMinutes_NoWatchCreationLoopBoundedLookups`.
  Seeds one task resumed across 50 historical sessions (mirroring the plan's
  AC1 fixture) plus 24 other concurrently-active tasks (25 canonical
  task/repository/branch identities total, standing in for many
  simultaneous coordinator-driven boards), then drives ten simulated
  `checkPRWatches` poll cycles (the poller's real `defaultPRPollInterval`
  cadence, without sleeping wall-clock time between cycles). Asserts: (1)
  the exact watch-ID set is unchanged from cycle 0 through cycle 9 and the
  canonical watch count is always 25, never the 74 total historical-session
  entries the provider reports (no watch-creation loop); (2) each cycle
  performs exactly one batched GraphQL branch-search call for all 25
  still-searching watches, so ten cycles produce exactly 10 branch calls and
  0 numbered-PR calls (bounded lookups, independent of session-history
  size); (3) no single cycle's wall-clock duration exceeds a 2-second
  absolute bound (stable latency, ruling out a duplicate-row leak's
  quadratic-cost shape). Reuses the package's existing
  `setupBatchedPollerTest`/`mockTaskBranchProvider`/`graphQLMockClient`/
  `seedTask` test helpers.
- `apps/backend/internal/task/statussummary/projector_load_test.go` —
  `TestProjectorSustainedLoadAcrossManyTasksConvergesWithoutExhaustedCAS`.
  Scales the existing single-task
  `TestProjectorConcurrentSessionEventsConvergeWithoutExhaustedCAS` harness
  up to the same 25-task, ten-simulated-cycle shape as the github-package
  fixture: each cycle fires one concurrent burst of 4 sessions x 25 tasks
  (100 concurrent `HandleEvent` calls/cycle, 1000 total across the run)
  while a new per-task `perTaskAlternatingRejectStore` continuously forces
  every other `CompareAndUpdateTaskStatusSummary` call to lose its CAS race
  for every task throughout the entire sustained run (not just a single
  burst) — modeling a boot-reconciliation pass or HTTP rebuild racing the
  live projector continuously. A new per-task store was required because
  the existing single-task `alternatingRejectProjectorStore`'s one *global*
  counter only guarantees "reject-then-accept" ordering when no other
  task's calls can interleave between a given task's own attempts; with 25
  tasks contending on one global counter, that guarantee breaks and
  produces spurious CAS-exhaustion errors unrelated to the code under test
  (confirmed by first reproducing the failure, then fixing it with the
  per-task counter). Asserts zero exhausted-CAS handler errors across all
  1000 calls and that every task's final summary reflects the cumulative
  40 sessions applied to it over the ten cycles, proving sustained,
  many-task contention never drops or misapplies concurrent work.

**Verification commands run** (from `apps/backend`):
- `go test ./internal/github ./internal/task/statussummary -run 'Test.*(Canonical|Load|TenMinute|Concurrent|CAS).*' -count=1 -v` — all pass (includes both new load fixtures and the existing canonical-migration/CAS/contention suites).
- `go test ./internal/github ./internal/task/statussummary -count=1` — `ok` both packages.
- `go test ./internal/github/... -run 'TestLoadValidation' -race -count=10` and `go test ./internal/task/statussummary/... -run 'TestProjectorSustainedLoad' -race -count=10` — both stable across 10 repeated runs under `-race`.
- `go build ./...` and `go vet ./...` (full repo) — clean.
- `gofmt -l` on both new test files — no output (clean).
- `golangci-lint run ./internal/github/... ./internal/task/statussummary/...` — `0 issues` (one `staticcheck` QF1008 finding on the first draft of the per-task store was fixed by renaming its own mutex field to avoid embedded-field ambiguity, then re-verified).
- `node scripts/validate-public-docs.mjs` — validated 41 published docs pages.
- `node --test scripts/validate-public-docs.test.mjs` — 61 tests, all pass.
- `git diff --check` — clean.

**Deliberate deferrals:** none new. The three pre-existing, unrelated
`internal/launcher/dev_test.go` toolchain failures documented in Task 06's
Results remain untouched and out of scope for this wave; this task's
verification commands do not target `internal/launcher`.

This is the final task in the plan; all seven waves (Tasks 01-07) are now
implemented, tested, documented, and committed.

