---
status: active
system: platform
created: 2026-10-28
owners:
  - kandev
---

# Database maintenance JSON requirements

## Overview

Operators need to consume the existing bounded SQLite maintenance report and
committed outcome in shell scripts. This extends the [maintenance
contract](pr-watch-and-bounded-storage.md) without changing retention policy.

## Requirements

### REQ-PLATFORM-MAINTENANCE-JSON-001: Machine-readable maintenance outcome

**Intent:** An operator can request a stable, bounded JSON representation of
what the native database maintenance command reported and committed.

#### Acceptance criteria

- **AC-PLATFORM-MAINTENANCE-JSON-001.1:** `kandev maintenance database --json`
  outputs exactly one JSON object followed by a newline on stdout on success.
  It remains the same read-only dry run, and without `--json` existing text,
  behavior and exit codes remain unchanged. Help and usage retain their existing
  behavior, including usage exit 2.
- **AC-PLATFORM-MAINTENANCE-JSON-001.2:** The object uses the exact fields and
  semantics in the [design](../system-design/maintenance-json.md). It reports
  the same pre-execution candidate counts, estimated bytes, sampling limits,
  availability, and mode as the existing outcome. Totals sum reported groups,
  not necessarily all database candidates or future disk savings.
- **AC-PLATFORM-MAINTENANCE-JSON-001.3:** Explicit `--execute` and `--compact`
  retain all existing lock, verified-backup, transaction, and compaction
  guards. An executed outcome includes committed counts, partial-work state,
  backup path and, only when applied, compaction and rollback details. A later
  failure emits that truthful outcome on stdout, a diagnostic on stderr, and
  exits 1. Failure before an outcome may leave stdout empty.
- **AC-PLATFORM-MAINTENANCE-JSON-001.4:** No human prose or log prefix enters
  JSON stdout; output excludes row contents, task/session identifiers,
  credentials and configuration. Only existing operator-needed backup and
  rollback paths are included. Output write failure is diagnosed on stderr
  and exits nonzero; a truncated write is never treated as success.
- **AC-PLATFORM-MAINTENANCE-JSON-001.5:** The public CLI reference documents
  the schema, an actual `jq` example, bounded sample and byte-estimate limits,
  and the distinction between partial work and a failed command.

## Implementation plans

- [Maintenance JSON](../../../plans/maintenance-json/plan.md)
