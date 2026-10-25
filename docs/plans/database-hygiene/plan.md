---
created: 2026-10-12
status: complete
requirements:
  - REQ-PLATFORM-SQLITE-FILE-HYGIENE-001
  - REQ-PLATFORM-SQLITE-FILE-HYGIENE-002
  - REQ-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002
system_design:
  - ../../specs/platform/system-design/sqlite-file-hygiene.md
  - ../../specs/system-page/system-design/tool-payload-retention.md
legacy_specs: []
---

# Implementation plan: database hygiene

## Overview

Three small, independent backend fixes keep local SQLite installs from
silently keeping disk space or losing diagnostic evidence:

1. Bound the writer WAL file with `journal_size_limit`.
2. Reclaim boot backup staging directories that a crash left behind.
3. Record why a tool-payload retention backup preparation failed.

Each work order has its own commit and verification, so the items can be
reviewed or merged separately. None of them changes a schema, an API response,
or the UI.

## Source contracts

- [SQLite file hygiene requirements](../../specs/platform/requirements/sqlite-file-hygiene.md)
  and [design](../../specs/platform/system-design/sqlite-file-hygiene.md).
- [Tool payload retention requirements](../../specs/system-page/requirements/tool-payload-retention.md)
  (`AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.9`) and
  [design](../../specs/system-page/system-design/tool-payload-retention.md).
- [ADR 0008](../../decisions/0008-db-upgrade-safety.md) and
  [ADR 2026-10-05](../../decisions/2026-10-05-sqlite-writer-transaction-admission.md)
  stay unchanged.

## Work orders

| Wave | Work order | Result |
| --- | --- | --- |
| 1 | [Task 01: Bound the writer WAL file](task-01-bound-writer-wal.md) | Every writer connection sets `journal_size_limit` |
| 1 | [Task 02: Reclaim boot backup staging](task-02-reclaim-boot-backup-staging.md) | Abandoned `.kandev-backup-*` directories are removed |
| 1 | [Task 03: Record retention preparation failure](task-03-record-retention-preparation-failure.md) | The backup error is logged and kept in the record |

The work orders do not depend on each other and touch different files, except
that Task 01 adds one line to `internal/db/sqlguard/exemptions.json`.

## Assumptions

- The 64 MiB WAL limit and the six-hour staging window are fixed constants.
  No configuration key is added.
- The staging sweep runs on every SQLite boot, not only on upgrade boots. A
  crash during the backup makes the restart another upgrade boot, but that
  debris is still inside the liveness window. Without the every-boot call it
  could stay until the next upgrade.
- The stored preparation message is diagnostic data in the local database. It
  is never returned by the status endpoint.

## Risks

- `journal_size_limit` relies on SQLite truncating the WAL at reset time. This
  is documented SQLite behavior. A permanent test that writes more than 64 MiB
  would be slow, so the permanent test checks the per-connection setting and a
  one-off local probe checks truncation.
- The sweep stats each path instead of trusting directory-listing metadata,
  but a file system can still delay the modification time of a file that is
  open for writing. The six-hour window, not the newest-entry rule, is the
  protection for a live backup in that case.

## Verification strategy

Every work order starts with a test that fails on the base branch and passes
after the change. Run only the changed packages' new tests by name, then
`go build`, `go vet`, and the SQL guard once for the backend. Run the docs
validators once after the specification changes:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

## Review verification

The review integrates current upstream contracts for SQLite snapshot copying,
development database selection, persistence identity, and Windows fixtures.
Task 02 adds failure-warning and concurrent-sweep coverage. Task 03 fixes
invalid UTF-8 expanding the stored diagnostic past its byte limit and covers
diagnostic lifecycle and stale completion guards.

Error logging is also gated on committing the matching preparation failure,
with expected cancellation and stale completion warnings suppressed. The known
shared goal-section fixture failure is addressed by the independently audited
single-file upstream test fix: settle its initial response before editing and
retain its field-error and draft assertions. It changes no frontend product
behavior; final full frontend verification still applies to this PR's candidate.

Local execution is limited to tiny exact-source proofs and static validators
inside an isolated task sandbox. Full backend, frontend, E2E, database, and
static gates are hosted delivery requirements. Their exact head and run
receipts are maintained outside the repository.
