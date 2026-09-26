---
status: current
system: platform
requirements:
  - REQ-PLATFORM-STARTUP-CONFIGURATION-PARITY-002
---

# Startup Database Selection Continuity System Design

## Purpose and boundaries

The Platform system owns database selection because it owns startup configuration, launcher-to-backend handoff, and shared operational safety. This design covers SQLite path provenance, legacy default discovery, safe adoption, conflict handling, and startup diagnostics. It does not change PostgreSQL selection, SQLite schema migration, backup retention, database restore, or deliberate custom-path behavior.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-PLATFORM-STARTUP-CONFIGURATION-PARITY-002` | [Path provenance](#path-provenance), [Selection flow](#selection-flow), [Legacy adoption](#legacy-adoption), [Failure and recovery](#failure-and-recovery) |

## Path provenance

`internal/common/config` remains the authority for the effective `database.path` value and its `SettingSource`. The launcher can resolve the default path for display and locking, but it must not turn that derived value into a public `KANDEV_DATABASE_PATH` environment override. An explicit environment value remains inherited. A YAML value remains in the selected typed configuration file and reaches the backend through `KANDEV_INTERNAL_CONFIG_FILE`. Dev-mode isolation continues to pass its repository-local database path as managed child wiring.

The backend classifies a SQLite target as explicit only when `database.path` came from configuration or environment, or when a managed launch mode deliberately supplied an isolated path. A nonempty resolved string alone does not prove operator intent.

## Components and responsibilities

- `internal/launcher.backendEnvForConfig` preserves database-path provenance across the child-process boundary.
- `internal/common/config.ConfigSource` remains the typed source record used by startup consumers.
- `internal/persistence` owns default target discovery, legacy candidate inspection, adoption, and conflict errors before `db.OpenSQLite` can create a file.
- `internal/persistence.SnapshotSQLite` provides a transaction-consistent copy that includes committed WAL contents.
- The existing runtime-state lock protects the Kandev home while selection and adoption run.

## Selection flow

SQLite selection runs before opening the writable target:

1. Resolve the effective target and its source.
2. If the source is explicit, open only that target. Do not inspect or adopt a default-location candidate.
3. For a default-derived target, inspect the current path and `<home>/kandev.db` without creating either file.
4. If neither exists, continue with normal first-install initialization.
5. If only the legacy candidate exists and it is a readable SQLite database, adopt it through the staged-copy flow.
6. If the current target exists, it remains authoritative unless it has no task history while the legacy candidate has task history. That condition is ambiguous and stops startup.
7. If both candidates contain task history, do not switch away from the established current target or merge histories. The current target remains selected and diagnostics identify the retained legacy candidate; operator-directed recovery remains separate.

Candidate inspection is read-only. It reports whether the file exists, whether it is readable as SQLite, whether the `tasks` table exists, and its task count. It does not read task titles, messages, secrets, or other content.

## Legacy adoption

Adoption never renames or deletes the legacy source. Kandev opens it under the already-held runtime-state lock, creates a transaction-consistent snapshot in a private `0700` staging directory beside the current default target, and validates the staged database with SQLite integrity checking. It applies owner-only file permissions where supported and atomically installs the staged file at the absent target without replacing a file that appears late.

Any error removes only the private staging directory and its contents. The legacy database and its WAL and SHM sidecars remain untouched. The normal pre-migration backup then evaluates the adopted target before repository schema initialization.

## Failure and recovery

Startup fails before repository initialization when candidate inspection fails, adoption cannot complete, staged validation fails, or the current target has no task history while the legacy candidate does. The error identifies the selected target, the legacy candidate, and the non-destructive recovery action. It never recommends deleting either file.

An explicit database path bypasses legacy discovery. A user who intentionally selects a new database therefore does not receive a false conflict with the default home.

Recovery tooling for reconciling two databases that both contain task history is out of scope. Operators preserve both files and use the existing backup/restore workflow or a later dedicated storage-doctor command.

## Observability

Startup logs the canonical selected path, its source, whether it existed before opening, and the outcome `fresh`, `existing`, `legacy_adopted`, or `conflict`. Conflict and adoption logs include candidate paths and task counts but no record content. The diagnostic bundle receives these messages through the existing bounded backend log source.

## Verification strategy

Launcher tests prove that default-derived paths do not become environment overrides and that explicit environment, YAML, and dev-mode paths retain their behavior. Persistence tests use real file-backed SQLite databases, including WAL-backed legacy data, to prove staged adoption, source preservation, explicit-path isolation, conflict failure, and replay. A launcher-to-persistence regression test exercises the child environment and typed configuration boundary that the incident exposed.

## Related decisions

- [Startup Configuration Uses One Typed Source Model](../../../decisions/2026-08-20-startup-configuration-source-parity.md)
- [The Go Launcher Owns Every Entrypoint](../../../decisions/2026-08-08-go-launcher-owns-all-launch-modes.md)
- [Lock Runtime State Before Backend Startup](../../../decisions/2026-08-09-exclusive-runtime-state-ownership.md)
- [Preserve Legacy SQLite Data Before Default Initialization](../../../decisions/2026-08-27-preserve-legacy-sqlite-before-default-initialization.md)
