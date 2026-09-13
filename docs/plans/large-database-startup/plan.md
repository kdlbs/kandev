---
created: 2026-09-11
status: complete
requirements:
  - REQ-PLATFORM-STARTUP-LIFECYCLE-001
system_design:
  - ../../specs/platform/system-design/startup-lifecycle.md
legacy_specs: []
---

# Large database startup implementation plan

## Root cause

`provideRepositories` performs the upgrade backup and migrations before
`startGatewayAndServe` binds the bootstrap listener. The reported 42.5-second
backup consumes almost all of the 45-second listener budget.

## Scope and approach

Move shared bootstrap ownership before initialization. Add typed phase reporting
and cancellation-aware backup execution. Preserve version gates, retention,
health identity, binding semantics, and background recovery ordering.

## Work orders

- [x] [01: Startup lifecycle](task-01-lifecycle.md)
- [x] [02: Startup measurements and documentation](task-02-evidence.md)

## Verification

Backend commands run from `apps/backend`; script and diff commands run from root.

- `rtk go test -race ./internal/backendapp ./internal/persistence ./internal/launcher ./internal/common/config ./internal/db`
- `rtk go test ./internal/task/repository/sqlite -run 'Test(StartupMigrationCosts|StartupMigrationCostsPopulated|NewWithDBContextStopsActiveMigration|BackfillPromptSeqStopsOnCanceledContext)' -v`
- `rtk proxy python3 scripts/lint-spec-files.py --all`
- `rtk proxy node --test scripts/validate-public-docs.test.mjs`
- `rtk proxy node scripts/validate-public-docs.mjs`
- `rtk git diff --check`

## End-to-end evidence

Use real HTTP listeners with controlled initialization and short launcher health
deadlines. Cover success, failure, route isolation, cancellation, and bind failure.
No browser UI changes or live database access are needed.

## Risks

Legacy constructors include non-context SQL. Shutdown must not race cleanup with
those constructors. Retry retention can discard the original snapshot; no reuse
or retention changes are authorized in this package.

## Results

Implemented early bootstrap ownership, typed startup phases, cancellation-aware
SQLite backup and task migration execution, worker-only restore quiescing,
launcher phase diagnostics, populated startup measurements, and public
troubleshooting documentation.

Validation passed:

- `env -u KANDEV_INTERNAL_CONFIG_FILE -u KANDEV_INTERNAL_CONFIG_HOME_FILE -u KANDEV_INTERNAL_AGENTCTL_STARTUP_CONFIG -u KANDEV_HEALTH_TIMEOUT_MS go test -race ./internal/backendapp ./internal/persistence ./internal/launcher ./internal/common/config ./internal/db -count=1`
- `go test -race ./internal/startup -count=1`
- `go test -race ./internal/persistence/storeconformance -count=1`
- `go test ./internal/task/repository/sqlite -run 'Test(StartupMigrationCosts|StartupMigrationCostsPopulated|NewWithDBContextStopsActiveMigration|BackfillPromptSeqStopsOnCanceledContext)$' -v -count=1`
- `make -C apps/backend build`
- `make -C apps/backend lint` (0 issues)
- `go run ./cmd/sqlguard ./internal`
- `python3 scripts/lint-spec-files.py --all`
- `node --test scripts/validate-public-docs.test.mjs` (62 passed)
- `node scripts/validate-public-docs.mjs` (46 published pages)
- `git diff --check`

The populated isolated SQLite evidence is recorded in
[task-02-evidence](task-02-evidence.md). It covers a 929,792-byte database,
6,000 messages, a 2,252,800-byte `VACUUM INTO` snapshot, 285.06 ms replay,
and separate recurring backfill timings on Linux amd64 with Go 1.26.0. The
measurement remains synthetic and does not predict production startup time.

Deferred risks remain outside this package: backup retry retention and the
backup-registry redesign are unchanged. Constructors for non-task stores still
have bounded admission barriers rather than a new shared context API; an
already admitted statement can finish according to its driver's cancellation
behavior before the next store is rejected.
