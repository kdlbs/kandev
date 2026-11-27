---
status: current
system: platform
requirements:
  - REQ-PLATFORM-GO-DEV-LAUNCHER-001
created: 2026-08-26
owners:
  - kandev
---
# Go dev launcher and startup safety System Design

## Purpose and boundaries

The platform launcher owns the startup banner shown by `dev`, `start`, and
`run`, and the pre-launch production-database snapshot guard for `dev`. Platform
owns this contract because it controls admission before development children
can touch the selected database. System-page backup/restore policy remains
separate. The guard supplements existing database-target and runtime ownership
checks; it does not permit two backends to own the same runtime state.

The launcher consumes build metadata from the native binary entrypoint. It does
not resolve a version from the database, the selected bundle directory, or an
environment variable used only for release display metadata.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLATFORM-GO-DEV-LAUNCHER-001` (.1-.9) | [Build identity](#build-identity) and [Startup flow](#startup-flow) |
| `REQ-PLATFORM-GO-DEV-LAUNCHER-001` (.10-.16) | [Production snapshot adapter](#production-snapshot-adapter) |

## Components and responsibilities

- `apps/backend/cmd/kandev` owns the build-time `Version` variable and passes it
  into `launcher.BuildInfo`.
- `internal/launcher.Run` selects the command and forwards the build version to
  the selected launch path.
- `internal/launcher.runStart` and `internal/launcher.runInstalled` carry the
  version in `managedAppConfig`.
- `internal/launcher.runDev` carries the version through the dev launch path.
- `internal/launcher.logStartup` renders the shared stdout banner.

## Build identity

`launcher.BuildInfo.Version` is the canonical value for the launcher banner. It
is the same value used by the native `--version` path and by the backend build
metadata when the binary is dispatched with `__backend`. The compiled default
is `dev`, so a local or otherwise unstamped binary never prints an empty version.

The existing `KANDEV_VERSION` value remains optional release display metadata for
the installed-release header. It does not override the binary build identity
shown on the `version:` line.

## Startup flow

Each shared launch mode resolves ports, endpoints, database path, and log level
before it calls the banner. The banner prints the existing header first, then a
single line in this shape:

```text
[kandev] version: <build version>
```

The line appears before the URL, network, MCP, database, and log-level lines.
Those existing lines retain their current order and values. The version line is
printed before child startup so it remains available when backend readiness
fails.

## Failure and recovery

If build metadata is empty at an internal call boundary, the launcher uses the
same `dev` fallback as the native binary defaults. Version rendering cannot
prevent launch, change endpoint selection, or change backend readiness behavior.

## Observability

The startup banner exposes the build identity on launcher stdout. `--version`
remains the machine-readable single-value command. The backend `/health` and
`/api/v1/system/info` version contracts remain independent but use the same
build metadata for the dispatched backend process.

## Production snapshot adapter

### Existing behavior and repair boundary

Before this repair, `internal/launcher/backup.go:backupProductionDb` streamed only
the main file with `io.Copy`. `runDev` calls `backupProductionDBIfNeeded` before
the startup banner, supervisor construction and backend launch, but a successful
file copy can omit committed WAL data. The earlier public raw-copy limitation
was explicit; WAL completeness is a strengthened contract, not a claim about
previous behavior.

Keep the adapter in `internal/launcher`. Reuse `internal/db.BackupSQLite` and
`db.EscapeSQLiteURIPath`, following the existing
[native page-copy design](sqlite-snapshot-copy.md) and
[backup/compaction decision](../../../decisions/2026-10-12-separate-sqlite-backups-from-compaction.md).
Do not call persistence/bootstrap constructors, config discovery, migrations or
the application writer opener. Do not modify shared backup/restore policy or
introduce another backup engine. Native copying preserves free pages; compaction
is outside this contract. This local consumer adoption needs no new ADR.

### Source admission and context

Thread `runDev`'s context through `backupProductionDBIfNeeded` and
`backupProductionDb`; all caller and test signatures change together. Derive a
five-minute maximum snapshot context, inheriting any earlier caller deadline.
This is a local safety bound, not a new operator setting. Check cancellation
before database admission and again before publication. Copy and verification
use that same context. Native copy checks between bounded page steps and limits
no-progress retries to five seconds; SQLite connection busy waits can still take
up to the existing five-second busy timeout. Cancellation is not a promise to
interrupt arbitrary filesystem calls immediately.

Inspect the source first: only genuine absence retains `"", nil`; other path
errors fail. Open the existing absolute file with an escaped `file:` URI,
`mode=ro` and `_busy_timeout=5000`, with one connection for this short-lived
operation. At the qualified baseline `db.OpenSQLiteReader` does not escape URI
delimiters. Use a small launcher-local `sql.Open` adapter with the shared escape
helper rather than changing that reader's behavior for unrelated consumers.
Do not use `immutable=1`, disable locking, change journal mode or issue a source
checkpoint. SQLite owns WAL visibility through the normal reader connection.

Read-only WAL access can require existing readable sidecars or permission for
SQLite's transient WAL-index bookkeeping. Lack of those capabilities fails
closed. Source preservation means no main-file or WAL writes by the adapter;
it does not forbid SQLite reader locks or `-shm` bookkeeping. Offline fixtures
must retain the committed WAL at their owned source path. Closing the fixture's
last writer before constructing that image would checkpoint away the regression.
See [SQLite read-only WAL rules](https://www.sqlite.org/wal.html#read_only_databases).

### Temporary snapshot signal owner

`launcher.Run` supplies a background context, and `runDev` intentionally admits
the snapshot before supervisor construction and `attachSignalsFn`. Without a
temporary owner, normal Ctrl-C during this phase exits by the OS default action
and skips the staging defer. AC-PLATFORM-GO-DEV-LAUNCHER-001.16 extends cooperative
cancellation to this ordinary CLI interruption path.

Only `backupProductionDBIfNeeded`, after the `.kandev-dev` skip, derives a
temporary `signal.NotifyContext` child from its caller for `os.Interrupt` and
`syscall.SIGTERM`. Pass that child to `backupProductionDb`; its existing five-minute
deadline remains in force. Always stop this notification owner before returning
from the guard, after backup/verification handles and staging cleanup have
finished. A canceled attempt returns an error, so `runDev` returns non-zero before
supervisor creation or child launch. Successful admission likewise unregisters
the temporary owner before the existing supervisor registers its signals.

Do not change `launcher.Run`, installed/start/service contexts, process groups or
`processSupervisor.attachSignals`. Its supervised second-signal behavior stays
with that existing owner. Repeated signals during the snapshot phase request the
same bounded cancellation; they do not create another process supervisor. Native
SQLite busy waits retain their existing bound, so interruption need not be
instantaneous. SIGKILL, power loss and abrupt process death can leave private
staging; do not add a crash scavenger or delete historical/unknown directories.

Prove the normal CLI path in a Unix-only isolated test helper process, never by
signaling the parent test process. Use the existing launcher `TestMain` to dispatch
an exact private argument plus test-only environment marker into a fixture-only
helper. It uses a `t.TempDir()` repo/config/home and real SQLite source; the existing
copy seam pauses after writing an owned partial staged file, signals readiness
over an inherited pipe, then waits on the real copy context's cancellation.
The parent sends SIGINT only after that barrier, joins the helper and requires
non-zero dev exit, no backend/supervisor launch, no partial staging or old-history
pruning, and unchanged source bytes/rows. Bound the helper and joins with context
timeouts; do not sleep or start a live Kandev backend. On the old code, SIGINT kills
that helper before its report/cleanup, providing causal RED. A successful helper
control proves snapshot notification is stopped before the existing supervisor's
injected registration boundary. Existing callback-cancel/deadline and full
launcher tests cover caller cancellation and unchanged supervision integration.

Windows lacks Go's Unix `Process.Signal(os.Interrupt)` transport for this private
helper. Keep its actual OS-signal regression Unix-only; platform-neutral guard
fixtures isolate both `HOME` and `USERPROFILE`, and hosted Windows launcher jobs
cover compilation and the ordinary admission paths. No test signal or kill may
target a developer's or another task's process.

### Copy, verification and publication

1. Keep `backupTimestamp`, `backupPrefix`, `backupSuffix`, the user-home backup
   location and existing `pruneBackups` policy. Create/harden the backup directory
   to `0700`, failing if the required hardening fails.
2. Create one owned `0700` temporary directory within that backup directory.
   Invoke `db.BackupSQLite(ctx, source, stagedPath)` there. The primitive exclusively
   creates a `0600` destination, copies with native SQLite page steps and DELETE
   destination journaling, closes it, and cleans failed owned output.
3. Open the closed staged file read-only using the escaped URI adapter. Run full
   `PRAGMA integrity_check` and require exactly one `ok` result. Close all reader
   rows and source/verification handles before publication; propagate errors.
4. Stamp the staged file with the injected timestamp and confirm the context is
   still live. Publish without replacement using the existing local
   `os.Link(stagedPath, finalPath)` pattern from
   `internal/persistence/sqlite_selection.go`. Same-directory staging keeps this
   on one filesystem. An existing destination is an error, never an overwrite.
   Do not add a generic atomic-installation framework or rename-overwrite fallback.
5. Remove the owned staging directory. Only a verified, installed snapshot can
   return its final path, print the existing success message and trigger pruning.
   All failures before installation preserve old backups, skip pruning and
   clean only owned staging. Never remove a collided destination or source
   sidecars. Report cleanup failure with the attempt error where applicable.

The final-path link is the publication boundary. Cleanup of the staging alias
after successful installation does not invalidate the verified final file;
report any cleanup issue separately without describing the snapshot as corrupt.
Unsupported hard links, source locking, disk space, integrity errors and
deadline expiry fail before child startup. Runtime owner checks remain separate.
The non-CGO `BackupSQLite` stub fails without creating output; never fall back
to raw copy. No schema, storage format, runtime flag or dependency changes apply.

### Evidence and surfaces

Launcher tests use real SQLite schema and distinguishable sentinel rows, then
independently open only the resulting `.db`. Cover open WAL, a stable offline
WAL image, rollback journal, URI-sensitive paths, source byte/data preservation,
absence, invalid input, exclusive collisions, cancellation, failed-copy and
failed-verification cleanup, retention and foreign families. The `runDev` guard
uses the existing injected launch seams and never starts a real process.

This is backend/CLI-only: no rendered UI, mobile interaction, screenshots,
Playwright flow or localized product copy changes. CLI diagnostics remain English
under the backend guidance. Public remote-environment instructions change with
implementation: replace the obsolete raw-copy limitation with the complete
snapshot contract, retain deliberate dev/production intent, independent backup
advice and the exclusive runtime ownership guard.

## Implementation plans

- [Original Go dev launcher](../../../plans/go-dev-launcher/plan.md) records the
  completed raw-copy migration; its done work orders remain historical evidence.
- [Complete dev production snapshots](../../../plans/dev-production-sqlite-snapshots/plan.md)
  owns this bounded repair and its verification results.
