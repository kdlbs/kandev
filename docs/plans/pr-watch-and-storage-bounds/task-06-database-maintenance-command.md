---
id: "06-database-maintenance-command"
title: "Provide safe database retention maintenance"
status: done
wave: 3
depends_on: ["04-github-auth-backoff-observability", "05-bounded-history-storage"]
plan: "plan.md"
spec: "../../specs/platform/pr-watch-and-storage-bounds.md"
---

# Task 06: Provide safe database retention maintenance

## Intent

Give operators a backup-gated command to report, execute, compact, and roll back bounded-storage cleanup.

## Acceptance

- The native maintenance database command supports dry run, explicit retention settings, per-table candidate/reclaim estimates, and fails closed when a verified backup is absent.
- Execution removes only selected redundant payloads/snapshots/revisions in transactions, stages/validates compaction with VACUUM INTO or equivalent, atomically replaces only after success, and prints rollback steps.
- The command reports SQLite/WAL and table-storage measurements without contents, credentials, or destructive default behavior.

## Files likely touched

- apps/backend/internal/maintenance/*.go (new package: analyze/execute/backup/compact/run)
- apps/backend/internal/launcher/maintenance_database.go (new CLI subcommand)
- apps/backend/internal/launcher/launcher.go (dispatch wiring)
- apps/backend/internal/task/repository/sqlite/message_payload.go (`ListOrphanedMessagePayloadCandidates`)
- apps/backend/internal/task/repository/sqlite/base_queries.go (shared candidate-query helper)
- apps/backend/internal/task/repository/sqlite/git_snapshot_digest.go (reuses shared helper)
- apps/backend/internal/persistence/snapshot.go (reused, not modified)
- docs/public/cli.md
- New `*_test.go` files in the above packages

## Dependencies

Tasks 04 and 05.

## Parallelism

Sequential. It consumes finalized retention data and health/configuration contracts.

## Verification

~~~bash
cd apps/backend && go test ./internal/maintenance/... ./internal/launcher/... ./internal/task/repository/sqlite/... -run 'Test.*(Maintenance|Retention|DryRun|Backup|Vacuum|Compact|Rollback|Orphaned|Duplicate).*' -count=1 -v
cd apps/backend && go test ./internal/maintenance/... ./internal/launcher/... ./internal/task/repository/sqlite/... -count=1
cd apps/backend && go test ./internal/maintenance/... ./internal/launcher/... ./internal/task/repository/sqlite/... -race -count=1
cd apps/backend && go build ./... && go vet ./...
cd apps/backend && gofmt -l internal/maintenance internal/launcher internal/task/repository/sqlite
cd apps/backend && golangci-lint run ./internal/maintenance/... ./internal/launcher/... ./internal/task/repository/sqlite/...
git diff --check
~~~

## Output contract

Report dry-run versus execution row/byte counts, backup verification, staged-compaction failure preservation, successful restore/rollback, command output redaction, and test results. Update task and plan status.

## Results

**Status: done.**

Implemented as a new offline `internal/maintenance` package (`maintenance.go`,
`report.go`, `execute.go`, `backup.go`, `compact.go`, `run.go`) plus the
`kandev maintenance database` CLI subcommand
(`internal/launcher/maintenance_database.go`, dispatched from
`internal/launcher/launcher.go`). Added the missing Task-05-adjacent
`ListOrphanedMessagePayloadCandidates` query
(`internal/task/repository/sqlite/message_payload.go`) so the "stale
payload" retention category has real data to report/delete, and extracted a
shared `listLimitedCandidates` generic helper
(`internal/task/repository/sqlite/base_queries.go`) reused by both the
orphaned-payload and duplicate-git-snapshot listers to keep them
lint-clean under `dupl`.

**Acceptance:**
- Dry run (no `--execute`) never acquires a lock, takes a backup, or
  mutates anything; it reports row/byte estimates per category
  (duplicate git snapshots, obsolete plan revisions, orphaned message
  payloads) plus current database size. Verified by
  `TestRunOptionsDryRun...`-style tests in `internal/maintenance` and the
  CLI-level `TestRunMaintenanceDryRunEndToEndReportsCandidates`.
- `--execute` takes the same `ownershiplock` exclusive-access lock a live
  backend takes at boot (refuses to run concurrently with `kandev`),
  always takes and verifies a fresh `VACUUM INTO` backup before deleting
  anything, and performs all retention deletes inside one transaction.
  Verified by ownership-conflict-refusal, snapshot-failure-abort, and
  execute+idempotency tests in both `internal/maintenance` and the
  CLI-level `TestRunMaintenanceExecuteRefusesWhileOwnershipLockHeld` /
  `TestRunMaintenanceExecuteEndToEndDeletesAndBacksUp`.
- `--compact` stages a `VACUUM INTO` compacted copy after retention
  deletes commit, verifies it with an integrity check, requires
  sufficient free disk, and atomically replaces the live database,
  preserving a `.pre-compact-<timestamp>.bak` rollback artifact. A
  compaction failure after committed deletes returns a partial-success
  `Outcome` (populated backup path/delete counts) rather than a total
  abort. Verified by insufficient-disk-refusal, integrity-check-failure,
  and execute+compact+rollback tests.
- Reconciliation/candidate selection is read-only and non-destructive
  until `--execute`; normal conversation rows, current plan HEAD, revert
  ancestry, and protected snapshots are never candidates (candidate
  queries only ever select superseded/orphaned rows).
- No secrets, tokens, branches, task IDs, or payload contents appear in
  CLI output or logs; the CLI prints only structural counts, byte
  estimates, and file paths.

**Tests:** 8 focused tests in `internal/maintenance/run_test.go` (dry-run
non-mutation, unsupported-driver refusal, ownership-conflict refusal,
execute+backup+idempotency, execute+compact+rollback,
insufficient-disk-space refusal, snapshot-failure abort,
integrity-check-failure abort) plus 2 tests in
`internal/task/repository/sqlite/message_payload_retention_test.go` and 9
new tests in `internal/launcher/maintenance_database_test.go` (arg-parsing
unit tests + help/unknown-target + 3 end-to-end CLI-path tests covering
dry run, execute+backup, and ownership-lock refusal). All pass, including
under `-race`.

**Verification commands run** (from `apps/backend`):
- `go build ./...` — clean.
- `go vet ./...` — clean.
- `go test ./internal/maintenance/... ./internal/task/repository/sqlite/... -race -count=1` — `ok` both packages.
- `go test ./internal/launcher/... -race -skip 'TestRunDevAbortsOnBackupFailureBeforeLaunch|TestRunDevLaunchesBackendThenWebAndOpensBrowser|TestRunDevUsesConfiguredHealthTimeoutForBackendAndWeb' -count=1` — `ok` both `internal/launcher` and `internal/launcher/cli`.
- `gofmt -l` on all new/changed files — no output (clean).
- `golangci-lint run ./internal/maintenance/... ./internal/launcher/... ./internal/task/repository/sqlite/...` — `0 issues`.
- `git diff --check` — clean.

**Deliberate deferral / pre-existing issue (not part of this task's
scope):** `TestRunDevAbortsOnBackupFailureBeforeLaunch`,
`TestRunDevLaunchesBackendThenWebAndOpensBrowser`, and
`TestRunDevUsesConfiguredHealthTimeoutForBackendAndWeb` in
`internal/launcher/dev_test.go` fail on this Go toolchain
(`go1.26.0`) with a nil-pointer panic in
`restartableBackend.Exited`/`waitForHealth`, reproduced identically
against the pre-Task-06 commit
(`e0768ba67423c5563f5c27d514076a4f053e80e1`) with this task's new files
moved aside — confirmed pre-existing and unrelated to this change, not
introduced or fixed by Task 06.

`internal/system/storage` (the live, HTTP-triggered System→Database
maintenance UI) and `internal/persistence` (boot-time version-change
snapshot) were deliberately left unmodified: Task 06 is a separate,
offline/exclusive CLI path that reuses `persistence.SnapshotSQLite` and
the same `ownershiplock` targets a live backend uses, but does not extend
either existing package, per the plan's separation of the "live" and
"offline" maintenance concepts.


