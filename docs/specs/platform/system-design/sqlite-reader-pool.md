---
status: current
system: platform
requirements:
  - REQ-PLATFORM-SQLITE-READER-POOL-001
---

# SQLite Reader Pool System Design

## Purpose and boundaries

Platform owns the shared SQLite persistence factory in `internal/db` and the
stable startup configuration catalog in `internal/common/config`. This design
adds one startup-only sizing input for the read-only pool. It does not change
the writer pool, the WAL or durability pragmas, the required-store health
probe, or the shipped default of four readers.

The [required-store contract](postgres-domain-store-parity.md#runtime-health)
and the
[SQLite writer-admission decision](../../../decisions/2026-10-05-sqlite-writer-transaction-admission.md)
remain authoritative. The [SQLite file hygiene design](sqlite-file-hygiene.md)
continues to own the WAL bound and boot staging cleanup.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-PLATFORM-SQLITE-READER-POOL-001` | Configuration ownership; Factory contract; Failure boundaries; PostgreSQL |

## Configuration ownership

`database.sqliteReaderConns` is a stable operator setting with the compatible
environment alias `KANDEV_SQLITE_READER_CONNS`. It follows the existing startup
catalog and validation conventions:

- The built-in default is four.
- Accepted configured values are integers from one through 64.
- A configured-file value outside that range fails startup validation and names
  the selected file and key when the SQLite driver is selected.
- An environment value is parsed strictly when the SQLite driver is selected;
  an unparseable or out-of-range value fails startup instead of falling back to
  the default. An explicitly present empty environment value is invalid, not
  absent.
- Validation is driver-scoped. A PostgreSQL selection keeps its existing
  startup behavior even if the SQLite-only key or environment variable is
  invalid; the value is not used by the PostgreSQL provider.
- The value is resolved once during `config.Load`. It is not watched, polled, or
  exposed as a live runtime setting.

The setting is SQLite-only. PostgreSQL keeps its existing shared pool and the
`database.maxConns` and `database.minConns` settings.

## Factory contract

`db.OpenSQLiteReader` remains the default-four entry point. The new
`db.OpenSQLiteReaderWithConns` takes the resolved size and applies it to both
`SetMaxOpenConns` and `SetMaxIdleConns`, so idle capacity matches the configured
open capacity. A zero value means the default and keeps zero-value internal
configuration fixtures working; negative values and values above the factory
bound are rejected.

`internal/persistence.provideSQLite` passes the typed configuration value to the
new factory. The writer still opens through `db.OpenSQLite` with one connection,
and reader transactions still use deferred/read-only snapshots.

## Failure boundaries

Reader pool sizing is not a health policy. Startup checks, missing required
stores, closed pools, destructive `MarkUnavailable`, and actual query or storage
faults keep their existing immediate fail-closed behavior. In particular, this
design does not add a consecutive-failure threshold, a longer probe timeout, or
a busy-classification bypass.

## PostgreSQL

The PostgreSQL provider continues to use one pool for writer and reader roles.
`database.sqliteReaderConns` has no effect there, and no PostgreSQL behavior
changes with this design. The strict environment parse and configured-file
range check apply only when the selected driver is SQLite, so an unused
SQLite-only value cannot reject a PostgreSQL startup.

## Verification

Focused tests cover the default, the configured and environment override, the
one and 64 boundaries, rejection of unparseable and out-of-range values, the
actual maximum open and idle counts, persistence wiring, and the unchanged
writer size. Existing required-store tests continue to cover immediate
fail-closed behavior.
