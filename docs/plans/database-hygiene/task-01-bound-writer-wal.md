---
id: "01-bound-writer-wal"
title: "Bound the writer WAL file with journal_size_limit"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-SQLITE-FILE-HYGIENE-001
acceptance_criteria:
  - AC-PLATFORM-SQLITE-FILE-HYGIENE-001.1
  - AC-PLATFORM-SQLITE-FILE-HYGIENE-001.2
  - AC-PLATFORM-SQLITE-FILE-HYGIENE-001.3
system_design:
  - ../../specs/platform/system-design/sqlite-file-hygiene.md
---

# Task 01: Bound the writer WAL file

## Summary

Register a writer-only SQLite driver whose `ConnectHook` sets
`PRAGMA journal_size_limit = 67108864` on every connection. `OpenSQLite` opens
that driver name. Everything else about the writer stays the same.

## In scope

- `internal/db/sqlite.go`: the driver registration in `init`, the hook through
  `driver.ExecerContext`, and the `OpenSQLite` driver name. Keep the DSN,
  `SetMaxOpenConns(1)`, and the reader factory unchanged.
- `internal/db/sqlguard/exemptions.json`: one `sqlite-catalog` exemption for
  the hook symbol, next to the other `internal/db` entries.
- A test in `internal/db/sqlite_test.go` that reads the pragma from a writer
  connection and from a replacement connection after the first one is
  discarded with `driver.ErrBadConn`. It also checks that a reader connection
  keeps the SQLite default.

## Out of scope

Checkpoint scheduling, `wal_autocheckpoint`, reader settings, PostgreSQL, and
any new configuration key.

## Acceptance

- The new test fails on the base branch (the writer reports `-1`) and passes
  after the change.
- `go vet` and the SQL guard pass, and the package compiles with
  `CGO_ENABLED=0`.
- A one-off local probe, not committed, writes more than 64 MiB through the
  writer, lets a checkpoint reset the WAL, and observes that the WAL file is at
  most 64 MiB.

## Verification

```bash
(cd apps/backend && go test -tags fts5 ./internal/db -run '^TestSQLiteWriterJournalSizeLimit' -count=1 -v)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go vet ./internal/db/...)
(cd apps/backend && CGO_ENABLED=0 go build ./internal/db/)
```

## Likely files

- `apps/backend/internal/db/sqlite.go`
- `apps/backend/internal/db/sqlite_test.go`
- `apps/backend/internal/db/sqlguard/exemptions.json`

## Results

Implemented on 2026-10-12, verified on Windows 11 with Go 1.26.

- Red: on the base commit, `TestSQLiteWriterJournalSizeLimit` failed with
  `expected: 67108864, actual: -1` for the first writer connection.
- Green: the test passes, including the replacement connection after
  `driver.ErrBadConn` and the reader's unchanged `-1`. The existing
  `TestSQLiteWriter*` and `TestSQLiteReader*` tests in the package pass.
- `go run ./cmd/sqlguard ./internal`, `go vet ./internal/db/...` and
  `CGO_ENABLED=0 go build ./internal/db/` pass.
- One-off probe (deleted after the run): after an 86,384,072-byte WAL burst,
  the next write reset the WAL to 67,108,864 bytes. The stock driver without
  the limit kept 86,384,072 bytes.
