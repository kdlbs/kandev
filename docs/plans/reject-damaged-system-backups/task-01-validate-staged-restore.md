---
id: "01-validate-staged-restore"
title: "Validate staged SQLite restore"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-SYSTEM-PAGE-SYSTEM-PAGE-001
acceptance_criteria:
  - AC-SYSTEM-PAGE-SYSTEM-PAGE-001.9
system_design:
  - ../../specs/system-page/system-design/system-page-01.md
---

# Task 01: Validate staged SQLite restore

## Summary

Verify the staged SQLite file before a restore can disable persistence, stop
workers, close the pool, or replace current files. Use real SQLite regressions
to prove damaged backups cannot destroy newer work, and preserve existing valid
restore and failure controls.

## In scope

Implement the staged-file check in `Service.runRestore`, reuse `verifySnapshot`,
remove owned staging on failure, and update affected public tests and docs.
Requirement/design amendments are already prepared in this package.

## Out of scope

Retention/publication changes, schema/version compatibility, migrations,
concurrency redesign, runtime policy, UI changes, and new dependencies.
Do not read/copy/replay ROOT private qualification sources.

## Acceptance

1. Every invalid-snapshot case fails the public restore job before persistence,
   quiescence, legacy shutdown, checkpoint/close, or replacement; staging is gone,
   live files are unchanged, and newer data remains readable/writable.
2. A valid snapshot containing saved data replaces distinct newer data and
   returns `restart_required: true`; configured-path, confirmation, staging-write,
   checkpoint-busy, PostgreSQL, and replacement-rollback controls remain meaningful.
3. Public restore ordering and rejection guidance agree with the amended owning
   requirement/design. All changed A/M paths are covered below and task checks
   have actual recorded exits before this work order is marked done.

## Tests and implementation sequence

After the later explicit implementation release and heavy-lease grant:

1. Set status to `in_progress`. Independently author
   `restore_integrity_test.go` using public `newTestService`, `createSnapshot`,
   `Restore`, and `waitForJob` patterns. Use `t.TempDir`, register cleanup
   immediately, and avoid sleeps or private qualification fixtures.
2. Add `TestRestoreRejectsInvalidSnapshotBeforeDisruption` as independent
   table-driven fixtures: create and verify a valid snapshot from saved data,
   then (a) truncate to zero or 128 bytes, (b) damage its SQLite page/header structure
   deterministically while preserving file length, or (c) replace contents with
   non-SQLite bytes. Assert verification fails for nonempty corruption; for zero-byte truncation,
   assert the shared verifier accepts the empty file under SQLite semantics
   while the restore admission must reject it.
   Enable WAL on the real pool before fixture writes (the default public
   helper alone does not enable it), disable automatic checkpointing for the
   fixture, and ensure WAL/SHM exist. Write distinct newer current data after
   creating the snapshot. Capture main,
   WAL, and SHM bytes/existence immediately before the job; compare immediately
   after terminal failure before SQL reads can change SHM. Assert no
   `PersistenceUnavailable`, `RestoreQuiesce`, or `OrchestratorShutdown` callback
   fired (cover modern and legacy callback wiring), no success/restart result,
   no remaining `.new`, and no changes to source backup bytes. Then query current
   data and execute/read a new write through the original pool. This establishes
   both unchanged persisted work and continued pool usability.
3. Run the exact RED command below. Confirm failure is the unexpected successful
   restore/disruption, not fixture setup or compilation. Keep the original handle
   and actual exit. Do not repeatedly replay destructive assertions.
4. In `store.go`, verify the synced/closed actual `stagedPath` before all hooks;
   remove owned staging and return a contextual error on failure. Reuse existing
   `verifySnapshot` without changing its retention behavior or adding a hook just
   to make tests pass.
5. Add `TestRestoreValidSnapshotReplacesNewerData`: saved and current rows differ;
   public job succeeds, modern callbacks run once, pool closes, result requires
   restart, reopened restored DB contains saved data and excludes newer work.
   Register cleanup immediately for any opened reader. Strengthen existing
   `TestRestore_WrongConfirmReturnsError` with newer-data/pool-usability evidence.
6. Adapt `path_test.go` tests
   `TestConfiguredDatabasePath_RestoreReplacesConfiguredFilename` and
   `TestConfiguredDatabasePath_RestoreQuiescesPoolAndReplacesWALSidecars` to use
   valid production-created SQLite snapshots and assert restored SQL sentinels.
   Preserve configured filename/default-path absence and legacy shutdown checks.
   Change `TestRestoreCheckpointBusyLeavesDatabaseAndSidecarsUntouched` to use a
   valid snapshot so it reaches checkpoint-busy; require checkpoint/busy error
   evidence, not only failed state. Keep the held-reader barrier and pool checks.
   `TestRestoreReplacementFailureRestoresOriginalFiles` calls the replacement
   helper directly; its byte fixture remains appropriate and need not change.
   Existing `TestRestore_WriteFailurePreservesOriginal`, PostgreSQL rejection,
   handler confirmation, and valid restore tests remain in the package GREEN run.
7. Update the two public docs as specified in the plan. Run GREEN and required
   scoped persistence checks, then canonical initial changed-Go lint serially.
   Record actual commands/exits and any justified later scope changes.

## Files likely touched and A/M coverage

- M `apps/backend/internal/system/backups/store.go`: staged verification and error cleanup.
- A `apps/backend/internal/system/backups/restore_integrity_test.go`: invalid matrix and distinct-data valid control.
- M `apps/backend/internal/system/backups/store_test.go`: strengthen wrong-confirmation control.
- M `apps/backend/internal/system/backups/path_test.go`: valid SQLite fixtures and checkpoint-specific assertion.
- M `apps/backend/internal/backendapp/startup_database_test.go`: replace the bootstrap restore success fixture with a valid SQLite snapshot while preserving process/readiness/quiescence/result/shutdown assertions.
- M `docs/public/operations.md`: validation-before-shutdown order and rejection recovery.
- M `docs/public/configuration.md`: matching configured-path restore order and rejection behavior.
- M `docs/specs/system-page/requirements/system-page.md`: owning AC amendment.
- M `docs/specs/system-page/system-design/system-page-01.md`: staged-validation ordering and failure invariant.
- A `docs/plans/reject-damaged-system-backups/plan.md`: manifest/results.
- A `docs/plans/reject-damaged-system-backups/task-01-validate-staged-restore.md`: this work order/results.

`retention_preparation.go` is read/reused, not expected to change. If implementation
requires another path, assess scope and add its explicit coverage before delivery.

## Verification

Run only after explicit implementation release and a granted heavy lease. Use
Go 1.26 local (`GOTOOLCHAIN=local`, verify `go version`), task-local Node 24.21.0
on PATH, and serial commands. Each `scripts/run-quiet` invocation, if used, needs
its own task-owned `KANDEV_RUN_QUIET_DIR`; retain every original session handle
through actual join/exit, then record current owned cleanup. Do not use shared
log pruning, cache wipes, foreign kills, or an ungranted resource escalation.

From the repository root:

```bash
# RED: only after independently adding the invalid-snapshot regression.
(cd apps/backend && GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -race -trimpath -tags fts5 -p 1 ./internal/system/backups -run '^TestRestoreRejectsInvalidSnapshotBeforeDisruption$' -count=1)
# GREEN: all existing backup, handler, retention-verifier, path and new tests.
(cd apps/backend && GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -race -trimpath -tags fts5 -p 1 ./internal/system/backups -count=1)
# Required for persistence changes, even though schema/requiredstores are unchanged.
(cd apps/backend && GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB go run -trimpath -tags fts5 -p 1 ./cmd/sqlguard ./internal)
(cd apps/backend && GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -race -trimpath -tags fts5 -p 1 ./internal/persistence/storeconformance -count=1)
# INITIAL canonical changed-Go lint; default 3GiB only with actual host capacity.
bash scripts/lint-go-changed apps/backend/internal/system/backups/store.go apps/backend/internal/system/backups/restore_integrity_test.go apps/backend/internal/system/backups/store_test.go apps/backend/internal/system/backups/path_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/reject-damaged-system-backups
```

sqlguard/storeconformance apply because this changes the destructive persistence
restore path; run the repository-required checks rather than assuming that no
schema change exempts them. No PostgreSQL schema/dialect method changes, so no
new live PostgreSQL fixture is needed; existing rejection coverage applies.
CI owns full backend lint; do not run broad local suites. A fresh worktree needs
one `pnpm install --frozen-lockfile` from `apps/` before any pnpm command/hooks
under the granted lease. No install occurs during design. Normal commit hooks
may require it even though no web tests are planned.

Before completion rerun the documentation coverage preflight using the exported
`validateCoverage` in `.github/scripts/pr-docs.cjs`, actual A/M changed paths
(including untracked paths), and current requirement/design/work-order contents.
The design checkpoint records its concrete local invocation externally. Inspect
all outputs and record results; no hosted status mutation during design.

## Dependencies

None. Implementation is gated on ROOT's later explicit release. ROOT granted the single global local-heavy lease to this task for implementation
and validation. Serial merge authority remains separate.
No callback or approval question is requested at the completed-plan checkpoint.

## Risks

Integrity verification includes the existing file hash scan. Preserve error
cleanup and the maintenance gate. Compare live sidecar bytes before issuing
post-job SQL probes. Do not weaken downstream controls into early-invalid tests.

## Parallelism

`sequential`; same primary session, no native delegates or new Kandev sessions.

## Inputs

- `AC-SYSTEM-PAGE-SYSTEM-PAGE-001.9` and the paired System pages part 1 design.
- `store.go`, `retention_preparation.go`, public backup tests, and the backend TDD reference.
- Adjacent payload retention design: shared maintenance admission and read-only verifier.
- External recovery/checkpoint record at `/tmp/child149-reject-damaged-snapshots/recovery-plan.md`.

## Results

Implemented after explicit release. Production invokes the existing verifier against the staged path before
all disruptive callbacks and deletes staging on error. Staged copying also
rejects zero bytes because SQLite integrity checking accepts an empty file. All ten A/M paths
above are present; no additional production owner changed.

- RED: all six invalid cases failed with actual succeeded versus expected failed;
  fixtures independently verified invalid. Wrong-cwd setup failure excluded.
- Final GREEN used the exact test options above with both backup and
  storeconformance package paths in one serial `-p 1` invocation: exit 0,
  package times 1.392s and 55.355s. This follows the last Go/test edit.
- sqlguard: exit 0. Canonical initial changed-Go lint: exit 0, zero issues.
- Catalog validation: exit 0 (388 decisions, 1656 specifications).
  Specification lint, actual ten-path documentation coverage, and diff check:
  exit 0. Coverage reports covered without reference errors.
- Updated configured-path success/checkpoint fixtures use valid snapshots;
  checkpoint-busy requires its specific error. Modern/legacy valid controls
  prove callbacks are reachable and restored SQL contains saved data.
- Public restore ordering/failure guidance updated in both owning pages.
  No UI, mobile composition, or translations changed.

Operational handles, install/hook receipts, current head, leases, and hosted
CI/review budgets remain in the external Kandev task plan and recovery record.
No production/permanent tests/install/heavy command/commit/push occurred before
the design checkpoint; subsequent delivery uses normal hooks.

### Zero-byte correction

A later independently authored public-Restore regression on the initial published
fix failed both modern/legacy cases: expected failed, actual succeeded. SQLite's
shared verifier correctly reflects its permissive empty-file semantics, so the
restore copy now rejects zero copied bytes without changing shared retention or
schema contracts. This extends the same invalid matrix and acceptance criterion.
Targeted RED exit 1. Corrective full backup-package GREEN exit 0 (1.452s)
with Go 1.26.0/race/trimpath/fts5/p1/GOMAX2/GOMEM512MiB, inner240/GNU280/kill10.
Scoped changed-Go lint over `store.go` and `restore_integrity_test.go` exit 0,
zero issues (GNU360/kill10). Earlier passing sqlguard/conformance are retained for this local
pre-disruption guard; no SQL/schema/repository changes were introduced.

### Required CI startup-fixture correction

Current-head CI identified an additional affected owner:
`TestBootstrapRestoreKeepsProcessAliveUntilResultAndShutdown` writes non-SQLite
`after` bytes into `manual-restore.db`. Integrity admission correctly rejects
that placeholder before the test's blocked quiesce boundary. The existing
lifecycle assertion fails at line 367, so it is not valid success-path evidence
until the fixture becomes a real SQLite database. No lifecycle policy change is
required. Preserve all process/readiness/shutdown assertions; do not weaken the
barrier or bypass restore verification.

ROOT accepted the exact hosted startup failure as causal RED. Do not replay
the invalid fixture locally. After the focused local-heavy grant, validate the
prepared real SQLite snapshot fixture, created through production persistence
helpers with cleanup registered immediately, using the same targeted test
and scoped changed-Go lint. Add no production seam. Commands from repo root:

```bash
(cd apps/backend && GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -race -trimpath -tags fts5 -p 1 ./internal/backendapp -run '^TestBootstrapRestoreKeepsProcessAliveUntilResultAndShutdown$' -count=1 -timeout=240s)
bash scripts/lint-go-changed apps/backend/internal/backendapp/startup_database_test.go
```

Use GNU timeout 280 seconds/kill-after 10 for the targeted test and 360 seconds
for lint. Retain earlier unchanged backups, sqlguard, and conformance evidence.
The fixture-only edit does not change the startup lifecycle contract or require
a second product specification. ROOT reviewed and released this fixture correction. The original hosted
failure is accepted as causal RED; no invalid fixture was replayed locally.
Targeted startup GREEN passed (1.156s), bounded Go 1.26.0 local/race/trimpath/
fts5/p1/GOMAX2/GOMEM512MiB, inner240/GNU280/kill10. Canonical scoped changed-Go
lint passed with zero issues, GNU360/kill10, default3GiB after checking actual
capacity (~8GiB available). Catalog/spec/diff and actual full-candidate 11-path
documentation coverage passed. No backup/sqlguard/conformance replay or reinstall.

The separate hosted ledger cap failure remains diagnostically unresolved;
no ledger file is changed by this work order. Exact CI-tested merge/current-main/
candidate source comparison is byte-identical for calls, its test, fixtures,
queue/lifecycle helpers. No-op fixture logging limits inference from absent
write-error logs. One bounded local diagnostic is tracked externally; its result
cannot establish a hosted root cause. Subsequent hosted CI/review evidence stays
external and must pass before READY.
