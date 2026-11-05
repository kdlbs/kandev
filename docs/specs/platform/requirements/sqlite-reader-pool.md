---
status: active
system: platform
created: 2026-10-29
updated: 2026-10-29
owners:
  - kandev
---

# SQLite Reader Pool Requirements

## Overview

SQLite WAL mode lets readers proceed alongside a single writer, but the read
pool has a fixed size. Installations that run many concurrent interactive reads
can exhaust that pool. Platform owns the shared SQLite persistence factory and
its startup configuration, so it also owns the safe way to opt into a larger
read pool. This contract does not change the shipped default or the required
store health semantics.

## Requirements

### REQ-PLATFORM-SQLITE-READER-POOL-001: Startup-only reader pool sizing

**Intent:** An operator can opt into a larger SQLite read pool at startup
without changing the default, the writer pool, or the immediacy of storage
failure detection.

#### Acceptance criteria

- **AC-PLATFORM-SQLITE-READER-POOL-001.1:** With no reader-pool override,
  Kandev shall open the SQLite read-only pool with four maximum open
  connections and four maximum idle connections.
- **AC-PLATFORM-SQLITE-READER-POOL-001.2:** When `database.sqliteReaderConns`
  or `KANDEV_SQLITE_READER_CONNS` supplies an integer from 1 through 64,
  Kandev shall open the SQLite read-only pool with that value as both the
  maximum open and maximum idle connection count. An environment value shall
  win over a configured file value.
- **AC-PLATFORM-SQLITE-READER-POOL-001.3:** When the SQLite driver is
  selected and the override is unparseable (including an explicitly present but
  empty environment value), below 1, or above 64, startup shall fail with an
  error naming `database.sqliteReaderConns` before the reader pool is opened.
  The value shall not be silently replaced, clamped, or ignored. A PostgreSQL
  selection shall not fail startup because of this setting.
- **AC-PLATFORM-SQLITE-READER-POOL-001.4:** The override shall be resolved once
  at startup and shall not apply to PostgreSQL. A PostgreSQL startup shall
  remain unaffected by an invalid or out-of-range value for this setting. The
  override shall not change the single-connection SQLite writer pool,
  transaction admission, DSNs, or pragmas.
- **AC-PLATFORM-SQLITE-READER-POOL-001.5:** Reader pool sizing shall not change
  required-store probe interval, probe timeout, probe failure hysteresis,
  destructive unavailability marking, or the immediate fail-closed response to
  missing stores, closed pools, or actual storage faults.

## Out of scope

- Raising the shipped default above four.
- Probe timeout or consecutive-failure hysteresis changes.
- Classifying reader-pool contention as a storage fault.
- Runtime or Settings-page overrides.
- PostgreSQL pool sizing beyond the existing `database.maxConns` and
  `database.minConns`.
