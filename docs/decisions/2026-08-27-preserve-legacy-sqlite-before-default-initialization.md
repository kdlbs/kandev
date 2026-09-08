# ADR-2026-08-27-preserve-legacy-sqlite-before-default-initialization: Preserve Legacy SQLite Data Before Default Initialization

**Status:** accepted
**Date:** 2026-08-27
**Area:** backend, cli, operations

## Context

The Go launcher resolves the current default SQLite path before starting the backend. When it publishes that derived value as `KANDEV_DATABASE_PATH`, the backend classifies the path as explicit and skips its legacy-location migration. `db.OpenSQLite` then creates an empty database at the current default path. The application appears to have lost data even though the previous database can remain at the legacy path.

The same installation can later contain both the new initialized database and the legacy database. Automatically switching between them based only on timestamp or file size can hide valid data from the other.

## Decision

Database path provenance must survive the launcher-to-backend boundary. A derived default is not an explicit override.

Before a writable SQLite open can create the default target, persistence checks the known legacy default location. When the target is absent and the legacy database is valid, Kandev adopts it through a validated staged SQLite snapshot and retains the source. It does not rename or delete the source database or its sidecars.

When the current default database has no task history and the legacy candidate has task history, Kandev fails startup without modifying either candidate. Explicit operator-selected paths bypass legacy discovery. Kandev does not automatically switch away from an established current database that contains task history or merge divergent histories.

## Consequences

- Default upgrades preserve existing data instead of silently initializing an empty database.
- Adoption requires additional startup I/O and enough disk space for a second database file.
- The legacy source remains on disk until the operator removes it after verification.
- Some installations with a deliberately empty current database and retained legacy task history must resolve the conflict explicitly before startup.
- Startup diagnostics must identify path source and candidate state without exposing stored content.
- Cross-process tests must cover launcher environment construction through persistence selection.

## Alternatives Considered

### Continue initializing the current default path

Rejected because the application starts successfully against an empty database and presents the incident as data loss.

### Rename the legacy database into place

Rejected because a partial sidecar move or later validation failure can make the only known source harder to recover.

### Always prefer the legacy database

Rejected because the current target can contain newer valid data and explicit operator intent must remain authoritative.

### Choose by modification time or file size

Rejected because neither value proves which SQLite history is authoritative, especially with WAL files and copied backups.
